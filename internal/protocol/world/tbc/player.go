package tbc

import (
	"context"
	"time"
	characterstats "wowserver/internal/application/character/stats"
	"wowserver/internal/application/movement"
	"wowserver/internal/domain/account"
	"wowserver/internal/domain/character"
	domainworld "wowserver/internal/domain/world"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

type PlayerStore interface {
	AcquirePlayer(context.Context, uint64, uint32) (*character.Player, error)
	ReleasePlayer(context.Context, uint64, *character.Player, uint32) error
}

func (h *Handler) playerLogin(ctx context.Context, s *Session, p world.Packet) (response *world.Packet, resultErr error) {
	s.log.Debug("player login requested")
	r := wire.NewReader(p.Payload)
	guid := r.Uint64()
	if err := r.Finish(); err != nil {
		return nil, err
	}
	fail := func() (*world.Packet, error) {
		// The client tears down its tentative world UI after a rejected login
		// too, and may flush CANCEL_TRADE before returning to character selection.
		// 0x45 = CHAR_LOGIN_FAILED (the generic failure; 0x46 is DISABLED).
		s.recentlyLoggedOut = true
		return &world.Packet{Opcode: op.SMSG_CHARACTER_LOGIN_FAILED, Payload: []byte{0x45}}, nil
	}
	if h.Players == nil || h.Data == nil || guid == 0 || guid>>32 != 0 {
		return fail()
	}
	if err := s.transition(StateLoggingIn); err != nil {
		return nil, err
	}
	restore := true
	defer func() {
		if restore {
			_ = s.transition(StateAuthed)
		}
	}()
	ctx, end := context.WithTimeout(ctx, h.RequestTimeout)
	defer end()
	current, err := h.lookup(ctx, s.name, s.ip)
	if err != nil {
		// Transient DB error: do not drop the session. The client can retry
		// login from the character screen. A real session change returns a
		// row with a different key, not an error.
		s.log.Warn("session lookup failed before acquire", "error", err)
		return fail()
	}
	if !sameSession(s.account, current) {
		return nil, account.ErrChanged
	}
	s.log.Debug("loading player", "guid", guid)
	player, err := h.Players.AcquirePlayer(ctx, s.account.ID, uint32(guid))
	if err != nil {
		s.log.Warn("player login rejected", "guid", guid, "error", err)
		return fail()
	}
	s.player = player
	// Any failure after acquisition releases online even if the request timed out.
	defer func() {
		if restore && s.player != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), h.RequestTimeout)
			defer cancel()
			if e := h.releasePlayer(cleanup, s); e != nil {
				s.log.Error("failed login cleanup failed", "guid", guid, "error", e)
				// Close this session rather than letting another login overwrite
				// the still-owned snapshot and lose the opportunity to release it.
				response, resultErr = nil, e
			}
		}
	}()
	player.Stats, err = characterstats.Calculate(player, h.Data)
	if err != nil {
		s.log.Warn("player stat calculation failed", "error", err)
		return fail()
	}
	if len(player.Stats.Limitations) > 0 {
		s.log.Warn("player stats use base and equipment calculation", "limitations", player.Stats.Limitations)
	}
	packets, err := codec.LoginPackets(player, h.Data, time.Now())
	if err != nil {
		s.log.Warn("player snapshot unsupported", "guid", guid, "error", err)
		return fail()
	}
	current, err = h.lookup(ctx, s.name, s.ip)
	if err != nil {
		s.log.Warn("session lookup failed before send", "error", err)
		return fail()
	}
	if !sameSession(s.account, current) {
		return nil, account.ErrChanged
	}
	if err = s.send(packets...); err != nil {
		// A send failure (queue full, slow client) must not drop the session.
		// Release the acquired player and let the client retry login; the
		// fail response keeps the connection alive.
		s.log.Warn("login packets send failed; releasing player", "guid", guid, "error", err)
		return fail()
	}
	player.Entered = true
	s.recentlyLoggedOut = false
	s.joined = time.Now()
	s.logoutAt = time.Time{}
	s.syncCounter = 0
	s.syncPending = map[uint32]time.Time{}
	if err = s.transition(StateInWorld); err != nil {
		return nil, err
	}
	restore = false
	s.log.Info("player entered world", "guid", guid, "map", player.Map, "mode", "local movement")
	// Register the player in the world simulation so broadcasts can reach
	// other players on the same map. The presence snapshot includes the
	// team (from ChrRaces.BaseLanguage via gamedata) for faction-filtered
	// broadcasts in future slices.
	h.registerInWorld(s, player)
	// The player is now InWorld. An initial time-sync send failure is not
	// fatal; the tick loop retries on the next cadence.
	if syncErr := h.sendTimeSync(s); syncErr != nil {
		s.log.Warn("initial time sync send failed; will retry", "guid", guid, "error", syncErr)
		s.nextSync = time.Now().Add(5 * time.Second)
	}
	return nil, nil
}
func (h *Handler) releasePlayer(ctx context.Context, s *Session) error {
	if s.player == nil {
		return nil
	}
	seconds := uint32(0)
	if s.player.Entered && !s.joined.IsZero() {
		seconds = uint32(time.Since(s.joined) / time.Second)
	}
	if err := h.Players.ReleasePlayer(ctx, s.account.ID, s.player, seconds); err != nil {
		return err
	}
	s.log.Info("player saved and released", "guid", s.player.GUID, "played_seconds", seconds)
	// Leave every joined chat channel before the despawn flow (reference:
	// LogoutPlayer calls CleanupChannels; remaining members get the LEFT
	// notice, the leaving player gets nothing).
	h.leaveAllChannels(s)
	// Despawn flow: broadcast DESTROY_OBJECT to nearby players so they
	// stop rendering this player. Must happen before Unregister, while
	// the player is still in the world registry.
	if h.Broadcast != nil {
		destroyPkt := codec.DestroyObject(s.player.GUID)
		h.Broadcast.Broadcast(s.player.GUID, domainworld.Packet{Opcode: destroyPkt.Opcode, Payload: destroyPkt.Payload}, false)
		h.Broadcast.Unregister(s.player.GUID)
	}
	if s.player.Entered {
		s.recentlyLoggedOut = true
	}
	s.player = nil
	s.logoutAt = time.Time{}
	s.syncPending = nil
	return nil
}

