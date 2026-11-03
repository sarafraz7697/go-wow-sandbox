// Package tbc owns build 8606 login responses. Other builds need their own
// adapters; the account store and TCP transport remain version-independent.
package tbc

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net"
	"time"

	"wowserver/internal/auth/srp"
	"wowserver/internal/domain/account"
	"wowserver/internal/protocol/auth"
	"wowserver/internal/protocol/auth/tbc/codec"
	"wowserver/internal/protocol/client"
)

type Handler struct {
	Store                       Store
	Logger                      *slog.Logger
	Realms                      RealmStore
	Sessions                    SessionStore
	IdleTimeout, RequestTimeout time.Duration
	OnAuthenticated             func(context.Context) error
}

func (h Handler) Serve(ctx context.Context, conn net.Conn) (result error) {
	stage := "challenge"
	trace := func(message string, args ...any) {
		if h.Logger != nil {
			h.Logger.Debug(message, append([]any{"peer", conn.RemoteAddr().String(), "stage", stage}, args...)...)
		}
	}
	defer func() {
		if result != nil {
			trace("authentication interrupted", "error", result)
		}
	}()
	trace("waiting for login challenge")
	c, err := auth.ReadChallengeTrace(conn, 285, trace)
	if err != nil {
		return err
	}
	trace("login challenge decoded", "build", c.Build, "version", c.Version)
	fail := func(code byte) error {
		trace("authentication rejected", "result_code", code)
		return writeRejection(ctx, conn, codec.ChallengeFailure(c.Command, code), trace)
	}
	if _, ok := client.Lookup(c.Build, c.Version); !ok {
		return fail(9)
	}
	ip, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		return err
	}
	stage = "account_lookup"
	a, code := h.lookupAccount(ctx, c.Account, ip, trace)
	if code != 0 {
		return fail(code)
	}
	trace("account found", "account_id", a.ID)
	if c.Command == 2 {
		stage = "reconnect"
		return h.reconnect(ctx, conn, a, c.Build, ip, trace)
	}
	return h.login(ctx, conn, a, c.Build, ip, &stage, trace)
}

func (h Handler) login(ctx context.Context, conn net.Conn, a account.Account, build uint16, ip string, stage *string, trace func(string, ...any)) error {
	*stage = "srp_challenge"
	exchange, err := srp.New(a.Name, a.Salt, a.Verifier)
	if err != nil {
		return writeRejection(ctx, conn, codec.ChallengeFailure(0, 8), trace)
	}
	var nonce [16]byte
	if _, err = io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return err
	}
	packet := codec.LoginChallenge(exchange.Public, a.Salt, srp.Modulus(), nonce)
	if err = auth.WriteAll(conn, packet); err != nil {
		return err
	}
	trace("server challenge sent", "bytes", len(packet))
	*stage = "client_proof"
	trace("waiting for client proof")
	decoded, err := readLoginProof(conn, trace)
	if err != nil {
		return err
	}
	key, m2, err := exchange.Verify(decoded.Public, decoded.Digest)
	if err != nil {
		trace("client proof rejected", "reason", "invalid public value or proof")
		if err = h.Store.Failed(ctx, a.ID); err != nil {
			return writeRejection(ctx, conn, codec.ProofFailure(8), trace)
		}
		return writeRejection(ctx, conn, codec.ProofFailure(4), trace)
	}
	trace("client proof verified")
	*stage = "session_publish"
	trace("saving session key")
	if err = h.Store.Publish(ctx, a, key, build, ip); err != nil {
		trace("session publication failed", "reason", "database write failed or account changed")
		return writeRejection(ctx, conn, codec.ProofFailure(8), trace)
	}
	trace("session key saved")
	*stage = "server_proof"
	response := codec.LoginSuccess(m2)
	if err := auth.WriteAll(conn, response); err != nil {
		return err
	}
	trace("server proof sent", "bytes", len(response))
	if h.Logger != nil {
		h.Logger.Info("authentication succeeded", "peer", conn.RemoteAddr().String(), "account_id", a.ID, "build", build)
	}
	*stage = "realm_list"
	return h.authenticated(ctx, conn, a, build, key, trace)
}

func readLoginProof(conn io.Reader, trace func(string, ...any)) (codec.LoginProof, error) {
	// Read command first: a realm request or second challenge is not a proof.
	var proof [75]byte
	if _, err := io.ReadFull(conn, proof[:1]); err != nil {
		return codec.LoginProof{}, err
	}
	trace("proof command received", "opcode", proof[0])
	if proof[0] != 1 {
		return codec.LoginProof{}, auth.ErrCommand
	}
	if _, err := io.ReadFull(conn, proof[1:]); err != nil {
		return codec.LoginProof{}, err
	}
	trace("client proof received", "bytes", len(proof), "key_count", proof[73], "security_flags", proof[74])
	decoded, err := codec.DecodeLoginProof(proof[:])
	if err != nil {
		return codec.LoginProof{}, err
	}
	return decoded, nil
}
