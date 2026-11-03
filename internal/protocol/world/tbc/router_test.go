package tbc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

func TestSessionTransitions(t *testing.T) {
	allowed := map[[2]State]bool{{StateAuthed, StateLoggingIn}: true, {StateLoggingIn, StateAuthed}: true, {StateLoggingIn, StateInWorld}: true, {StateInWorld, StateLoggingOut}: true, {StateLoggingOut, StateAuthed}: true, {StateLoggingOut, StateInWorld}: true}
	for from := StateAuthed; from <= StateClosed; from++ {
		for to := StateAuthed; to <= StateClosed; to++ {
			s := Session{state: from}
			err := s.transition(to)
			want := allowed[[2]State{from, to}] || (from != StateClosed && to == StateClosed)
			if (err == nil) != want {
				t.Fatalf("%d -> %d: %v", from, to, err)
			}
			if err != nil && s.state != from {
				t.Fatal("rejected transition changed state")
			}
		}
	}
}
func TestRouterRejectsBeforeCallingHandler(t *testing.T) {
	h, _ := setupWorld(t)
	r := h.newRouter()
	for _, state := range []State{StateLoggingIn, StateInWorld, StateLoggingOut, StateClosed} {
		for _, opcode := range []uint32{op.CMSG_CHAR_ENUM, op.CMSG_CHAR_CREATE, op.CMSG_PLAYER_LOGIN, op.CMSG_CHAR_DELETE} {
			if _, err := r.dispatch(context.Background(), &Session{state: state}, world.Packet{Opcode: opcode}); !errors.Is(err, codec.ErrWorldPacket) {
				t.Fatalf("state %d opcode %d: %v", state, opcode, err)
			}
		}
	}
	if _, err := r.dispatch(context.Background(), &Session{state: StateAuthed}, world.Packet{Opcode: op.CMSG_AUTH_SESSION}); err == nil {
		t.Fatal("handshake routed after auth")
	}
}
func TestSessionAcceptsEventsWhileReaderIdle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s := &Session{state: StateAuthed, events: make(chan transition, 8), done: ctx.Done(), log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	incoming := make(chan received)
	queue := make(chan world.Packet, 1)
	done := make(chan error, 1)
	go func() { done <- s.run(ctx, &Router{routes: map[uint32]route{}}, incoming, queue) }()
	// No inbound packet is available; the event loop must still acknowledge.
	for _, next := range []State{StateLoggingIn, StateInWorld, StateLoggingOut, StateAuthed} {
		if err := s.RequestTransition(ctx, next); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RequestTransition(ctx, StateInWorld); err == nil {
		t.Fatal("skipped login accepted")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("loop did not stop")
	}
}
