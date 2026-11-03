package tbc

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"
	"wowserver/internal/account"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

type WorldSessionStore interface {
	WorldSession(context.Context, string, string, uint32) (account.WorldSession, error)
}

// Handler must be shared by all connections of this realm and never copied.
// It owns a process-local admission registry; it does not model a game world.
type Handler struct {
	Creator                     CharacterCreator
	Creation                    CreationProfile
	Characters                  CharacterStore
	Data                        *gamedata.Catalog // Immutable startup snapshot, used by later character phases.
	Store                       WorldSessionStore
	Logger                      *slog.Logger
	RealmID                     uint32
	SendQueue                   int
	IdleTimeout, RequestTimeout time.Duration
	OnAuthenticated             func(context.Context) error
	mu                          sync.Mutex
	active                      map[uint64]bool
}

func (h *Handler) claim(id uint64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active == nil {
		h.active = make(map[uint64]bool)
	}
	if h.active[id] {
		return false
	}
	h.active[id] = true
	return true
}
func (h *Handler) release(id uint64) { h.mu.Lock(); delete(h.active, id); h.mu.Unlock() }

func (h *Handler) lookup(ctx context.Context, name, ip string) (account.WorldSession, error) {
	ctx, cancel := context.WithTimeout(ctx, h.RequestTimeout)
	defer cancel()
	return h.Store.WorldSession(ctx, name, ip, h.RealmID)
}
func (h *Handler) Serve(ctx context.Context, conn net.Conn) error {
	if h.Store == nil || h.RealmID == 0 || h.SendQueue < 1 || h.SendQueue > 1024 || h.IdleTimeout <= 0 || h.RequestTimeout <= 0 {
		return errors.New("invalid world handler configuration")
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	defer conn.Close()
	log := h.Logger
	if log == nil {
		log = slog.Default()
	}
	log = log.With("peer", conn.RemoteAddr().String(), "realm_id", h.RealmID)
	ip, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		return err
	}
	source := &deadlineSource{conn: conn, timeout: h.RequestTimeout}
	reader, writer := NewReader(source), NewWriter(conn)
	var seed [4]byte
	if _, err = rand.Read(seed[:]); err != nil {
		return err
	}
	if err = writer.WritePacket(world.Packet{Opcode: op.SMSG_AUTH_CHALLENGE, Payload: seed[:]}); err != nil {
		return err
	}
	log.Debug("world challenge sent", "stage", "auth_challenge")
	p, err := reader.ReadPacket()
	if err != nil {
		return err
	}
	if p.Opcode != op.CMSG_AUTH_SESSION {
		return errWorldPacket
	}
	request, err := decodeAuthRequest(p.Payload)
	if err != nil {
		return err
	}
	log.Debug("world auth request decoded", "stage", "auth_session", "build", request.Build, "addon_bytes", len(request.Addons))
	fail := func(code byte) error {
		log.Debug("world authentication rejected", "result_code", code)
		if err := writer.WritePacket(authResponse(code, 0)); err != nil {
			return err
		}
		return errWorldAuth
	}
	if request.Build != 8606 {
		return fail(authVersionMismatch)
	}
	name, _ := account.Normalize(request.Name)
	a, err := h.lookup(ctx, name, ip)
	if err != nil {
		return fail(authFailed)
	}
	if !verifyWorldDigest(request, binary.LittleEndian.Uint32(seed[:]), a.Key) {
		return fail(authFailed)
	}
	log.Debug("world proof verified", "account_id", a.ID)
	// Both directions switch only after digest verification. Rejections after
	// this point and the first AUTH_OK are encrypted, matching the reference.
	if err = reader.EnableEncryption(a.Key); err != nil {
		return err
	}
	if err = writer.EnableEncryption(a.Key); err != nil {
		return err
	}
	addons, err := addonResponse(request.Addons)
	if err != nil {
		return fail(authFailed)
	}
	current, err := h.lookup(ctx, name, ip)
	if err != nil || !sameSession(a, current) {
		return fail(authFailed)
	}
	if !h.claim(a.ID) {
		return fail(authAlreadyOnline)
	}
	defer h.release(a.ID)
	if h.OnAuthenticated != nil {
		if err = h.OnAuthenticated(ctx); err != nil {
			return err
		}
	}
	if err = conn.SetWriteDeadline(time.Now().Add(h.RequestTimeout)); err != nil {
		return err
	}
	if err = writer.WritePacket(authResponse(authOK, a.Expansion)); err != nil {
		return err
	}
	if err = writer.WritePacket(world.Packet{Opcode: op.SMSG_ADDON_INFO, Payload: addons}); err != nil {
		return err
	}
	log.Info("world authentication succeeded", "account_id", a.ID, "build", request.Build)
	return h.runSession(ctx, conn, reader, writer, source, a, name, ip, log)
}
func sameSession(a, b account.WorldSession) bool {
	return a.ID == b.ID && a.Expansion == b.Expansion && subtle.ConstantTimeCompare(a.Key[:], b.Key[:]) == 1
}

