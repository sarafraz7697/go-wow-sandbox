package tbc

import (
	"context"
	"time"
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
	r := wire.NewReader(p.Payload)
	guid := r.Uint64()
	if err := r.Finish(); err != nil {
		return nil, err
	}
	fail := func() (*world.Packet, error) {
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
	if err != nil || !sameSession(s.account, current) {
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
	packets, err := codec.LoginPackets(player, h.Data, time.Now())
	if err != nil {
		s.log.Warn("player snapshot unsupported", "guid", guid, "error", err)
		return fail()
	}
	current, err = h.lookup(ctx, s.name, s.ip)
	if err != nil || !sameSession(s.account, current) {
		return nil, account.ErrChanged
	}
	if err = s.send(packets...); err != nil {
		return nil, err
	}
	player.Entered = true
	s.joined = time.Now()
	s.logoutAt = time.Time{}
	s.syncCounter = 0
	s.syncPending = map[uint32]time.Time{}
	if err = s.transition(StateInWorld); err != nil {
		return nil, err
	}
	restore = false
	s.log.Info("player entered world", "guid", guid, "map", player.Map, "mode", "stationary")
	return nil, h.sendTimeSync(s)
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
			return err
		}
		if err := s.transition(StateAuthed); err != nil {
			return err
		}
		return s.send(world.Packet{Opcode: op.SMSG_LOGOUT_COMPLETE})
	}
	if !now.Before(s.nextSync) {
		current, err := h.lookup(ctx, s.name, s.ip)
		if err != nil || !sameSession(s.account, current) {
			return account.ErrChanged
		}
		return h.sendTimeSync(s)
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
		if guid != uint64(s.player.GUID) {
			return nil, nil
		}
		result, e := codec.NameQuery(s.player)
		return &result, e
	})
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
		if guid != uint64(s.player.GUID) {
			return nil, codec.ErrWorldPacket
		}
		return nil, nil
	})
	add(op.CMSG_TIME_SYNC_RESP, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		r := wire.NewReader(p.Payload)
		counter, clock := r.Uint32(), r.Uint32()
		if e := r.Finish(); e != nil {
			return nil, e
		}
		if sent, ok := s.syncPending[counter]; ok {
			s.log.Debug("time sync acknowledged", "counter", counter, "client_clock", clock, "rtt_ms", time.Since(sent).Milliseconds())
			delete(s.syncPending, counter)
		}
		return nil, nil
	})
	router.routes[op.CMSG_LOGOUT_REQUEST] = route{[]State{StateInWorld}, func(ctx context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		if len(p.Payload) != 0 {
			return nil, codec.ErrWorldPacket
		}
		if e := s.transition(StateLoggingOut); e != nil {
			return nil, e
		}
		// Without rest-area geometry or GM policy, use the normal 20-second path.
		s.logoutAt = time.Now().Add(20 * time.Second)
		s.log.Debug("logout countdown started", "guid", s.player.GUID)
		return &world.Packet{Opcode: op.SMSG_LOGOUT_RESPONSE, Payload: make([]byte, 5)}, nil
	}}
	router.routes[op.CMSG_LOGOUT_CANCEL] = route{[]State{StateLoggingOut}, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		if len(p.Payload) != 0 {
			return nil, codec.ErrWorldPacket
		}
		s.logoutAt = time.Time{}
		if e := s.transition(StateInWorld); e != nil {
			return nil, e
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
	add(op.CMSG_ZONEUPDATE, consume(4))
	add(op.CMSG_SET_SELECTION, consume(8))
	add(op.CMSG_STANDSTATECHANGE, consume(4))
	add(op.CMSG_MOVE_TIME_SKIPPED, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		r := wire.NewReader(p.Payload)
		if r.Uint64() != uint64(s.player.GUID) {
			return nil, codec.ErrWorldPacket
		}
		r.Uint32()
		return nil, r.Finish()
	})
	add(op.CMSG_MOVE_NOT_ACTIVE_MOVER, func(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
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
		return nil, codec.ValidateRootAck(p.Payload, uint64(s.player.GUID))
	})
	// Movement reports can arrive while the client settles onto terrain. Parse
	// them for framing, but never apply position, flags or client timestamps.
	for _, opcode := range []uint32{op.MSG_MOVE_HEARTBEAT, op.MSG_MOVE_FALL_LAND, op.CMSG_MOVE_FALL_RESET, op.MSG_MOVE_START_FORWARD, op.MSG_MOVE_START_BACKWARD, op.MSG_MOVE_STOP, op.MSG_MOVE_START_STRAFE_LEFT, op.MSG_MOVE_START_STRAFE_RIGHT, op.MSG_MOVE_STOP_STRAFE, op.MSG_MOVE_START_TURN_LEFT, op.MSG_MOVE_START_TURN_RIGHT, op.MSG_MOVE_STOP_TURN, op.MSG_MOVE_SET_FACING, op.MSG_MOVE_JUMP} {
		add(opcode, func(_ context.Context, _ *Session, p world.Packet) (*world.Packet, error) {
			return nil, codec.ValidateMovement(p.Payload)
		})
	}
}
func consume(size int) handlerFunc {
	return func(_ context.Context, _ *Session, p world.Packet) (*world.Packet, error) {
		if len(p.Payload) != size {
			return nil, codec.ErrWorldPacket
		}
		return nil, nil
	}
}
