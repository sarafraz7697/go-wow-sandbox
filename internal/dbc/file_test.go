package dbc

import (
	"encoding/binary"
	"testing"
)

func fixture() []byte {
	b := make([]byte, 30)
	copy(b, "WDBC")
	for i, v := range []uint32{1, 2, 8, 2} {
		binary.LittleEndian.PutUint32(b[4+i*4:], v)
	}
	binary.LittleEndian.PutUint32(b[20:], 42)
	return b
}
func TestDecodeAndOwnership(t *testing.T) {
	b := fixture()
	f, e := Decode(b, 2, 8)
	if e != nil {
		t.Fatal(e)
	}
	b[20] = 99
	if v, e := f.Uint32(0, 0); e != nil || v != 42 {
		t.Fatal("borrowed input")
	}
	if s, e := f.String(0, 4); e != nil || s != "" {
		t.Fatal("empty string", e)
	}
	for _, p := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 5}} {
		if _, e := f.Uint32(p[0], p[1]); e == nil {
			t.Fatal("out-of-bounds field")
		}
	}
	// Two one-byte fields: field_count*4 is deliberately NOT record size.
	b = make([]byte, 23)
	copy(b, "WDBC")
	for i, v := range []uint32{1, 2, 2, 1} {
		binary.LittleEndian.PutUint32(b[4+i*4:], v)
	}
	b[20] = 11
	b[21] = 7
	f, e = Decode(b, 2, 2)
	if e != nil {
		t.Fatal(e)
	}
	if v, e := f.Byte(0, 1); e != nil || v != 7 {
		t.Fatal("byte field")
	}
}
func TestMalformedDBC(t *testing.T) {
	good := fixture()
	for n := 0; n < len(good); n++ {
		if _, e := Decode(good[:n], 2, 8); e == nil {
			t.Fatalf("accepted truncated length %d", n)
		}
	}
	for _, offset := range []int{0, 4, 8, 12, 16, 28, 29} {
		b := fixture()
		b[offset] = 255
		if _, e := Decode(b, 2, 8); e == nil {
			t.Fatalf("accepted malformed offset %d", offset)
		}
	}
	b := fixture()
	binary.LittleEndian.PutUint32(b[24:], 99)
	f, e := Decode(b, 2, 8)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.String(0, 4); e == nil {
		t.Fatal("invalid string offset")
	}
	if _, e := Decode(append(fixture(), 0), 2, 8); e == nil {
		t.Fatal("trailing bytes")
	}
}
func FuzzDecode(f *testing.F) {
	f.Add(fixture())
	f.Add([]byte("WDBC"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		d, e := Decode(b, 2, 8)
		if e == nil && d.Len() > 0 {
			d.Uint32(0, 0)
			d.String(0, 4)
		}
	})
}
