package tbc

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"wowserver/internal/domain/account"
	"wowserver/internal/auth/srp"
	"wowserver/internal/testclient"
)

type memoryStore struct {
	a                     account.Account
	lookupErr, publishErr error
	key                   [40]byte
	published, failed     int
}

func (s *memoryStore) Find(context.Context, string) (account.Account, error) { return s.a, s.lookupErr }
func (s *memoryStore) Publish(_ context.Context, _ account.Account, k [40]byte, _ uint16, _ string) error {
	if s.publishErr != nil {
		return s.publishErr
	}
	s.key = k
	s.published++
	return nil
}
func (s *memoryStore) Failed(context.Context, uint64) error { s.failed++; return nil }

type peer struct{ net.Conn }

func (peer) RemoteAddr() net.Addr { return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1000} }
func connect(t *testing.T, s Store) (net.Conn, func()) {
	return connectLogged(t, s, nil)
}
func connectLogged(t *testing.T, s Store, logger *slog.Logger) (net.Conn, func()) {
	t.Helper()
	server, client := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	_ = server.SetDeadline(time.Now().Add(3 * time.Second))
	go func() {
		defer close(done)
		defer server.Close()
		_ = (Handler{Store: s, Logger: logger}).Serve(ctx, peer{server})
	}()
	finish := func() { cancel(); client.Close(); server.Close(); <-done }
	t.Cleanup(finish)
	return client, func() { client.Close(); <-done }
}

func TestDebugTraceAndFiltering(t *testing.T) {
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo} {
		t.Run(level.String(), func(t *testing.T) {
			var output bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: level}))
			s := fixtureStore(t)
			c, wait := connectLogged(t, s, logger)
			write(t, c, testclient.Challenge("TEST", 8606))
			challenge := read(t, c, 119)
			p, key, _ := testclient.Proof("TEST", "PASSWORD", challenge)
			write(t, c, p)
			read(t, c, 32)
			wait()
			logs := output.String()
			for _, secret := range []string{"PASSWORD", "TEST", hex.EncodeToString(key[:]), hex.EncodeToString(p[33:53])} {
				if strings.Contains(logs, secret) {
					t.Fatal("sensitive data in logs")
				}
			}
			if !strings.Contains(logs, "authentication succeeded") {
				t.Fatal("success missing")
			}
			for _, step := range []string{"waiting for login challenge", "challenge header received", "login challenge decoded", "looking up account", "server challenge sent", "client proof verified", "session key saved", "server proof sent", "authenticated; waiting for realm list"} {
				if strings.Contains(logs, step) != (level == slog.LevelDebug) {
					t.Fatalf("incorrect filtering for %s", step)
				}
			}
		})
	}
}

func TestTraceMalformedChallenge(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	c, wait := connectLogged(t, fixtureStore(t), logger)
	p := testclient.Challenge("TEST", 8606)
	p[4] = 'X'
	write(t, c, p)
	_, _ = io.ReadAll(c)
	wait()
	if !strings.Contains(output.String(), "authentication interrupted") || !strings.Contains(output.String(), "stage=challenge") {
		t.Fatal("missing failure location")
	}
}
func fixtureStore(t *testing.T) *memoryStore {
	t.Helper()
	s, v, err := srp.Credentials("TEST", "PASSWORD")
	if err != nil {
		t.Fatal(err)
	}
	return &memoryStore{a: account.Account{ID: 1, Name: "TEST", Salt: s, Verifier: v}}
}
func read(t *testing.T, c net.Conn, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := io.ReadFull(c, b); err != nil {
		t.Fatal(err)
	}
	return b
}
func write(t *testing.T, c net.Conn, b []byte) {
	t.Helper()
	if _, err := c.Write(b); err != nil {
		t.Fatal(err)
	}
}

func TestLogin(t *testing.T) {
	for _, tc := range []struct {
		name, password string
		dbFailure      bool
	}{{"success", "PASSWORD", false}, {"wrong password", "WRONG", false}, {"database failure", "PASSWORD", true}} {
		t.Run(tc.name, func(t *testing.T) {
			s := fixtureStore(t)
			if tc.dbFailure {
				s.publishErr = errors.New("offline")
			}
			c, wait := connect(t, s)
			write(t, c, testclient.Challenge("TEST", 8606))
			challenge := read(t, c, 119)
			if !bytes.Equal(challenge[:3], []byte{0, 0, 0}) || challenge[118] != 0 {
				t.Fatal("invalid challenge")
			}
			p, key, m2 := testclient.Proof("TEST", tc.password, challenge)
			write(t, c, p)
			if tc.password != "PASSWORD" || tc.dbFailure {
				response := read(t, c, 4)
				want := byte(4)
				if tc.dbFailure {
					want = 8
				}
				if !bytes.Equal(response, []byte{1, want, 3, 0}) {
					t.Fatal(response)
				}
				wait()
				if s.published != 0 {
					t.Fatal("published failed login")
				}
				if tc.password != "PASSWORD" && s.failed != 1 {
					t.Fatal("missing failed counter")
				}
			} else {
				response := read(t, c, 32)
				wait()
				if response[0] != 1 || response[1] != 0 || !bytes.Equal(response[2:22], m2[:]) || s.key != key || s.published != 1 {
					t.Fatal("authentication mismatch")
				}
				if !bytes.Equal(response[22:], []byte{0, 0, 128, 0, 0, 0, 0, 0, 0, 0}) {
					t.Fatal("incorrect account flags")
				}
			}
		})
	}
}
func TestChallengeRejections(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build uint16
		code  byte
		setup func(*memoryStore)
	}{
		{"unknown", 8606, 4, func(s *memoryStore) { s.lookupErr = account.ErrNotFound }},
		{"database", 8606, 8, func(s *memoryStore) { s.lookupErr = errors.New("offline") }},
		{"banned", 8606, 3, func(s *memoryStore) { s.a.Banned = true }},
		{"suspended", 8606, 12, func(s *memoryStore) { s.a.Suspended = true }},
		{"locked", 8606, 16, func(s *memoryStore) { s.a.LockedIP = "192.0.2.1" }},
		{"build", 12340, 9, func(*memoryStore) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := fixtureStore(t)
			tc.setup(s)
			c, _ := connect(t, s)
			write(t, c, testclient.Challenge("TEST", tc.build))
			if b := read(t, c, 3); !bytes.Equal(b, []byte{0, 0, tc.code}) {
				t.Fatal(b)
			}
		})
	}
}
func TestProofStateRejectsUnexpectedInput(t *testing.T) {
	for _, mode := range []string{"command", "flags", "zero A", "truncated"} {
		t.Run(mode, func(t *testing.T) {
			s := fixtureStore(t)
			c, wait := connect(t, s)
			write(t, c, testclient.Challenge("TEST", 8606))
			challenge := read(t, c, 119)
			p, _, _ := testclient.Proof("TEST", "PASSWORD", challenge)
			switch mode {
			case "command":
				p = []byte{0x10}
			case "flags":
				p[74] = 1
			case "zero A":
				clear(p[1:33])
			case "truncated":
				p = p[:20]
			}
			write(t, c, p)
			if mode == "truncated" {
				c.Close()
			} else {
				_, _ = io.ReadAll(c)
			}
			wait()
			if s.published != 0 {
				t.Fatal("invalid input authenticated")
			}
		})
	}
}
