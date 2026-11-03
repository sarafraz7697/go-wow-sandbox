package tbc

import (
	"context"
	"errors"
	"io"
	"net"
	"time"

	"wowserver/internal/protocol/auth"
)

// writeRejection leaves the failed authentication connection alive briefly so
// the client can consume the protocol error before closing it. No further
// requests are authorized, and the transport's handshake timer remains active.
func writeRejection(ctx context.Context, conn net.Conn, packet []byte, trace func(string, ...any)) error {
	if err := auth.WriteAll(conn, packet); err != nil {
		return err
	}
	trace("authentication failure sent; waiting for client close", "bytes", len(packet))
	deadline := time.Now().Add(5 * time.Second)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return err
	}
	var next [1]byte
	n, err := conn.Read(next[:])
	if n != 0 {
		return auth.ErrCommand
	}
	if errors.Is(err, io.EOF) {
		return nil
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return nil
	}
	return err
}
