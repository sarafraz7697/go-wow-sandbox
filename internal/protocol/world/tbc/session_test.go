package tbc

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"
	"wowserver/internal/account"
	"wowserver/internal/character"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

type worldStoreStub struct {
	mu      sync.Mutex
	a       account.WorldSession
	revoked bool
}

func (s *worldStoreStub) WorldSession(_ context.Context, name, ip string, realm uint32) (account.WorldSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revoked || name != "ADMIN" || ip != "127.0.0.1" || realm != 1 {
		return account.WorldSession{}, account.ErrChanged
	}
	return s.a, nil
}

type addressedPipe struct{ net.Conn }

// Phase-5 session tests keep using an account with no stored characters.
type emptyCharacterStore struct{}

func (emptyCharacterStore) List(context.Context, uint64) ([]character.Summary, error) {
	return nil, nil
}

func (c addressedPipe) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 50000}
}
func setupWorld(t *testing.T) (*Handler, *worldStoreStub) {
	t.Helper()
	s := &worldStoreStub{a: account.WorldSession{ID: 7, Name: "ADMIN", Expansion: 1}}
	for i := range s.a.Key {
		s.a.Key[i] = byte(i + 1)
	}
	h := &Handler{Store: s, Characters: emptyCharacterStore{}, Data: &gamedata.Catalog{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), RealmID: 1, SendQueue: 4, IdleTimeout: time.Second, RequestTimeout: time.Second}
	return h, s
}
func worldPipe(t *testing.T, h *Handler) (net.Conn, <-chan error, context.CancelFunc) {
	t.Helper()
	server, client := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.Serve(ctx, addressedPipe{server}) }()
	client.SetDeadline(time.Now().Add(4 * time.Second))
	t.Cleanup(func() { cancel(); client.Close() })
	return client, done, cancel
}

// Independent client-side cipher, never production Reader/Writer or deriveKey.
type worldClient struct {
	conn               net.Conn
	key                []byte
	send, recv         int
	sendPrev, recvPrev byte
}

func (c *worldClient) enable(key [40]byte) {
	n := 40
	for n > 0 && key[n-1] == 0 {
		n--
	}
	seed := []byte{56, 167, 131, 21, 248, 146, 37, 48, 113, 152, 103, 177, 140, 4, 226, 170}
	h := hmac.New(sha1.New, seed)
	h.Write(key[:n])
	c.key = h.Sum(nil)
}
func (c *worldClient) sendPacket(t *testing.T, p world.Packet) {
	t.Helper()
	b := clientFrame(p.Opcode, p.Payload)
	if c.key != nil {
		for i := 0; i < 6; i++ {
			b[i] = (b[i] ^ c.key[c.send%20]) + c.sendPrev
			c.sendPrev = b[i]
			c.send++
		}
	}
	if _, err := c.conn.Write(b); err != nil {
		t.Fatal(err)
	}
}
func (c *worldClient) readPacket(t *testing.T) world.Packet {
	t.Helper()
	var header [4]byte
	if _, e := io.ReadFull(c.conn, header[:]); e != nil {
		t.Fatal(e)
	}
	if c.key != nil {
		for i, v := range header {
			header[i] = (v - c.recvPrev) ^ c.key[c.recv%20]
			c.recvPrev = v
			c.recv++
		}
	}
	size := int(binary.BigEndian.Uint16(header[:2]))
	if size < 2 {
		t.Fatal("bad server length")
	}
	p := world.Packet{Opcode: uint32(binary.LittleEndian.Uint16(header[2:])), Payload: make([]byte, size-2)}
	if _, e := io.ReadFull(c.conn, p.Payload); e != nil {
		t.Fatal(e)
	}
	return p
}
func (c *worldClient) authenticate(t *testing.T, key [40]byte, build uint32, badDigest bool) world.Packet {
	t.Helper()
	challenge := c.readPacket(t)
	if challenge.Opcode != op.SMSG_AUTH_CHALLENGE || len(challenge.Payload) != 4 {
		t.Fatal("challenge")
	}
	seed := []byte{1, 2, 3, 4}
	h := sha1.New()
	h.Write([]byte("ADMIN"))
	h.Write(make([]byte, 4))
	h.Write(seed)
	h.Write(challenge.Payload)
	n := 40
	for n > 0 && key[n-1] == 0 {
		n--
	}
	h.Write(key[:n])
	digest := h.Sum(nil)
	if badDigest {
		digest[0] ^= 1
	}
	b := make([]byte, 8)
	binary.LittleEndian.PutUint32(b, build)
	b = append(b, []byte("ADMIN\x00")...)
	b = append(b, seed...)
	b = append(b, digest...)
	b = append(b, 0, 0, 0, 0)
	c.sendPacket(t, world.Packet{Opcode: op.CMSG_AUTH_SESSION, Payload: b})
	if build == 8606 && !badDigest {
		c.enable(key)
	}
	return c.readPacket(t)
}
func waitWorld(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("session goroutine did not stop")
	}
}

