package tbc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"wowserver/internal/domain/account"
	"wowserver/internal/domain/character"
	"wowserver/internal/domain/inventory"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

type State uint8

const (
	StateAuthed State = iota
	StateLoggingIn
	StateInWorld
	StateLoggingOut
	StateClosed
)

type transition struct {
	next   State
	result chan error
}

// Session is owned by its event loop. Readers and world workers must not read
// or mutate state directly. Handler.active remains a separate admission registry.
type Session struct {
	recentlyLoggedOut          bool
	vis                        visibleSet // creatures/gameobjects the client knows (visibility.go)
	unsupportedSeen            map[uint32]bool
	player                     *character.Player
	joined, logoutAt, nextSync time.Time
	inv                        *inventory.Inventory // live inventory (inventory.go)
	loadedItems                map[uint32]struct{}  // item guids present at login
	allocatedItems             []uint32             // guids reserved during the session
	syncCounter                uint32
	syncPending                map[uint32]time.Time
	send                       func(...world.Packet) error
	trySend                    func(world.Packet) bool // non-blocking, for broadcasts
	onTick                     func(context.Context, *Session) error
	state                      State
	account                    account.WorldSession
	name, ip                   string
	log                        *slog.Logger
	events                     chan transition
	done                       <-chan struct{}
}

func (s *Session) transition(next State) error {
	valid := next == StateClosed && s.state != StateClosed
	switch s.state {
	case StateAuthed:
		valid = valid || next == StateLoggingIn
	case StateLoggingIn:
		valid = valid || next == StateAuthed || next == StateInWorld
	case StateInWorld:
		valid = valid || next == StateLoggingOut
	case StateLoggingOut:
		valid = valid || next == StateAuthed || next == StateInWorld
	}
	if !valid {
		return fmt.Errorf("invalid session transition %d -> %d", s.state, next)
	}
	s.state = next
	return nil
}

// RequestTransition is for external workers, never the owning event loop.
// After enqueue, wait for its definitive result (or session shutdown) so a
// caller cannot mistake cancellation for a transition that did not happen.
func (s *Session) RequestTransition(ctx context.Context, next State) error {
	request := transition{next: next, result: make(chan error, 1)}
	select {
	case s.events <- request:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return context.Canceled
	}
	select {
	case err := <-request.result:
		return err
	case <-s.done:
		return context.Canceled
	}
}

type handlerFunc func(context.Context, *Session, world.Packet) (*world.Packet, error)
type route struct {
	states []State
	handle handlerFunc
}
type Router struct{ routes map[uint32]route }

func (r *Router) dispatch(ctx context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	// The client can flush in-world UI commands after LOGOUT_COMPLETE.
	// Never run their handlers against the released player snapshot.
	if s.state == StateAuthed && s.recentlyLoggedOut {
		if m, ok := op.Lookup(p.Opcode); ok && (strings.HasPrefix(m.Name, "CMSG_") || strings.HasPrefix(m.Name, "MSG_")) && (m.Status == op.STATUS_LOGGEDIN || m.Status == op.STATUS_LOGGEDIN_OR_RECENTLY_LOGGEDOUT) && p.Opcode != op.CMSG_UPDATE_ACCOUNT_DATA {
			if s.log != nil {
				s.log.Debug("late world packet ignored after logout", "opcode", p.Opcode, "name", m.Name)
			}
			return nil, nil
		}
	}
	entry, ok := r.routes[p.Opcode]
	if !ok {
		return r.unsupported(s, p)
	}
	for _, state := range entry.states {
		if state == s.state {
			return entry.handle(ctx, s, p)
		}
	}
	return nil, fmt.Errorf("%w: disallowed opcode=%d state=%d", codec.ErrWorldPacket, p.Opcode, s.state)
}

