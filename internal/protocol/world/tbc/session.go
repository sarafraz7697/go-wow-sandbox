package tbc

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"
	"wowserver/internal/domain/account"
	domainworld "wowserver/internal/domain/world"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

type WorldSessionStore interface {
	WorldSession(context.Context, string, string, uint32) (account.WorldSession, error)
}

// Handler must be shared by all connections of this realm and never copied.
// It owns a process-local admission registry and a reference to the shared
// world simulation (Broadcaster) used for packet routing between players.
type Handler struct {
	Players                     PlayerStore
	Creator                     CharacterCreator
	Deleter                     CharacterDeleter
	Characters                  CharacterStore
	Data                        *gamedata.Catalog // Immutable startup snapshot, used by later character phases.
	Store                       WorldSessionStore
	Broadcast                   domainworld.Broadcaster // May be nil; broadcast is skipped when nil.
	Logger                      *slog.Logger
	RealmID                     uint32
	SendQueue                   int
	IdleTimeout, RequestTimeout time.Duration
	OnAuthenticated             func(context.Context) error
	mu                          sync.Mutex
	active                      map[uint64]*activeConn
}

// activeConn tracks one admitted account so a duplicate connection can
// kick the previous session and wait for its release. This mirrors the
// reference World::AddSession behaviour: the NEW connection wins, the OLD
// session is deleted and unwinds through its normal cleanup path.
type activeConn struct {
	kick func()       // closes the owning session's connection
	done chan struct{} // closed by release once the session fully unwound
}

// errAlreadyOnline is returned when the previous session of the account did
// not unwind within kickWait; the caller answers AUTH_ALREADY_ONLINE.
var errAlreadyOnline = errors.New("account session still active")

// kickWait bounds how long a new connection waits for the previous session
// of the same account to save and release. The save path is bounded by
// RequestTimeout (default 10s), so this covers it plus a grace period.
const kickWait = 15 * time.Second

// claim admits the account for this connection. If the account is already
// admitted, the previous session's connection is closed (kick) and the
// caller waits for it to finish saving and releasing before re-claiming.
func (h *Handler) claim(id uint64, kick func()) (*activeConn, error) {
	for {
		h.mu.Lock()
		if h.active == nil {
			h.active = make(map[uint64]*activeConn)
		}
		if cur, ok := h.active[id]; ok {
			oldKick, oldDone := cur.kick, cur.done
			h.mu.Unlock()
			// Reference: the old session is kicked, not the new one.
			if oldKick != nil {
				oldKick()
			}
			if oldDone == nil {
				continue
			}
			timer := time.NewTimer(kickWait)
			select {
			case <-oldDone:
				timer.Stop()
			case <-timer.C:
				return nil, errAlreadyOnline
			}
			continue
		}
		c := &activeConn{kick: kick, done: make(chan struct{})}
		h.active[id] = c
		h.mu.Unlock()
		return c, nil
	}
}

// release removes the admission and wakes any waiting duplicate connection.
func (h *Handler) release(id uint64, c *activeConn) {
	h.mu.Lock()
	if cur, ok := h.active[id]; ok && cur == c {
		delete(h.active, id)
	}
	h.mu.Unlock()
	close(c.done)
}

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
		return codec.ErrWorldPacket
	}
	request, err := codec.DecodeAuthRequest(p.Payload)
	if err != nil {
		return err
	}
	log.Debug("world auth request decoded", "stage", "auth_session", "build", request.Build, "addon_bytes", len(request.Addons))
	fail := func(code byte) error {
		log.Debug("world authentication rejected", "result_code", code)
		if err := writer.WritePacket(codec.AuthResponse(code, 0)); err != nil {
			return err
		}
		return codec.ErrWorldAuth
	}
	if request.Build != 8606 {
		return fail(codec.AuthVersionMismatch)
	}
	name, _ := account.Normalize(request.Name)
	a, err := h.lookup(ctx, name, ip)
	if err != nil {
		return fail(codec.AuthFailed)
	}
	if !codec.VerifyWorldDigest(request, binary.LittleEndian.Uint32(seed[:]), a.Key) {
		return fail(codec.AuthFailed)
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
	addons, err := codec.AddonResponse(request.Addons)
	if err != nil {
		return fail(codec.AuthFailed)
	}
	current, err := h.lookup(ctx, name, ip)
	if err != nil || !sameSession(a, current) {
		return fail(codec.AuthFailed)
	}
	session, claimErr := h.claim(a.ID, func() { _ = conn.Close() })
	if claimErr != nil {
		return fail(codec.AuthAlreadyOnline)
	}
	defer h.release(a.ID, session)
	if h.OnAuthenticated != nil {
		if err = h.OnAuthenticated(ctx); err != nil {
			return err
		}
	}
	if err = conn.SetWriteDeadline(time.Now().Add(h.RequestTimeout)); err != nil {
		return err
	}
	if err = writer.WritePacket(codec.AuthResponse(codec.AuthOK, a.Expansion)); err != nil {
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

	incoming := make(chan received, 1)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			err := conn.SetReadDeadline(time.Now().Add(h.IdleTimeout))
			var p world.Packet
			if err == nil {
				source.armed = true
				p, err = reader.ReadPacket()
			}
			select {
			case incoming <- received{p, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	// Joining the reader before returning prevents it retaining the connection.
	defer func() { cancel(); _ = conn.Close(); <-readerDone }()
	s := &Session{state: StateAuthed, account: a, name: name, ip: ip, log: log, events: make(chan transition, 8), done: ctx.Done()}
	s.send = func(packets ...world.Packet) error {
		writeCtx, end := context.WithTimeout(ctx, h.RequestTimeout)
		defer end()
		for _, p := range packets {
			select {
			case queue <- p:
			case <-writeCtx.Done():
				return writeCtx.Err()
			}
		}
		return nil
	}
	// trySend is the non-blocking path used by broadcasts. A full queue drops
	// the packet rather than stalling the broadcast loop; the dropped packet
	// is logged at debug by the caller if needed.
	s.trySend = func(p world.Packet) bool {
		select {
		case queue <- p:
			return true
		default:
			return false
		}
	}
	s.onTick = h.playerTick
	defer func() {
		if s.player != nil {
			cleanup, end := context.WithTimeout(context.Background(), h.RequestTimeout)
			defer end()
			if err := h.releasePlayer(cleanup, s); err != nil {
				log.Error("player cleanup failed; online flag may require recovery", "error", err)
			}
		}
	}()
	defer func() { _ = s.transition(StateClosed) }()
	router := h.newRouter()
	return s.run(ctx, router, incoming, queue)
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
		}
	}
}