// sessionSender adapts the session's send/trySend closures to the
// domain/world.Sender interface. The world simulation holds one of these
// per in-world player and calls TrySend from broadcast primitives.
type sessionSender struct {
	send    func(...world.Packet) error
	trySend func(world.Packet) bool
}

func (s *sessionSender) Send(packets ...domainworld.Packet) error {
	if len(packets) == 0 {
		return nil
	}
	protos := make([]world.Packet, len(packets))
	for i, p := range packets {
		protos[i] = world.Packet{Opcode: p.Opcode, Payload: p.Payload}
	}
	return s.send(protos...)
}

func (s *sessionSender) TrySend(p domainworld.Packet) bool {
	return s.trySend(world.Packet{Opcode: p.Opcode, Payload: p.Payload})
}

// registerInWorld adds the player to the world simulation's broadcast
// registry. The presence snapshot is built from the loaded Player and the
// gamedata Catalog (for the team field). If the Broadcaster is nil or the
// registration fails, the player still enters the world; broadcasts just
// won't reach or from this session.
func (h *Handler) registerInWorld(s *Session, p *character.Player) {
	if h.Broadcast == nil {
		return
	}
	race, _ := h.Data.Race(uint32(p.Race))
	class, _ := h.Data.Class(uint32(p.Class))
	display := race.MaleDisplay
	if p.Gender == 1 {
		display = race.FemaleDisplay
	}
	// Build the full presence snapshot needed for CREATE_OBJECT.
	var visibleItems [19]uint32
	for _, item := range p.Items {
		if item.Bag == 0 && item.Slot < 19 {
			visibleItems[item.Slot] = item.Entry
		}
	}
	presence := domainworld.PlayerPresence{
		GUID:         p.GUID,
		AccountID:    s.account.ID,
		Name:         p.Name,
		Team:         race.Team,
		Race:         p.Race,
		Class:        p.Class,
		Map:          p.Map,
		Zone:         p.Zone,
		X:            p.X,
		Y:            p.Y,
		Z:            p.Z,
		Orientation:  p.Orientation,
		Gender:       p.Gender,
		Level:        p.Level,
		Power:        uint8(class.Power),
		Health:       p.Health,
		MaxHealth:    p.Stats.MaxHealth,
		Mana:         p.Mana,
		MaxMana:      p.Stats.MaxMana,
		Rage:         p.Rage,
		Energy:       p.Energy,
		DisplayID:    display,
		PlayerFlags:  p.PlayerFlags,
		PlayerBytes:  p.PlayerBytes,
		PlayerBytes2: p.PlayerBytes2,
		VisibleItems: visibleItems,
	}
	sender := &sessionSender{send: s.send, trySend: s.trySend}
	if err := h.Broadcast.Register(presence, sender); err != nil {
		s.log.Warn("world registration failed; broadcasts disabled for this session", "guid", p.GUID, "error", err)
		return
	}
	// Spawn flow: tell nearby players about the new player, and tell the new
	// player about nearby players. Both directions use CREATE_OBJECT.
	clock := uint32(time.Now().UnixMilli())
	// 1. Broadcast self's CREATE_OBJECT to nearby players (not self).
	spawnPkt := codec.CreateObjectForObserver(presence, clock)
	h.Broadcast.Broadcast(p.GUID, domainworld.Packet{Opcode: spawnPkt.Opcode, Payload: spawnPkt.Payload}, false)
	// 2. Send each nearby player's CREATE_OBJECT to the new player.
	nearby := h.Broadcast.Nearby(p.GUID)
	for _, np := range nearby {
		pkt := codec.CreateObjectForObserver(np, clock)
		_ = s.send(pkt)
	}
	// 3. Spawn nearby creatures and gameobjects. The visibility radius
	// matches DEFAULT_VISIBILITY_DISTANCE (90 yards). We track which
	// objects we've spawned so handleMovement can despawn ones the player
	// has moved away from.
	h.spawnNearbyObjects(s)
	s.log.Debug("player spawned in world", "guid", p.GUID, "nearby_players", len(nearby))
}

