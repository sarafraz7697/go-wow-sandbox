package auth

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"testing/iotest"
)

func fixture() []byte {
	b := make([]byte, 38)
	binary.LittleEndian.PutUint16(b[2:], 34)
	copy(b[4:], "WoW\x00")
	copy(b[8:], []byte{2, 4, 3})
	binary.LittleEndian.PutUint16(b[11:], 8606)
	b[33] = 4
	copy(b[34:], "TEST")
	return b
}

func TestFragmentedAndCoalesced(t *testing.T) {
	data := append(fixture(), fixture()...)
	r := iotest.OneByteReader(bytes.NewReader(data))
	for range 2 {
		c, err := ReadChallenge(r, 285)
		if err != nil || c.Build != 8606 || c.Account != "TEST" || c.Version != [3]byte{2, 4, 3} {
			t.Fatalf("%+v: %v", c, err)
		}
	}
	if _, err := ReadChallenge(r, 285); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
}

func TestTruncation(t *testing.T) {
	b := fixture()
	for n := 1; n < len(b); n++ {
		if _, err := ReadChallenge(bytes.NewReader(b[:n]), 285); err == nil {
			t.Fatalf("accepted %d bytes", n)
		}
	}
}

func TestInvalidFrames(t *testing.T) {
	for _, mutate := range []func([]byte){
		func(b []byte) { b[0] = 1 },
		func(b []byte) { b[2] = 255; b[3] = 255 },
		func(b []byte) { b[2] = 0 },
		func(b []byte) { b[33] = 3 },
		func(b []byte) { b[4] = 'X' },
	} {
		b := fixture()
		mutate(b)
		if _, err := ReadChallenge(bytes.NewReader(b), 285); err == nil {
			t.Fatal("accepted invalid frame")
		}
	}
	if _, err := ReadChallenge(bytes.NewReader(fixture()), 32); err == nil {
		t.Fatal("ignored limit")
	}
}

type shortWriter struct{ bytes.Buffer }

func (w *shortWriter) Write(b []byte) (int, error) { return w.Buffer.Write(b[:1]) }

type stalledWriter struct{}

func (stalledWriter) Write([]byte) (int, error) { return 0, nil }

func TestWriteAll(t *testing.T) {
	w := &shortWriter{}
	if err := WriteAll(w, []byte{0, 0, 8}); err != nil || !bytes.Equal(w.Bytes(), []byte{0, 0, 8}) {
		t.Fatal(err, w.Bytes())
	}
	if err := WriteAll(stalledWriter{}, []byte{1}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
}

func FuzzChallenge(f *testing.F) {
	f.Add(fixture())
	f.Add([]byte{0, 0, 255, 255})
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = ReadChallenge(bytes.NewReader(b), 285) })
}
