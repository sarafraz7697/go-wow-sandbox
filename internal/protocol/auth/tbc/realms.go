package tbc

import (
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"net"
	"time"

	"wowserver/internal/account"
	"wowserver/internal/protocol/auth"
	"wowserver/internal/protocol/auth/tbc/codec"
	"wowserver/internal/realm"
)

// EncodeRealms retains the public entry point for existing callers.
func EncodeRealms(realms []realm.Realm) ([]byte, error) { return codec.EncodeRealms(realms) }

func (h Handler) authenticated(ctx context.Context, conn net.Conn, a account.Account, build uint16, key [40]byte, trace func(string, ...any)) error {
	if h.OnAuthenticated != nil {
		if err := h.OnAuthenticated(ctx); err != nil {
			return err
		}
	}
	idle := h.IdleTimeout
	if idle <= 0 {
		idle = 2 * time.Minute
	}
	request := h.RequestTimeout
	if request <= 0 {
		request = 10 * time.Second
	}
	trace("authenticated; waiting for realm list")
	for {
		if err := conn.SetDeadline(time.Now().Add(idle)); err != nil {
			return err
		}
		var opcode [1]byte
		if _, err := io.ReadFull(conn, opcode[:]); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		trace("authenticated command received", "opcode", opcode[0])
		if opcode[0] != 0x10 {
			return auth.ErrCommand
		}
		deadline := time.Now().Add(request)
		if err := conn.SetDeadline(deadline); err != nil {
			return err
		}
		var unused [4]byte
		if _, err := io.ReadFull(conn, unused[:]); err != nil {
			return err
		}
		requestCtx, cancel := context.WithDeadline(ctx, deadline)
		list, err := h.loadRealms(requestCtx, a.ID, build, key, conn)
		cancel()
		if err != nil {
			return err
		}
		packet, err := EncodeRealms(list)
		if err != nil {
			return err
		}
		if err = auth.WriteAll(conn, packet); err != nil {
			return err
		}
		trace("realm list sent", "realms", len(list), "bytes", len(packet))
	}
}

func (h Handler) loadRealms(ctx context.Context, id uint64, build uint16, key [40]byte, conn net.Conn) ([]realm.Realm, error) {
	if h.Realms == nil {
		return nil, errors.New("realm store not configured")
	}
	if h.Sessions != nil {
		ip, _, err := net.SplitHostPort(conn.RemoteAddr().String())
		if err != nil {
			return nil, err
		}
		current, err := h.Sessions.Session(ctx, id, build, ip)
		if err != nil || subtle.ConstantTimeCompare(current[:], key[:]) != 1 {
			return nil, account.ErrChanged
		}
	}
	var list []realm.Realm
	var err error
	if routed, ok := h.Realms.(RoutedRealmStore); ok {
		ip, _, e := net.SplitHostPort(conn.RemoteAddr().String())
		if e != nil {
			return nil, e
		}
		list, err = routed.ListForClient(ctx, id, build, ip)
	} else {
		list, err = h.Realms.List(ctx, id, build)
	}
	if err != nil {
		return nil, errors.New("cannot load realms; run account check and check database")
	}
	return list, nil
}
