package tbc

import (
	"context"
	"time"
	characterstats "wowserver/internal/application/character/stats"
	"wowserver/internal/application/movement"
	"wowserver/internal/domain/account"
	"wowserver/internal/domain/character"
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
		s.recentlyLoggedOut = true
		return &world.Packet{Opcode: op.SMSG_CHARACTER_LOGIN_FAILED, Payload: []byte{0x46}}, nil
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
	if s.player.Entered {
		s.recentlyLoggedOut = true
	}
	s.player = nil
	s.logoutAt = time.Time{}
	s.syncPending = nil
	return nil
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
func (h *Handler) registerPlayerRoutes(router *Router) {
	router.routes[op.CMSG_PLAYER_LOGIN] = route{[]State{StateAuthed}, h.playerLogin}
	inWorld := []State{StateInWorld, StateLoggingOut}
	add := func(opcode uint32, fn handlerFunc) { router.routes[opcode] = route{inWorld, fn} }
	add(op.CMSG_NAME_QUERY, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		r := wire.NewReader(p.Payload)
		guid := r.Uint64()
		if e := r.Finish(); e != nil {
			return nil, e
		}
		if s.player == nil || guid != uint64(s.player.GUID) {
			return nil, nil
		}
		result, e := codec.NameQuery(s.player)
		return &result, e
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
		if e := s.transition(StateLoggingOut); e != nil {
			return nil, e
		}
		// Without rest-area geometry or GM policy, use the normal 20-second path.
		s.logoutAt = time.Now().Add(20 * time.Second)
		s.log.Debug("logout countdown started", "guid", s.player.GUID)
		return &world.Packet{Opcode: op.SMSG_LOGOUT_RESPONSE, Payload: make([]byte, 5)}, nil
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
	// movement; broadcasting to other players requires the future world loop.
	for _, opcode := range []uint32{op.MSG_MOVE_HEARTBEAT, op.MSG_MOVE_FALL_LAND, op.CMSG_MOVE_FALL_RESET, op.MSG_MOVE_START_FORWARD, op.MSG_MOVE_START_BACKWARD, op.MSG_MOVE_STOP, op.MSG_MOVE_START_STRAFE_LEFT, op.MSG_MOVE_START_STRAFE_RIGHT, op.MSG_MOVE_STOP_STRAFE, op.MSG_MOVE_START_TURN_LEFT, op.MSG_MOVE_START_TURN_RIGHT, op.MSG_MOVE_STOP_TURN, op.MSG_MOVE_SET_FACING, op.MSG_MOVE_JUMP} {
		add(opcode, handleMovement)
	}
	for _, opcode := range []uint32{op.MSG_MOVE_SET_RUN_MODE, op.MSG_MOVE_SET_WALK_MODE, op.MSG_MOVE_START_PITCH_UP, op.MSG_MOVE_START_PITCH_DOWN, op.MSG_MOVE_STOP_PITCH, op.MSG_MOVE_SET_PITCH, op.MSG_MOVE_START_SWIM, op.MSG_MOVE_STOP_SWIM} {
		add(opcode, handleMovement)
	}
}

func handleMovement(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
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
	moved := position.X != s.player.X || position.Y != s.player.Y || position.Z != s.player.Z
	if err = movement.Apply(s.player, position); err != nil {
		return nil, err
	}
	// Movement cancels the pending logout now that the player is not rooted.
	if s.state == StateLoggingOut && (moved || p.Opcode == op.MSG_MOVE_JUMP) {
		if err = s.transition(StateInWorld); err != nil {
			return nil, err
		}
		s.logoutAt = time.Time{}
		return &world.Packet{Opcode: op.SMSG_LOGOUT_CANCEL_ACK}, nil
	}
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
