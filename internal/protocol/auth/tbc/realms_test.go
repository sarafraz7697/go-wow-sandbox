package tbc

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"net"
	"testing"
	"time"

	"wowserver/internal/account"
	"wowserver/internal/realm"
	"wowserver/internal/testclient"
)

func TestRealmWireGolden(t *testing.T) {
	r := realm.Realm{ID: 1, Name: "A", Host: "127.0.0.1", Port: 8085, Build: 8606, Type: 1, Locked: true, Flags: 4, Population: 0.5, Characters: 2, Category: 1}
	got, err := EncodeRealms([]realm.Realm{r})
	if err != nil {
		t.Fatal(err)
	}
	// Manually specified bytes from the 8606 LoadRealmlist field layout.
	want, _ := hex.DecodeString("10280000000000010001010441003132372e302e302e313a38303835000000003f02012c0204039e211000")
	if !bytes.Equal(got, want) {
		t.Fatalf("wire mismatch\ngot  %x\nwant %x", got, want)
	}
	got, err = EncodeRealms(nil)
	want, _ = hex.DecodeString("1008000000000000001000")
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("empty realm framing")
	}
}

func TestRealmBounds(t *testing.T) {
	base := realm.Realm{Name: "A", Host: "localhost", Port: 8085, Build: 8606}
	for _, mutate := range []func(*realm.Realm){func(r *realm.Realm) { r.Name = "A\x00B" }, func(r *realm.Realm) { r.Port = 0 }, func(r *realm.Realm) { r.Population = float32(math.NaN()) }, func(r *realm.Realm) { r.Build = 12340 }} {
		r := base
		mutate(&r)
		if _, err := EncodeRealms([]realm.Realm{r}); err == nil {
			t.Fatal("accepted malformed realm")
		}
	}
	big := make([]realm.Realm, 1025)
	if _, err := EncodeRealms(big); err == nil {
		t.Fatal("count overflow")
	}
	for i := range big[:1024] {
		big[i] = base
		big[i].Name = string(bytes.Repeat([]byte{'A'}, 128))
	}
	if _, err := EncodeRealms(big[:1024]); err == nil {
		t.Fatal("length overflow")
	}
}

type realmMemory struct {
	list      []realm.Realm
	err       error
	calls     int
	accountID uint64
	build     uint16
}

func (r *realmMemory) List(_ context.Context, id uint64, build uint16) ([]realm.Realm, error) {
	r.calls++
	r.accountID = id
	r.build = build
	return r.list, r.err
}

type sessionMemory struct {
	key [40]byte
	err error
}

func (s *sessionMemory) Session(context.Context, uint64, uint16, string) ([40]byte, error) {
	return s.key, s.err
}

func phase3Connection(t *testing.T, h Handler) (net.Conn, func() error) {
	t.Helper()
	server, c := net.Pipe()
	done := make(chan error, 1)
	_ = server.SetDeadline(time.Now().Add(3 * time.Second))
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	go func() { defer server.Close(); done <- h.Serve(context.Background(), peer{server}) }()
	t.Cleanup(func() { c.Close(); server.Close() })
	return c, func() error {
		c.Close()
		select {
		case err := <-done:
			return err
		case <-time.After(4 * time.Second):
			t.Fatal("handler did not exit")
			return nil
		}
	}
}
func login(t *testing.T, c net.Conn) [40]byte {
	t.Helper()
	write(t, c, testclient.Challenge("TEST", 8606))
	challenge := read(t, c, 119)
	p, key, _ := testclient.Proof("TEST", "PASSWORD", challenge)
	write(t, c, p)
	if got := read(t, c, 32); got[1] != 0 {
		t.Fatal("login failed")
	}
	return key
}
func realmReply(t *testing.T, c net.Conn) []byte {
	t.Helper()
	header := read(t, c, 3)
	if header[0] != 0x10 {
		t.Fatal("wrong opcode")
	}
	return append(header, read(t, c, int(binary.LittleEndian.Uint16(header[1:])))...)
}