// spawnNearbyObjects sends CREATE_OBJECT for all creatures and gameobjects
// within the default visibility radius of the player's current position.
// Called on world enter and when the player moves far enough to need a
// visibility refresh.
func (h *Handler) spawnNearbyObjects(s *Session) {
	if s.player == nil || h.Data == nil {
		return
	}
	const radius = 90.0 // DEFAULT_VISIBILITY_DISTANCE
	clock := uint32(time.Now().UnixMilli())
	// Creatures.
	creatures := h.Data.CreatureSpawnsNear(s.player.Map, s.player.X, s.player.Y, s.player.Z, radius)
	for _, cs := range creatures {
		resolved, ok := h.Data.ResolveCreatureSpawn(cs)
		if !ok {
			continue
		}
		// Live position/facing: a wandering creature is not at its spawn point.
		if lx, ly, lz, lo, moved := h.currentCreaturePosition(cs.GUID, time.Now()); moved {
			resolved.X, resolved.Y, resolved.Z, resolved.Orientation = lx, ly, lz, lo
		}
		pkt := codec.CreateObjectForCreature(resolved, clock)
		_ = s.send(pkt)
	}
	// Gameobjects.
	gos := h.Data.GameObjectSpawnsNear(s.player.Map, s.player.X, s.player.Y, s.player.Z, radius)
	for _, gs := range gos {
		tmpl, ok := h.Data.ResolveGameObjectSpawn(gs)
		if !ok {
			continue
		}
		pkt := codec.CreateObjectForGameObject(gs.GUID, gs, tmpl)
		_ = s.send(pkt)
	}
	s.log.Debug("spawned nearby objects", "guid", s.player.GUID, "creatures", len(creatures), "gameobjects", len(gos))
}

// sendLogoutStandState mirrors Unit::SetStandState: in addition to the
// partial UPDATE_OBJECT that rewrites UNIT_FIELD_BYTES_1 byte 0, the
// reference also sends the standalone SMSG_STANDSTATE_UPDATE packet.
// Sending both matches MaNGOS byte-for-byte; some clients rely on the
// standalone message to animate the body and not just update the field.
func (h *Handler) sendLogoutStandState(s *Session, state uint8) {
	if s.player == nil {
		return
	}
	if upd, e := codec.LogoutStateUpdate(s.player.GUID, state); e == nil {
		_ = s.send(upd)
	}
	_ = s.send(codec.StandStateMessage(state))
}
func (h *Handler) sendTimeSync(s *Session) error {
	now := time.Now()
	s.nextSync = now.Add(10 * time.Second)
	for counter, sent := range s.syncPending {
		if now.Sub(sent) > 30*time.Second {
			delete(s.syncPending, counter)
		}
	}
	counter := s.syncCounter
	s.syncCounter++
	s.syncPending[counter] = now
	w := wire.NewWriter(4)
	w.Uint32(counter)
	b, _ := w.Result()
	return s.send(world.Packet{Opcode: op.SMSG_TIME_SYNC_REQ, Payload: b})
}
func (h *Handler) playerTick(ctx context.Context, s *Session) error {
	if s.player == nil {
		return nil
	}
	now := time.Now()
	// Creature random movement (MovementType 1) around this player.
	h.tickCreatureMotion(s.player.Map, s.player.X, s.player.Y, now)
	// Resting housekeeping: if the player is resting in a tavern, re-check
	// their position against the trigger volume every tick. MaNGOS does this
	// in UpdateAreaZoneAndOutdoors on zone change; we don't have zone-update
	// geometry yet, so a per-tick check is the cheapest correct source.
	// If the player left the volume, clear Resting and send a partial update
	// clearing PLAYER_FLAGS_RESTING.
	if s.player.Resting && s.player.InnTriggerID != 0 {
		t, ok := h.Data.TavernTrigger(s.player.InnTriggerID)
		if !ok || !codec.IsPointInAreaTrigger(t, s.player.Map, s.player.X, s.player.Y, s.player.Z, 5) {
			s.player.Resting = false
			s.player.InnTriggerID = 0
			s.log.Debug("player left tavern; resting cleared", "guid", s.player.GUID)
			if upd, e := codec.RestingStateUpdate(s.player.GUID, s.player.PlayerFlags, false); e == nil {
				_ = s.send(upd)
			}
		}
	}
	if s.state == StateLoggingOut && !s.logoutAt.IsZero() && !now.Before(s.logoutAt) {
		saveCtx, end := context.WithTimeout(ctx, h.RequestTimeout)
		defer end()
		if err := h.releasePlayer(saveCtx, s); err != nil {
			// A transient DB error must not abort the session. The player is
			// still owned (online=1, s.player non-nil); retry on the next
			// tick. The session closer's defer retries once more if the
			// connection drops before then.
			s.log.Warn("logout save failed; will retry", "guid", s.player.GUID, "error", err)
			s.logoutAt = now.Add(5 * time.Second)
			return nil
		}
		if err := s.transition(StateAuthed); err != nil {
			return err
		}
		// LOGOUT_COMPLETE is the final confirmation. A send failure (queue
		// full, slow client) must not undo a successful save or drop the
		// session; the client also returns to the character screen on its
		// own after the save, and the next relogin reuses this connection.
		if err := s.send(world.Packet{Opcode: op.SMSG_LOGOUT_COMPLETE}); err != nil {
			s.log.Warn("logout complete send failed; player already saved", "error", err)
		}
		return nil
	}
	if !now.Before(s.nextSync) {
		current, err := h.lookup(ctx, s.name, s.ip)
		if err != nil {
			// Transient DB error; retry without dropping the session. A real
			// session change returns a row with a different key, not an error.
			s.log.Debug("session lookup failed; will retry", "error", err)
			s.nextSync = now.Add(5 * time.Second)
			return nil
		}
		if !sameSession(s.account, current) {
			return account.ErrChanged
		}
		// Time sync is a keep-alive cadence, not a correctness requirement.
		// A transient send failure should not terminate the session.
		if err := h.sendTimeSync(s); err != nil {
			s.log.Debug("time sync send failed; will retry next tick", "error", err)
			s.nextSync = now.Add(5 * time.Second)
		}
		return nil
	}
	return nil
}

