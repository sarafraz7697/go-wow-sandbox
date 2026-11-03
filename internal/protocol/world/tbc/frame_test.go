package tbc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"testing/iotest"
	"wowserver/internal/protocol/world"
)

func clientFrame(op uint32, payload []byte) []byte {
	b := make([]byte, 6+len(payload))
	binary.BigEndian.PutUint16(b, uint16(len(payload)+4))
	binary.LittleEndian.PutUint32(b[2:], op)
	copy(b[6:], payload)
	return b
}
func TestHeadersAndLimits(t *testing.T) {
	op, n, err := DecodeClientHeader(unhex(t, "000cdc010000"))
	if err != nil || op != 0x1dc || n != 8 {
		t.Fatal(op, n, err)
	}
	for _, tc := range []struct {
		hex string
		err error
	}{{"000300000000", ErrSize}, {"280100000000", ErrSize}, {"000401280000", ErrOpcode}, {"0004ffffffff", ErrOpcode}, {"0004", ErrHeader}} {
		if _, _, e := DecodeClientHeader(unhex(t, tc.hex)); !errors.Is(e, tc.err) {
			t.Fatal(tc, e)
		}
	}
	if _, n, e := DecodeClientHeader(unhex(t, "280000280000")); e != nil || n != 10236 {
		t.Fatal("client maximum", n, e)
	}
	for _, tc := range []struct {
		size   int
		header string
	}{{0, "0002ec01"}, {4, "0006ec01"}, {32765, "7fffec01"}, {32766, "8000ec01"}, {65533, "ffffec01"}} {
		b, e := EncodeServer(world.Packet{Opcode: 0x1ec, Payload: make([]byte, tc.size)})
		if e != nil || len(b) != 4+tc.size || !bytes.Equal(b[:4], unhex(t, tc.header)) {
			t.Fatalf("server size %d: %v", tc.size, e)
		}
	}
	if _, e := EncodeServer(world.Packet{Payload: make([]byte, 65534)}); !errors.Is(e, ErrSize) {
		t.Fatal("server overflow")
	}
	if _, e := EncodeServer(world.Packet{Opcode: 65536}); !errors.Is(e, ErrOpcode) {
		t.Fatal("opcode truncation")
	}
}
func TestFragmentedAndCoalescedPackets(t *testing.T) {
	stream := append(clientFrame(0x37, nil), clientFrame(0x1dc, []byte{1, 2, 3, 4, 5, 6, 7, 8})...)
	for _, input := range []io.Reader{bytes.NewReader(stream), iotest.OneByteReader(bytes.NewReader(stream)), iotest.HalfReader(bytes.NewReader(stream))} {
		r := NewReader(input)
		a, e := r.ReadPacket()
		if e != nil || a.Opcode != 0x37 || len(a.Payload) != 0 {
			t.Fatal(a, e)
		}
		b, e := r.ReadPacket()
		if e != nil || b.Opcode != 0x1dc || !bytes.Equal(b.Payload, stream[12:]) {
			t.Fatal(b, e)
		}
		if _, e = r.ReadPacket(); !errors.Is(e, io.EOF) {
			t.Fatal(e)
		}
	}
}
func TestTruncationIsTerminal(t *testing.T) {
	full := clientFrame(0x1dc, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	for n := 0; n < len(full); n++ {
		r := NewReader(bytes.NewReader(full[:n]))
		_, err := r.ReadPacket()
		want := io.ErrUnexpectedEOF
		if n == 0 {
			want = io.EOF
		}
		if !errors.Is(err, want) {
			t.Fatalf("length %d: %v", n, err)
		}
		if _, again := r.ReadPacket(); again != err {
			t.Fatal("reader resumed after error")
		}
	}
	bad := append(unhex(t, "ffff37000000"), full...)
	r := NewReader(bytes.NewReader(bad))
	if _, e := r.ReadPacket(); !errors.Is(e, ErrSize) {
		t.Fatal(e)
	}
	if _, e := r.ReadPacket(); !errors.Is(e, ErrSize) {
		t.Fatal("resynchronized malformed stream")
	}
}
func TestCryptTransitionAndPayloadIsolation(t *testing.T) {
	v := vectors(t)[0]
	var key [40]byte
	copy(key[:], unhex(t, v.Session))
	plain := clientFrame(0x1ed, []byte{9}) // last plaintext handshake packet
	stream := append(plain, unhex(t, v.ClientEncrypted[0])...)
	stream = append(stream, unhex(t, v.ClientEncrypted[1])...)
	stream = append(stream, []byte{1, 2, 3, 4, 5, 6, 7, 8}...)
	r := NewReader(iotest.OneByteReader(bytes.NewReader(stream)))
	if _, e := r.ReadPacket(); e != nil {
		t.Fatal(e)
	}
	if e := r.EnableEncryption(key); e != nil {
		t.Fatal(e)
	}
	if e := r.EnableEncryption(key); !errors.Is(e, ErrCryptState) {
		t.Fatal("reset permitted")
	}
	if p, e := r.ReadPacket(); e != nil || p.Opcode != 0x1dc || len(p.Payload) != 0 {
		t.Fatal(p, e)
	}
	if p, e := r.ReadPacket(); e != nil || !bytes.Equal(p.Payload, []byte{1, 2, 3, 4, 5, 6, 7, 8}) {
		t.Fatal(p, e)
	}
	var out bytes.Buffer
	w := NewWriter(&out)
	if e := w.EnableEncryption(key); e != nil {
		t.Fatal(e)
	}
	// Encoding errors must not consume the cipher stream.
	if e := w.WritePacket(world.Packet{Opcode: 65536}); !errors.Is(e, ErrOpcode) {
		t.Fatal(e)
	}
	for i, header := range v.ServerPlain {
		h := unhex(t, header)
		p := world.Packet{Opcode: uint32(binary.LittleEndian.Uint16(h[2:])), Payload: bytes.Repeat([]byte{0xa5}, int(binary.BigEndian.Uint16(h))-2)}
		out.Reset()
		if e := w.WritePacket(p); e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(out.Bytes()[:4], unhex(t, v.ServerEncrypted[i])) || !bytes.Equal(out.Bytes()[4:], p.Payload) {
			t.Fatal("encrypted payload or wrong header", i)
		}
	}
}

type shortWriter struct {
	bytes.Buffer
	max int
}

func (w *shortWriter) Write(b []byte) (int, error) {
	if len(b) > w.max {
		b = b[:w.max]
	}
	return w.Buffer.Write(b)
}

type brokenWriter struct {
	calls, n int
	err      error
}

func (w *brokenWriter) Write(b []byte) (int, error) { w.calls++; return w.n, w.err }
func TestShortWritesAndFailure(t *testing.T) {
	output := &shortWriter{max: 1}
	w := NewWriter(output)
	p := world.Packet{Opcode: 0x1ec, Payload: []byte{1, 2, 3, 4}}
	if e := w.WritePacket(p); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(output.Bytes(), unhex(t, "0006ec0101020304")) {
		t.Fatal("short write corrupted")
	}
	for _, failure := range []*brokenWriter{{n: 0}, {n: -1}, {n: 100}, {n: 1, err: io.ErrClosedPipe}} {
		w := NewWriter(failure)
		if e := w.EnableEncryption([40]byte{1}); e != nil {
			t.Fatal(e)
		}
		e := w.WritePacket(p)
		if e == nil {
			t.Fatal("write failure ignored")
		}
		if again := w.WritePacket(p); again != e || failure.calls != 1 {
			t.Fatal("retry after partial write")
		}
		if again := w.EnableEncryption([40]byte{2}); again != e {
			t.Fatal("revived failed writer")
		}
	}
	if e := NewReader(bytes.NewReader(nil)).EnableEncryption([40]byte{}); !errors.Is(e, ErrCryptState) {
		t.Fatal("zero key")
	}
	if e := NewWriter(io.Discard).EnableEncryption([40]byte{}); !errors.Is(e, ErrCryptState) {
		t.Fatal("zero key")
	}
}
func FuzzClientFraming(f *testing.F) {
	f.Add(clientFrame(0x37, nil))
	f.Add(clientFrame(0x1dc, []byte{1, 2, 3}))
	f.Add([]byte{255, 255, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 65536 {
			return
		}
		a, c := NewReader(bytes.NewReader(b)), NewReader(iotest.OneByteReader(bytes.NewReader(b)))
		for i := 0; i <= len(b)/6; i++ {
			p, e := a.ReadPacket()
			q, g := c.ReadPacket()
			if !errors.Is(e, g) || p.Opcode != q.Opcode || !bytes.Equal(p.Payload, q.Payload) {
				t.Fatal("fragmentation changed decode")
			}
			if e != nil {
				return
			}
			if len(p.Payload) > MaxClientSize-4 {
				t.Fatal("unbounded payload")
			}
		}
		t.Fatal("reader failed to consume headers")
	})
}