func TestRepeatedRealmRequests(t *testing.T) {
	r := &realmMemory{list: []realm.Realm{{ID: 9, Name: "Go TBC", Host: "127.0.0.1", Port: 8085, Build: 8606, Flags: 2}}}
	c, finish := phase3Connection(t, Handler{Store: fixtureStore(t), Realms: r})
	login(t, c)
	// Fragmented request, followed by two coalesced requests.
	for _, b := range []byte{0x10, 0, 0, 0, 0} {
		write(t, c, []byte{b})
	}
	want, _ := EncodeRealms(r.list)
	if !bytes.Equal(realmReply(t, c), want) {
		t.Fatal("realm data")
	}
	r.list[0].Name = "Reloaded"
	want, _ = EncodeRealms(r.list)
	sent := make(chan struct{})
	go func() { defer close(sent); _, _ = c.Write([]byte{0x10, 0, 0, 0, 0, 0x10, 0, 0, 0, 0}) }()
	for range 2 {
		if !bytes.Equal(realmReply(t, c), want) {
			t.Fatal("refresh failed")
		}
	}
	<-sent
	if err := finish(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	if r.calls != 3 || r.accountID != 1 || r.build != 8606 {
		t.Fatal("incorrect account/build routing")
	}
}

func TestRealmStateAndFailures(t *testing.T) {
	t.Run("unauthenticated", func(t *testing.T) {
		r := &realmMemory{}
		c, finish := phase3Connection(t, Handler{Store: fixtureStore(t), Realms: r})
		write(t, c, []byte{0x10})
		_, _ = io.ReadAll(c)
		if err := finish(); !errors.Is(err, io.EOF) && err == nil {
			t.Fatal("request accepted")
		}
		if r.calls != 0 {
			t.Fatal("queried unauthenticated realm list")
		}
	})
	for _, mode := range []string{"wrong command", "database", "expired session", "partial", "idle"} {
		t.Run(mode, func(t *testing.T) {
			r := &realmMemory{}
			h := Handler{Store: fixtureStore(t), Realms: r, IdleTimeout: 50 * time.Millisecond, RequestTimeout: 50 * time.Millisecond}
			if mode == "database" {
				r.err = errors.New("offline")
			}
			if mode == "expired session" {
				h.Sessions = &sessionMemory{err: account.ErrChanged}
			}
			c, finish := phase3Connection(t, h)
			login(t, c)
			switch mode {
			case "idle":
			case "partial":
				write(t, c, []byte{0x10, 0})
			case "wrong command":
				write(t, c, []byte{1})
			default:
				write(t, c, []byte{0x10, 0, 0, 0, 0})
			}
			b, err := io.ReadAll(c)
			if err != nil {
				t.Fatal("client timed out", err)
			}
			if len(b) != 0 {
				t.Fatal("unexpected response")
			}
			if err := finish(); err == nil {
				t.Fatal("failure not reported")
			}
		})
	}
}

func TestReconnect(t *testing.T) {
	for _, mode := range []string{"success", "wrong proof", "rotated key", "missing key", "wrong command"} {
		t.Run(mode, func(t *testing.T) {
			sessions := &sessionMemory{key: [40]byte{1, 2, 3}}
			if mode == "missing key" {
				sessions.err = account.ErrChanged
			}
			r := &realmMemory{}
			c, finish := phase3Connection(t, Handler{Store: fixtureStore(t), Realms: r, Sessions: sessions})
			challenge := testclient.Challenge("TEST", 8606)
			challenge[0] = 2
			write(t, c, challenge)
			if mode == "missing key" {
				_, _ = io.ReadAll(c)
				if finish() == nil {
					t.Fatal("missing key accepted")
				}
				return
			}
			response := read(t, c, 34)
			if response[0] != 2 || response[1] != 0 || !bytes.Equal(response[18:], make([]byte, 16)) {
				t.Fatal("bad reconnect challenge")
			}
			proof := make([]byte, 58)
			proof[0] = 3
			proof[1] = 42
			h := sha1.New()
			h.Write([]byte("TEST"))
			h.Write(proof[1:17])
			h.Write(response[2:18])
			h.Write(sessions.key[:])
			copy(proof[17:37], h.Sum(nil))
			if mode == "wrong proof" {
				proof[17] ^= 1
			}
			if mode == "rotated key" {
				sessions.key[0] ^= 1
			}
			if mode == "wrong command" {
				proof = []byte{0x10}
			}
			write(t, c, proof)
			if mode != "success" {
				_, _ = io.ReadAll(c)
				if finish() == nil {
					t.Fatal("invalid reconnect accepted")
				}
				return
			}
			if got := read(t, c, 4); !bytes.Equal(got, []byte{3, 0, 0, 0}) {
				t.Fatal("bad reconnect response")
			}
			write(t, c, []byte{0x10, 0, 0, 0, 0})
			want, _ := EncodeRealms(nil)
			if !bytes.Equal(realmReply(t, c), want) {
				t.Fatal("reconnect realm list")
			}
			if err := finish(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
				t.Fatal(err)
			}
		})
	}
}
