package tbc

import (
	"context"
	"errors"
	"io"
	"time"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

type received struct {
	packet world.Packet
	err    error
}

// run is the only consumer allowed to modify session state.
func (s *Session) run(ctx context.Context, router *Router, incoming <-chan received, queue chan<- world.Packet) error {
	timer := time.NewTicker(250 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			if s.onTick != nil {
				if err := s.onTick(ctx, s); err != nil {
					return err
				}
			}
		case <-ctx.Done():
			return ctx.Err()
		case event := <-s.events:
			event.result <- s.transition(event.next)
			if s.state == StateClosed {
				return nil
			}
		case result := <-incoming:
			if errors.Is(result.err, io.EOF) {
				return nil
			}
			if result.err != nil {
				return result.err
			}
			p := result.packet
			meta, _ := op.Lookup(p.Opcode)
			s.log.Debug("world packet received", "opcode", p.Opcode, "name", meta.Name, "bytes", len(p.Payload), "state", s.state)
			response, err := router.dispatch(ctx, s, p)
			if err != nil {
				return err
			}
			if response == nil {
				continue
			}
			select {
			case queue <- *response:
			case <-ctx.Done():
				return ctx.Err()
			default:
				return errors.New("world send queue full")
			}
		}
	}
}