// The first byte waits for the idle deadline. Once it arrives the rest of the
// packet gets an absolute request deadline, so byte trickling cannot keep it alive.
type deadlineSource struct {
	conn    net.Conn
	timeout time.Duration
	armed   bool
}

func (s *deadlineSource) Read(b []byte) (int, error) {
	if !s.armed || len(b) == 0 {
		return s.conn.Read(b)
	}
	n, err := s.conn.Read(b[:1])
	if n > 0 {
		s.armed = false
		if e := s.conn.SetReadDeadline(time.Now().Add(s.timeout)); e != nil {
			return n, e
		}
	}
	return n, err
}

func (h *Handler) runSession(parent context.Context, conn net.Conn, reader *Reader, writer *Writer, source *deadlineSource, a account.WorldSession, name, ip string, log *slog.Logger) error {
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	queue := make(chan world.Packet, h.SendQueue)
	done := make(chan error, 1)
	go func() {
		err := writeSession(ctx, conn, writer, queue, h.RequestTimeout, log)
		done <- err
		cancel()
	}()
	defer func() {
		cancel()
		_ = conn.Close()
		err := <-done
		stop()
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Debug("world writer stopped", "error", err)
		}
	}()
	for {
		if err := conn.SetReadDeadline(time.Now().Add(h.IdleTimeout)); err != nil {
			return err
		}
		source.armed = true
		p, err := reader.ReadPacket()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		meta, known := op.Lookup(p.Opcode)
		if !known || meta.Status != op.STATUS_AUTHED {
			log.Debug("world packet rejected", "opcode", p.Opcode, "name", meta.Name, "reason", "not allowed before player login")
			return errWorldPacket
		}
		log.Debug("world packet received", "opcode", p.Opcode, "name", meta.Name, "bytes", len(p.Payload))
		if p.Opcode == op.CMSG_CHAR_ENUM || p.Opcode == op.CMSG_PING || p.Opcode == op.CMSG_CHAR_CREATE {
			current, err := h.lookup(ctx, name, ip)
			if err != nil || !sameSession(a, current) {
				return account.ErrChanged
			}
		}
		var response *world.Packet
		switch p.Opcode {
		case op.CMSG_CHAR_ENUM:
			if len(p.Payload) != 0 {
				return errWorldPacket
			}
			response, err = h.characterList(ctx, a, name, ip, log)
		case op.CMSG_CHAR_CREATE:
			response, err = h.createCharacter(ctx, a, ip, p, log)
		default:
			response, err = characterScreenPacket(p)
		}
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
func writeSession(ctx context.Context, conn net.Conn, writer *Writer, queue <-chan world.Packet, timeout time.Duration, log *slog.Logger) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p := <-queue:
			if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
				return err
			}
			if err := writer.WritePacket(p); err != nil {
				return err
			}
			meta, _ := op.Lookup(p.Opcode)
			log.Debug("world packet sent", "opcode", p.Opcode, "name", meta.Name, "bytes", len(p.Payload))
		}
	}
}
