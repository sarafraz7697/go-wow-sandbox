package transport

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestShutdownClosesBlockedConnection(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	s := Server{MaxConnections: 1, SessionTimeout: time.Minute, Handle: func(_ context.Context, c net.Conn) error {
		close(started)
		_, err := io.Copy(io.Discard, c)
		return err
	}}
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, ln) }()
	c, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not start")
	}
	// A full server rejects the next connection, without spawning a handler.
	extra, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer extra.Close()
	_ = extra.SetReadDeadline(time.Now().Add(time.Second))
	var b [1]byte
	if _, err := extra.Read(b[:]); err == nil {
		t.Fatal("excess connection not closed")
	} else if e, ok := err.(net.Error); ok && e.Timeout() {
		t.Fatal("excess connection timed out instead of closing")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown hung")
	}
}

func TestAbsoluteDeadline(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	s := Server{MaxConnections: 1, SessionTimeout: 50 * time.Millisecond, Handle: func(_ context.Context, c net.Conn) error { _, err := io.Copy(io.Discard, c); return err }}
	go func() { done <- s.Serve(ctx, ln) }()
	defer func() { cancel(); <-done }()
	c, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	var b [1]byte
	if _, err := c.Read(b[:]); err == nil {
		t.Fatal("expected close")
	} else if e, ok := err.(net.Error); ok && e.Timeout() {
		t.Fatal("server failed to expire connection")
	}
}

func TestAuthenticatedConnectionSurvivesHandshakeTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan error, 1)
	server := Server{MaxConnections: 1, SessionTimeout: 100 * time.Millisecond, Handle: func(ctx context.Context, c net.Conn) error {
		err := CompleteHandshake(ctx)
		ready <- err
		if err != nil {
			return err
		}
		_, err = io.Copy(c, c)
		return err
	}}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, ln) }()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err = <-ready; err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	_ = c.SetDeadline(time.Now().Add(time.Second))
	if _, err = c.Write([]byte{42}); err != nil {
		t.Fatal(err)
	}
	var b [1]byte
	if _, err = io.ReadFull(c, b[:]); err != nil || b[0] != 42 {
		t.Fatal("handshake timer still active", err)
	}
	cancel()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("authenticated shutdown hung")
	}
}
