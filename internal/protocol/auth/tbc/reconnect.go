package tbc

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"errors"
	"io"
	"net"

	"wowserver/internal/domain/account"
	"wowserver/internal/protocol/auth"
	"wowserver/internal/protocol/auth/tbc/codec"
)

func (h Handler) reconnect(ctx context.Context, conn net.Conn, a account.Account, loginName string, build uint16, ip string, trace func(string, ...any)) error {
	if h.Sessions == nil {
		return errors.New("session store not configured")
	}
	key, err := h.Sessions.Session(ctx, a.ID, build, ip)
	if err != nil {
		return errors.New("no valid reconnect session")
	}
	var nonce [16]byte
	if _, err = io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return err
	}
	packet := codec.ReconnectChallenge(nonce)
	if err = auth.WriteAll(conn, packet); err != nil {
		return err
	}
	trace("reconnect challenge sent")
	var proof [58]byte
	if _, err = io.ReadFull(conn, proof[:1]); err != nil {
		return err
	}
	if proof[0] != 3 {
		return auth.ErrCommand
	}
	if _, err = io.ReadFull(conn, proof[1:]); err != nil {
		return err
	}
	decoded, err := codec.DecodeReconnectProof(proof[:])
	if err != nil {
		return err
	}
	digest := sha1.New()
	// The reconnect proof hashes the client-sent login name, like the
	// reference _login in _HandleReconnectProof.
	digest.Write([]byte(loginName))
	digest.Write(decoded.Nonce[:])
	digest.Write(nonce[:])
	digest.Write(key[:])
	if subtle.ConstantTimeCompare(digest.Sum(nil), decoded.Digest[:]) != 1 {
		return errors.New("invalid reconnect proof")
	}
	// A concurrent fresh login or account restriction invalidates this challenge.
	current, err := h.Sessions.Session(ctx, a.ID, build, ip)
	if err != nil || subtle.ConstantTimeCompare(current[:], key[:]) != 1 {
		return account.ErrChanged
	}
	if err = auth.WriteAll(conn, codec.ReconnectSuccess()); err != nil {
		return err
	}
	trace("reconnect proof verified")
	return h.authenticated(ctx, conn, a, build, key, trace)
}
