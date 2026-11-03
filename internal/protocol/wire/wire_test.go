package wire

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"testing"
)

func TestPrimitiveFixture(t *testing.T) {
	// Hand-authored layout: u8,u16,u32,u64,f32,cstring,packed GUID.
	want, _ := hex.DecodeString("ab34127856341208070605040302010000c03f416200850102ff")
	w := NewWriter(len(want))
	w.Uint8(0xab)
	w.Uint16(0x1234)
	w.Uint32(0x12345678)
	w.Uint64(0x0102030405060708)
	w.Float32(1.5)
	w.CString("Ab", 2)
	w.PackedGUID(0xff00000000020001)
	got, err := w.Result()
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("encoding %x: %v", got, err)
	}
	r := NewReader(want)
	if r.Uint8() != 0xab || r.Uint16() != 0x1234 || r.Uint32() != 0x12345678 || r.Uint64() != 0x0102030405060708 || r.Float32() != 1.5 || r.CString(2) != "Ab" || r.PackedGUID() != 0xff00000000020001 {
		t.Fatal("fixture decode")
	}
	if err = r.Finish(); err != nil {
		t.Fatal(err)
	}
}
func TestBoundsAndStrings(t *testing.T) {
	for _, tc := range []struct {
		data string
		max  int
		want string
		err  error
	}{
		{"\x00", 0, "", nil}, {"abc\x00", 3, "abc", nil}, {"abc", 3, "", io.ErrUnexpectedEOF},
		{"abcd\x00", 3, "", ErrLimit}, {"", 0, "", io.ErrUnexpectedEOF}, {"\x00", -1, "", ErrLimit},
	} {
		r := NewReader([]byte(tc.data))
		got := r.CString(tc.max)
		if got != tc.want || !errors.Is(r.Err(), tc.err) {
			t.Fatalf("string %q max %d: %q %v", tc.data, tc.max, got, r.Err())
		}
	}
	r := NewReader([]byte{1})
	r.Uint64()
	r.Uint8()
	if !errors.Is(r.Finish(), io.ErrUnexpectedEOF) || r.Remaining() != 1 {
		t.Fatal("non-sticky bounds error")
	}
	r = NewReader([]byte{1})
	if !errors.Is(r.Finish(), ErrTrailing) {
		t.Fatal("trailing byte accepted")
	}
	r = NewReader(nil)
	r.Bytes(-1)
	if !errors.Is(r.Err(), ErrLimit) {
		t.Fatal("negative length")
	}
	for _, max := range []int{-1, 0, 3} {
		w := NewWriter(max)
		w.Uint32(1)
		w.Uint8(2)
		if b, e := w.Result(); !errors.Is(e, ErrLimit) || b != nil {
			t.Fatal("writer limit")
		}
	}
	w := NewWriter(20)
	w.CString("a\x00b", 10)
	if _, e := w.Result(); !errors.Is(e, ErrString) {
		t.Fatal("embedded NUL")
	}
	w = NewWriter(3)
	w.CString("abc", 3)
	if _, e := w.Result(); !errors.Is(e, ErrLimit) {
		t.Fatal("terminator overflow")
	}
	w = NewWriter(4)
	w.CString("abc", 2)
	if _, e := w.Result(); !errors.Is(e, ErrLimit) {
		t.Fatal("string maximum")
	}
}
func TestPackedGUIDFixtures(t *testing.T) {
	for _, tc := range []struct {
		guid uint64
		hex  string
	}{{0, "00"}, {1, "0101"}, {0x100, "0201"}, {math.MaxUint64, "ffffffffffffffffff"}} {
		w := NewWriter(9)
		w.PackedGUID(tc.guid)
		got, err := w.Result()
		if err != nil || hex.EncodeToString(got) != tc.hex {
			t.Fatal(tc, err)
		}
		r := NewReader(got)
		if r.PackedGUID() != tc.guid || r.Finish() != nil {
			t.Fatal("packed decode")
		}
	}
	r := NewReader([]byte{0x80})
	r.PackedGUID()
	if !errors.Is(r.Err(), io.ErrUnexpectedEOF) {
		t.Fatal("missing GUID byte")
	}
}
func FuzzReader(f *testing.F) {
	f.Add([]byte{0x80, 1, 0}, uint64(0xff00000000020001))
	f.Fuzz(func(t *testing.T, data []byte, guid uint64) {
		r := NewReader(data)
		r.Uint32()
		r.CString(128)
		r.PackedGUID()
		r.Uint64()
		r.Float32()
		r.Finish()
		if r.Remaining() < 0 || r.Remaining() > len(data) {
			t.Fatal("invalid cursor")
		}
		w := NewWriter(9)
		w.PackedGUID(guid)
		b, err := w.Result()
		if err != nil {
			t.Fatal(err)
		}
		r = NewReader(b)
		if r.PackedGUID() != guid || r.Finish() != nil {
			t.Fatal("GUID round trip")
		}
	})
}