// joinChannel adds the session's player to a chat channel and sends the
// notify packets. Mirrors Channel::Join + MakeYouJoined + MakeJoined.
// The channel registry is the domain contract satisfied by the world
// simulation; asserting the concrete per-method interfaces that were here
// before never matched *world.World's method signatures (a *Channel return
// does not satisfy an interface method returning interface{...}), so every
// join silently did nothing.
func (h *Handler) joinChannel(s *Session, name string, channelID uint32) {
	w, ok := h.Broadcast.(domainworld.ChannelRegistry)
	if !ok {
		return
	}
	sender := &sessionSender{send: s.send, trySend: s.trySend}
	ch, existing, _ := w.JoinChannel(name, s.player.GUID, sender)
	if ch == nil {
		return
	}
	// Send YOU_JOINED to the joining player.
	notify := codec.BuildChannelNotifyYouJoined(name, ch.Flags(), channelID)
	_ = s.send(notify)
	// Broadcast JOINED (someone joined) to existing members.
	joined := codec.BuildChannelNotifyJoined(name, s.player.GUID)
	for _, es := range existing {
		es.TrySend(domainworld.Packet{Opcode: joined.Opcode, Payload: joined.Payload})
	}
	s.log.Debug("player joined channel", "channel", name, "guid", s.player.GUID, "members", ch.Count())
}

// leaveChannel removes the session's player from a chat channel and sends
// the notify packets. Mirrors Channel::Leave(send=true) + MakeYouLeft +
// MakeLeft: the leaving player gets YOU_LEFT, the others get LEFT.
func (h *Handler) leaveChannel(s *Session, name string) {
	w, ok := h.Broadcast.(domainworld.ChannelRegistry)
	if !ok {
		return
	}
	ch, remaining := w.LeaveChannel(name, s.player.GUID)
	// Send YOU_LEFT to the leaving player.
	left := codec.BuildChannelNotifyYouLeft(name, 0)
	_ = s.send(left)
	// Broadcast LEFT (someone left) to remaining members.
	notify := codec.BuildChannelNotifyLeft(name, s.player.GUID)
	for _, rs := range remaining {
		rs.TrySend(domainworld.Packet{Opcode: notify.Opcode, Payload: notify.Payload})
	}
	if ch != nil {
		s.log.Debug("player left channel", "channel", name, "guid", s.player.GUID, "members", ch.Count())
	}
}

// leaveAllChannels removes the player from every joined channel during
// logout/disconnect. Mirrors Player::CleanupChannels: the LEFT notice is
// broadcast to the remaining members, but the leaving player is NOT sent
// YOU_LEFT (Channel::Leave is called with send=false during cleanup).
func (h *Handler) leaveAllChannels(s *Session) {
	if s.player == nil {
		return
	}
	w, ok := h.Broadcast.(domainworld.ChannelRegistry)
	if !ok {
		return
	}
	for _, name := range w.PlayerChannels(s.player.GUID) {
		_, remaining := w.LeaveChannel(name, s.player.GUID)
		notify := codec.BuildChannelNotifyLeft(name, s.player.GUID)
		for _, rs := range remaining {
			rs.TrySend(domainworld.Packet{Opcode: notify.Opcode, Payload: notify.Payload})
		}
		s.log.Debug("player left channel on cleanup", "channel", name, "guid", s.player.GUID)
	}
}