func TestWorldAuthenticationAndCharacterScreen(t *testing.T) {
	h, s := setupWorld(t)
	conn, done, cancel := worldPipe(t, h)
	c := worldClient{conn: conn}
	p := c.authenticate(t, s.a.Key, 8606, false)
	if p.Opcode != op.SMSG_AUTH_RESPONSE || !bytes.Equal(p.Payload, []byte{12, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}) {
		t.Fatal("AUTH_OK")
	}
	if p = c.readPacket(t); p.Opcode != op.SMSG_ADDON_INFO || len(p.Payload) != 0 {
		t.Fatal("addon response")
	}
	for i := 0; i < 6; i++ {
		c.sendPacket(t, world.Packet{Opcode: op.CMSG_CHAR_ENUM})
		p = c.readPacket(t)
		if p.Opcode != op.SMSG_CHAR_ENUM || !bytes.Equal(p.Payload, []byte{0}) {
			t.Fatal("empty list")
		}
		c.sendPacket(t, world.Packet{Opcode: op.CMSG_PING, Payload: []byte{byte(i), 2, 3, 4, 0, 0, 0, 0}})
		p = c.readPacket(t)
		if p.Opcode != op.SMSG_PONG || !bytes.Equal(p.Payload, []byte{byte(i), 2, 3, 4}) {
			t.Fatal("pong")
		}
	}
	cancel()
	waitWorld(t, done)
	if !h.claim(s.a.ID) {
		t.Fatal("admission leaked")
	}
	h.release(s.a.ID)
}
func TestWorldRejectsInvalidLogin(t *testing.T) {
	for _, tc := range []struct {
		build uint32
		bad   bool
		code  byte
	}{{8606, true, authFailed}, {12340, false, authVersionMismatch}} {
		h, s := setupWorld(t)
		conn, done, _ := worldPipe(t, h)
		c := worldClient{conn: conn}
		p := c.authenticate(t, s.a.Key, tc.build, tc.bad)
		if p.Opcode != op.SMSG_AUTH_RESPONSE || !bytes.Equal(p.Payload, []byte{tc.code}) {
			t.Fatal("auth failure")
		}
		waitWorld(t, done)
	}
}
func TestWorldDuplicateAndRevocation(t *testing.T) {
	h, s := setupWorld(t)
	conn, done, _ := worldPipe(t, h)
	c := worldClient{conn: conn}
	c.authenticate(t, s.a.Key, 8606, false)
	c.readPacket(t)
	second, secondDone, _ := worldPipe(t, h)
	d := worldClient{conn: second}
	p := d.authenticate(t, s.a.Key, 8606, false)
	if !bytes.Equal(p.Payload, []byte{authAlreadyOnline}) {
		t.Fatal("duplicate accepted")
	}
	waitWorld(t, secondDone)
	s.mu.Lock()
	s.revoked = true
	s.mu.Unlock()
	c.sendPacket(t, world.Packet{Opcode: op.CMSG_CHAR_ENUM})
	select {
	case err := <-done:
		if !errors.Is(err, account.ErrChanged) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("revoked session retained")
	}
}
func TestWorldOutOfStateAndSlowPacket(t *testing.T) {
	for _, slow := range []bool{false, true} {
		h, s := setupWorld(t)
		h.RequestTimeout = 100 * time.Millisecond
		conn, done, _ := worldPipe(t, h)
		c := worldClient{conn: conn}
		c.authenticate(t, s.a.Key, 8606, false)
		c.readPacket(t)
		if slow {
			if _, e := conn.Write([]byte{1}); e != nil {
				t.Fatal(e)
			}
		} else {
			c.sendPacket(t, world.Packet{Opcode: op.MSG_MOVE_HEARTBEAT})
		}
		waitWorld(t, done)
	}
}