// Explicit allowlist: reference opcode metadata is descriptive, not permission
// to execute an unimplemented handler or to start a goroutine per packet.
func (h *Handler) newRouter() *Router {
	r := &Router{routes: map[uint32]route{}}
	check := func(ctx context.Context, s *Session) error {
		current, err := h.lookup(ctx, s.name, s.ip)
		if err != nil || !sameSession(s.account, current) {
			return account.ErrChanged
		}
		return nil
	}
	r.routes[op.CMSG_CHAR_ENUM] = route{[]State{StateAuthed}, func(ctx context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		if len(p.Payload) != 0 {
			return nil, codec.ErrWorldPacket
		}
		if err := check(ctx, s); err != nil {
			return nil, err
		}
		return h.characterList(ctx, s.account, s.name, s.ip, s.log)
	}}
	r.routes[op.CMSG_CHAR_CREATE] = route{[]State{StateAuthed}, func(ctx context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		if err := check(ctx, s); err != nil {
			return nil, err
		}
		return h.createCharacter(ctx, s.account, s.ip, p, s.log)
	}}
	stateless := func(_ context.Context, _ *Session, p world.Packet) (*world.Packet, error) {
		return codec.CharacterScreenPacket(p)
	}
	r.routes[op.CMSG_CHAR_DELETE] = route{[]State{StateAuthed}, func(ctx context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		if err := check(ctx, s); err != nil {
			return nil, err
		}
		return h.deleteCharacter(ctx, s.account, p, s.log)
	}}
	r.routes[op.CMSG_REALM_SPLIT] = route{[]State{StateAuthed}, stateless}
	live := []State{StateAuthed, StateLoggingIn, StateInWorld, StateLoggingOut}
	r.routes[op.CMSG_VOICE_SESSION_ENABLE] = route{live, stateless}
	for _, opcode := range []uint32{op.CMSG_OPT_OUT_OF_LOOT, op.CMSG_SET_TAXI_BENCHMARK_MODE, op.CMSG_SET_ACTIVE_VOICE_CHANNEL} {
		r.routes[opcode] = route{live, stateless}
	}
	r.routes[op.CMSG_KEEP_ALIVE] = route{live, stateless}
	r.routes[op.CMSG_PING] = route{live, func(ctx context.Context, s *Session, p world.Packet) (*world.Packet, error) {
		if err := check(ctx, s); err != nil {
			return nil, err
		}
		return codec.CharacterScreenPacket(p)
	}}
	h.registerPlayerRoutes(r)
	h.registerChatRoutes(r)
	return r
}

// A missing feature is not malformed framing. Discard known client commands
// only in their reference-approved state, without executing game logic or
// pretending to have completed the operation. Registered handlers remain strict.
func (r *Router) unsupported(s *Session, p world.Packet) (*world.Packet, error) {
	m, known := op.Lookup(p.Opcode)
	if known && m.Status == op.STATUS_NEVER {
		// The reference never processes STATUS_NEVER opcodes; receiving one
		// (e.g. a second CMSG_AUTH_SESSION mid-session) is a protocol
		// violation that disconnects the client there.
		return nil, fmt.Errorf("%w: %s in state %d", codec.ErrFatalWorldPacket, m.Name, s.state)
	}
	inWorld := s.state == StateInWorld || s.state == StateLoggingOut
	live := s.state <= StateLoggingOut
	allowed := false
	if known && (strings.HasPrefix(m.Name, "CMSG_") || strings.HasPrefix(m.Name, "MSG_")) {
		switch m.Status {
		case op.STATUS_AUTHED:
			allowed = live
		case op.STATUS_LOGGEDIN, op.STATUS_LOGGEDIN_OR_RECENTLY_LOGGEDOUT:
			allowed = inWorld
		case op.STATUS_UNHANDLED:
			allowed = live
		}
	}
	if !allowed {
		return nil, fmt.Errorf("%w: unsupported or disallowed opcode=%d state=%d", codec.ErrWorldPacket, p.Opcode, s.state)
	}
	if s.unsupportedSeen == nil {
		s.unsupportedSeen = make(map[uint32]bool)
	}
	if !s.unsupportedSeen[p.Opcode] {
		s.unsupportedSeen[p.Opcode] = true
		if s.log != nil {
			s.log.Warn("world feature not implemented; packet ignored", "opcode", p.Opcode, "name", m.Name, "state", s.state, "bytes", len(p.Payload))
		}
	}
	return nil, nil
}