func (h *Handler) registerPlayerRoutes(router *Router) {
	router.routes[op.CMSG_PLAYER_LOGIN] = route{[]State{StateAuthed}, h.playerLogin}
	inWorld := []State{StateInWorld, StateLoggingOut}
	add := func(opcode uint32, fn handlerFunc) { router.routes[opcode] = route{inWorld, fn} }
	// Channel system opcodes. These require the World to be a concrete
	// *world.World (or implement the channel methods); if Broadcast is the
	// NoopBroadcaster, these are no-ops.
	add(op.CMSG_JOIN_CHANNEL, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		if s.player == nil {
			return nil, nil
		}
		name, pass, chId, err := codec.DecodeJoinChannel(p.Payload)
		if err != nil {
			return nil, err
		}
		_ = pass // password support is future work; we accept any
		h.joinChannel(s, name, chId)
		return nil, nil
	})
	add(op.CMSG_LEAVE_CHANNEL, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		if s.player == nil {
			return nil, nil
		}
		name, err := codec.DecodeLeaveChannel(p.Payload)
		if err != nil {
			return nil, err
		}
		h.leaveChannel(s, name)
		return nil, nil
	})
	add(op.CMSG_NAME_QUERY, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		r := wire.NewReader(p.Payload)
		guid := r.Uint64()
		if e := r.Finish(); e != nil {
			return nil, e
		}
		if s.player == nil {
			return nil, nil
		}
		// Self query: use the session's own player snapshot.
		if guid == uint64(s.player.GUID) {
			result, e := codec.NameQuery(s.player)
			return &result, e
		}
		// Other player: look up in the world registry.
		if h.Broadcast != nil {
			if presence, ok := h.Broadcast.(interface {
				Get(uint32) (domainworld.PlayerPresence, bool)
			}).Get(uint32(guid)); ok {
				result, e := codec.NameQueryFromPresence(presence.Name, presence.GUID, uint32(presence.Race), uint32(presence.Gender), uint32(presence.Class))
				return &result, e
			}
		}
		return nil, nil
	})
	nameQuery := router.routes[op.CMSG_NAME_QUERY]
	nameQuery.states = []State{StateAuthed, StateInWorld, StateLoggingOut}
	router.routes[op.CMSG_NAME_QUERY] = nameQuery
	add(op.CMSG_ITEM_QUERY_SINGLE, func(_ context.Context, _ *Session, p world.Packet) (*world.Packet, error) {
		r := wire.NewReader(p.Payload)
		id := r.Uint32()
		if r.Remaining() == 8 {
			r.Uint64()
		}
		if e := r.Finish(); e != nil {
			return nil, e
		}
		result, e := codec.ItemQuery(id, h.Data)
		return &result, e
	})
	// CMSG_CREATURE_QUERY / CMSG_GAMEOBJECT_QUERY: uint32 entry + uint64 guid.
	// Mirrors WorldSession::HandleCreatureQueryOpcode / HandleGameObjectQueryOpcode
	// (QueryHandler.cpp). Without these routes the client never receives the
	// template name and shows every NPC as "Unknown".
	add(op.CMSG_CREATURE_QUERY, func(_ context.Context, _ *Session, p world.Packet) (*world.Packet, error) {
		r := wire.NewReader(p.Payload)
		entry := r.Uint32()
		if r.Remaining() == 8 {
			r.Uint64() // spawn guid, unused
		}
		if e := r.Finish(); e != nil {
			return nil, e
		}
		if h.Data == nil {
			return nil, nil
		}
		result, e := codec.CreatureQuery(entry, h.Data)
		return &result, e
	})
	add(op.CMSG_GAMEOBJECT_QUERY, func(_ context.Context, _ *Session, p world.Packet) (*world.Packet, error) {
		r := wire.NewReader(p.Payload)
		entry := r.Uint32()
		if r.Remaining() == 8 {
			r.Uint64() // spawn guid, unused
		}
		if e := r.Finish(); e != nil {
			return nil, e
		}
		if h.Data == nil {
			return nil, nil
		}
		result, e := codec.GameObjectQuery(entry, h.Data)
		return &result, e
	})
	add(op.CMSG_LIST_INVENTORY, h.listInventory)
	add(op.CMSG_SET_ACTIVE_MOVER, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		r := wire.NewReader(p.Payload)
		guid := r.Uint64()
		if e := r.Finish(); e != nil {
			return nil, e
		}
		if s.player == nil {
			return nil, nil
		}
		if guid != uint64(s.player.GUID) {
			return nil, codec.ErrWorldPacket
		}
		return nil, nil
	})
	add(op.CMSG_TIME_SYNC_RESP, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		r := wire.NewReader(p.Payload)
		counter := r.Uint32()
		r.Uint32() // client clock; no per-packet success log
		if e := r.Finish(); e != nil {
			return nil, e
		}
		if _, ok := s.syncPending[counter]; ok {
			delete(s.syncPending, counter)
		}
		return nil, nil
	})
	router.routes[op.CMSG_LOGOUT_REQUEST] = route{[]State{StateInWorld}, func(ctx context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		if len(p.Payload) != 0 {
			return nil, codec.ErrWorldPacket
		}
		if s.player == nil {
			return nil, nil
		}
		// Reference: HandleLogoutRequestOpcode. instantLogout is true when the
		// player is resting (PLAYER_FLAGS_RESTING), on a taxi, or has GM
		// permission. We support the resting path here; taxi and GM come later.
		instantLogout := s.player.Resting
		if instantLogout {
			// Mirror LogoutPlayer(true): save+release synchronously, return
			// StateAuthed, send LOGOUT_RESPONSE(reason=0, instant=1) then
			// LOGOUT_COMPLETE. No countdown, no root, no sit.
			s.log.Debug("instant logout (resting)", "guid", s.player.GUID)
			saveCtx, end := context.WithTimeout(ctx, h.RequestTimeout)
			defer end()
			if err := h.releasePlayer(saveCtx, s); err != nil {
				// A transient DB error must not drop the session: fall back
				// to the countdown path so the tick loop retries the save.
				s.log.Warn("instant logout save failed; falling back to countdown", "guid", s.player.GUID, "error", err)
				if e := s.transition(StateLoggingOut); e != nil {
					return nil, e
				}
				s.logoutAt = time.Now().Add(20 * time.Second)
				h.sendLogoutStandState(s, codec.StandStateSit)
				s.send(codec.ForceMoveRoot(s.player.GUID))
				resp := codec.LogoutResponse(0, false)
				return &resp, nil
			}
			if err := s.transition(StateAuthed); err != nil {
				return nil, err
			}
			_ = s.send(codec.LogoutResponse(0, true))
			_ = s.send(world.Packet{Opcode: op.SMSG_LOGOUT_COMPLETE})
			return nil, nil
		}
		if e := s.transition(StateLoggingOut); e != nil {
			return nil, e
		}
		s.logoutAt = time.Now().Add(20 * time.Second)
		s.log.Debug("logout countdown started", "guid", s.player.GUID)
		// Reference: HandleLogoutRequestOpcode sends SMSG_LOGOUT_RESPONSE
		// FIRST, then sits the player and roots them so the client cannot move
		// while the countdown ticks:
		//   SMSG_LOGOUT_RESPONSE (reason=0, instant=0)
		//   SetStandState(UNIT_STAND_STATE_SIT)  -> partial UPDATE_OBJECT + SMSG_STANDSTATE_UPDATE
		//   SetRoot(true)                         -> SMSG_FORCE_MOVE_ROOT
		//   UNIT_FLAG_STUNNED                      -> intentionally NOT set here; the
		//      reference sets it together with SetRoot, but without a virtual-item /
		//      sheath pipeline the stunned flag makes the client draw weapons.
		// The response is sent through the same queue so the wire order is
		// response, sit, root (returning it would queue it last).
		_ = s.send(codec.LogoutResponse(0, false))
		h.sendLogoutStandState(s, codec.StandStateSit)
		_ = s.send(codec.ForceMoveRoot(s.player.GUID))
		return nil, nil
	}}
	router.routes[op.CMSG_LOGOUT_CANCEL] = route{[]State{StateLoggingOut, StateInWorld}, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		if len(p.Payload) != 0 {
			return nil, codec.ErrWorldPacket
		}
		s.logoutAt = time.Time{}
		if s.state == StateLoggingOut {
			if e := s.transition(StateInWorld); e != nil {
				return nil, e
			}
			// Reference: HandleLogoutCancelOpcode sends SMSG_LOGOUT_CANCEL_ACK
			// first, then unroots the player and stands them up.
			_ = s.send(world.Packet{Opcode: op.SMSG_LOGOUT_CANCEL_ACK})
			if s.player != nil {
				_ = s.send(codec.ForceMoveUnroot(s.player.GUID))
				h.sendLogoutStandState(s, codec.StandStateStand)
			}
			return nil, nil
		}
		return &world.Packet{Opcode: op.SMSG_LOGOUT_CANCEL_ACK}, nil
	}}
	add(op.CMSG_QUERY_TIME, func(_ context.Context, _ *Session, p world.Packet) (*world.Packet, error) {
		if len(p.Payload) != 0 {
			return nil, codec.ErrWorldPacket
		}
		w := wire.NewWriter(8)
		w.Uint32(uint32(time.Now().Unix()))
		w.Uint32(0)
		b, _ := w.Result()
		return &world.Packet{Opcode: op.SMSG_QUERY_TIME_RESPONSE, Payload: b}, nil
	})
	add(op.CMSG_PLAYED_TIME, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		if len(p.Payload) != 0 {
			return nil, codec.ErrWorldPacket
		}
		if s.player == nil {
			return nil, nil
		}
		elapsed := uint32(time.Since(s.joined) / time.Second)
		w := wire.NewWriter(8)
		w.Uint32(s.player.Played + elapsed)
		w.Uint32(s.player.LevelPlayed + elapsed)
		b, _ := w.Result()
		return &world.Packet{Opcode: op.SMSG_PLAYED_TIME, Payload: b}, nil
	})
	for _, opcode := range []uint32{op.CMSG_TUTORIAL_FLAG, op.CMSG_TUTORIAL_CLEAR, op.CMSG_TUTORIAL_RESET} {
		add(opcode, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
			want := 0
			if p.Opcode == op.CMSG_TUTORIAL_FLAG {
				want = 4
			}
			if len(p.Payload) != want {
				return nil, codec.ErrWorldPacket
			}
			if s.player == nil {
				return nil, nil
			}
			r := wire.NewReader(p.Payload)
			switch p.Opcode {
			case op.CMSG_TUTORIAL_FLAG:
				bit := r.Uint32()
				if bit >= 256 {
					return nil, codec.ErrWorldPacket
				}
				s.player.Tutorials[bit/32] |= 1 << (bit % 32)
			case op.CMSG_TUTORIAL_CLEAR:
				for i := range s.player.Tutorials {
					s.player.Tutorials[i] = 0xffffffff
				}
			case op.CMSG_TUTORIAL_RESET:
				s.player.Tutorials = [8]uint32{}
			}
			return nil, r.Finish()
		})
	}
	// MaNGOS also leaves account-data synchronization unimplemented. Do not
	// decode compressed client blobs or treat them as authoritative game state.
	add(op.CMSG_UPDATE_ACCOUNT_DATA, func(_ context.Context, _ *Session, p world.Packet) (*world.Packet, error) {
		if len(p.Payload) < 8 {
			return nil, codec.ErrWorldPacket
		}
		return nil, nil
	})
	lateAccountData := router.routes[op.CMSG_UPDATE_ACCOUNT_DATA]
	lateAccountData.states = []State{StateAuthed, StateInWorld, StateLoggingOut}
	router.routes[op.CMSG_UPDATE_ACCOUNT_DATA] = lateAccountData
	add(op.CMSG_REQUEST_ACCOUNT_DATA, consume(4))
	add(op.CMSG_GMTICKET_GETTICKET, func(_ context.Context, _ *Session, p world.Packet) (*world.Packet, error) {
		if len(p.Payload) != 0 {
			return nil, codec.ErrWorldPacket
		}
		return &world.Packet{Opcode: op.SMSG_GMTICKET_GETTICKET, Payload: []byte{10, 0, 0, 0}}, nil
	})
	add(op.CMSG_CONTACT_LIST, func(_ context.Context, _ *Session, p world.Packet) (*world.Packet, error) {
		r := wire.NewReader(p.Payload)
		flags := r.Uint32()
		if e := r.Finish(); e != nil {
			return nil, e
		}
		w := wire.NewWriter(8)
		w.Uint32(flags)
		w.Uint32(0)
		b, _ := w.Result()
		return &world.Packet{Opcode: op.SMSG_CONTACT_LIST, Payload: b}, nil
	})
	for _, opcode := range []uint32{op.MSG_MOVE_WORLDPORT_ACK, op.CMSG_PLAYER_LOGOUT} {
		add(opcode, consume(0))
	}
	router.routes[op.CMSG_PLAYER_LOGOUT] = route{[]State{StateAuthed, StateInWorld, StateLoggingOut}, consume(0)}
	add(op.CMSG_ZONEUPDATE, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		r := wire.NewReader(p.Payload)
		zone := r.Uint32()
		if e := r.Finish(); e != nil {
			return nil, e
		}
		// The client reports its current zone; trust it within the uint32
		// range. Map admission and area-trigger teleport are future
		// world-loop work. Zone 0 is not a valid area; ignore it.
		if s.player != nil && zone != 0 {
			s.player.Zone = zone
		}
		return nil, nil
	})
	// Reference: HandleAreaTriggerOpcode. We only implement the tavern/inn
	// branch: if the trigger id is in areatrigger_tavern, set REST_TYPE_IN_TAVERN
	// (PLAYER_FLAGS_RESTING). Teleport triggers, quest triggers and city rest
	// (REST_TYPE_IN_CITY, which needs AreaTable's AREA_FLAG_CAPITAL) are out of
	// scope for this slice.
	add(op.CMSG_AREATRIGGER, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		if s.player == nil {
			return nil, nil
		}
		r := wire.NewReader(p.Payload)
		id := r.Uint32()
		if e := r.Finish(); e != nil {
			return nil, e
		}
		t, ok := h.Data.TavernTrigger(id)
		if !ok {
			return nil, nil
		}
		// Position validation. delta matches the reference's 5-yard tolerance.
		if !codec.IsPointInAreaTrigger(t, s.player.Map, s.player.X, s.player.Y, s.player.Z, 5) {
			s.log.Debug("areatrigger: player not in trigger volume", "id", id)
			return nil, nil
		}
		if !s.player.Resting {
			s.player.Resting = true
			s.player.InnTriggerID = id
			s.log.Debug("player entered tavern", "trigger", id, "guid", s.player.GUID)
			if upd, e := codec.RestingStateUpdate(s.player.GUID, s.player.PlayerFlags, true); e == nil {
				_ = s.send(upd)
			}
		} else if s.player.InnTriggerID != id {
			// Already resting from a different tavern; switch the trigger id
			// without flipping the flag.
			s.player.InnTriggerID = id
		}
		return nil, nil
	})
	add(op.CMSG_SET_SELECTION, consume(8))
	add(op.CMSG_STANDSTATECHANGE, consume(4))
	add(op.CMSG_MOVE_TIME_SKIPPED, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		if s.player == nil {
			return nil, nil
		}
		r := wire.NewReader(p.Payload)
		if r.Uint64() != uint64(s.player.GUID) {
			return nil, codec.ErrWorldPacket
		}
		r.Uint32()
		return nil, r.Finish()
	})
	add(op.CMSG_MOVE_NOT_ACTIVE_MOVER, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		if s.player == nil {
			return nil, nil
		}
		r := wire.NewReader(p.Payload)
		if r.Uint64() != uint64(s.player.GUID) {
			return nil, codec.ErrWorldPacket
		}
		return nil, codec.ValidateMovement(r.Bytes(r.Remaining()))
	})
	// UI changes are read-only in this stationary slice; malformed payloads are rejected.
	add(op.CMSG_SET_ACTION_BUTTON, consume(5))
	add(op.CMSG_SET_WATCHED_FACTION, consume(4))
	add(op.CMSG_SET_FACTION_ATWAR, consume(5))
	add(op.CMSG_SET_FACTION_INACTIVE, consume(5))
	router.routes[op.CMSG_SET_ACTIONBAR_TOGGLES] = route{[]State{StateAuthed, StateInWorld, StateLoggingOut}, consume(1)}
	add(op.CMSG_FORCE_MOVE_ROOT_ACK, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		if s.player == nil {
			return nil, nil
		}
		return nil, codec.ValidateRootAck(p.Payload, uint64(s.player.GUID))
	})
	// Only the session event loop applies movement. The client renders its own
	// movement; broadcasting to other players is handled by the world
	// simulation via h.Broadcast (SendMessageToSetExcept equivalent).
	for _, opcode := range []uint32{op.MSG_MOVE_HEARTBEAT, op.MSG_MOVE_FALL_LAND, op.CMSG_MOVE_FALL_RESET, op.MSG_MOVE_START_FORWARD, op.MSG_MOVE_START_BACKWARD, op.MSG_MOVE_STOP, op.MSG_MOVE_START_STRAFE_LEFT, op.MSG_MOVE_START_STRAFE_RIGHT, op.MSG_MOVE_STOP_STRAFE, op.MSG_MOVE_START_TURN_LEFT, op.MSG_MOVE_START_TURN_RIGHT, op.MSG_MOVE_STOP_TURN, op.MSG_MOVE_SET_FACING, op.MSG_MOVE_JUMP} {
		add(opcode, h.handleMovement)
	}
	for _, opcode := range []uint32{op.MSG_MOVE_SET_RUN_MODE, op.MSG_MOVE_SET_WALK_MODE, op.MSG_MOVE_START_PITCH_UP, op.MSG_MOVE_START_PITCH_DOWN, op.MSG_MOVE_STOP_PITCH, op.MSG_MOVE_SET_PITCH, op.MSG_MOVE_START_SWIM, op.MSG_MOVE_STOP_SWIM} {
		add(opcode, h.handleMovement)
	}
}

