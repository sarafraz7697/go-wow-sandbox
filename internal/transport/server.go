// Package transport owns TCP lifetimes, not game protocol or authentication state.
package transport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

type Handler func(context.Context, net.Conn) error

type handshakeKey struct{}

// CompleteHandshake stops the pre-authentication deadline. The handler must
// install its authenticated idle/request deadlines; shutdown still cancels I/O.
// Called once by the connection's handler goroutine.
func CompleteHandshake(ctx context.Context) error {
	release, ok := ctx.Value(handshakeKey{}).(func() error)
	if !ok {
		return errors.New("missing transport handshake lifecycle")
	}
	return release()
}

type Server struct {
	MaxConnections int
	SessionTimeout time.Duration
	Logger         *slog.Logger
	Handle         Handler
}

// Serve owns ln and all accepted connections. Cancellation closes blocked I/O
// and waits for handlers to return. Handlers must honor context cancellation.
func (s Server) Serve(ctx context.Context, ln net.Listener) error {
	defer ln.Close()
	if s.MaxConnections < 1 || s.SessionTimeout <= 0 || s.Handle == nil {
		return errors.New("invalid transport configuration")
	}
	logger := s.Logger
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	stop := context.AfterFunc(ctx, func() { _ = ln.Close() })
	defer func() { cancel(); stop(); wg.Wait() }()
	slots := make(chan struct{}, s.MaxConnections)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		select {
		case slots <- struct{}{}:
		default:
			logger.Debug("connection rejected", "peer", conn.RemoteAddr().String(), "reason", "connection limit")
			_ = conn.Close()
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			defer conn.Close()
			log := logger.With("peer", conn.RemoteAddr().String())
			log.Debug("connection accepted")
			defer log.Debug("connection closed")
			session, end := context.WithCancel(ctx)
			defer end()
			timer := time.AfterFunc(s.SessionTimeout, end)
			defer timer.Stop()
			session = context.WithValue(session, handshakeKey{}, func() error {
				if !timer.Stop() {
					return context.DeadlineExceeded
				}
				if err := session.Err(); err != nil {
					return err
				}
				return conn.SetDeadline(time.Time{})
			})
			closeOnCancel := context.AfterFunc(session, func() { _ = conn.Close() })
			defer closeOnCancel()
			deadline := time.Now().Add(s.SessionTimeout)
			if err := conn.SetDeadline(deadline); err != nil {
				return
			}
			err := s.Handle(session, conn)
			if session.Err() != nil {
				log.Debug("connection interrupted", "reason", session.Err())
			} else if err != nil {
				log.Debug("connection ended", "error", err)
			}
		}()
	}
}