func (h *Handler) handleMovement(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	// A late movement packet must not crash or disconnect the session. If the
	// player is already released (e.g. logout completed in the same tick),
	// silently drop the packet; the state machine already filtered most late
	// packets, but StateLoggingOut can still receive movement before release.
	if s.player == nil {
		return nil, nil
	}
	position, err := codec.DecodeMovementPosition(p.Payload)
	if err != nil {
		return nil, err
	}
	if err = movement.Apply(s.player, position); err != nil {
		return nil, err
	}
	// Update the player's position in the world simulation so future
	// broadcasts from other players reach the new location, and so this
	// player's own broadcasts target the right neighbours.
	if h.Broadcast != nil {
		_ = h.Broadcast.UpdatePosition(s.player.GUID, s.player.Map, s.player.Zone, s.player.X, s.player.Y, s.player.Z, s.player.Orientation)
	}
	// Broadcast the movement to other visible players. MaNGOS builds a new
	// packet with the mover's packed GUID prepended and calls
	// SendMessageToSetExcept. We do the same: the payload is the client's
	// MovementInfo with the packed GUID prepended; the opcode stays the same.
	if h.Broadcast != nil {
		payload := codec.EncodeMovementBroadcast(s.player.GUID, p.Payload)
		h.Broadcast.Broadcast(s.player.GUID, domainworld.Packet{Opcode: p.Opcode, Payload: payload}, false)
	}
	// The reference has NO movement-driven logout cancellation: the rooted
	// client cannot move, and the countdown is only cancelled by an explicit
	// CMSG_LOGOUT_CANCEL (HandleLogoutCancelOpcode). Late movement during
	// StateLoggingOut is applied to the snapshot — which is saved at release —
	// and otherwise ignored; it never produces an unsolicited CANCEL_ACK.
	return nil, nil
}
func consume(size int) handlerFunc {
	return func(_ context.Context, _ *Session, p world.Packet) (*world.Packet, error) {
		if len(p.Payload) != size {
			return nil, codec.ErrWorldPacket
		}
		return nil, nil
	}
}
