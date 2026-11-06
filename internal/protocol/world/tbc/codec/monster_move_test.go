package codec

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestMonsterMoveLinearLayout(t *testing.T) {
	g := CreatureGUID(1234, 77)
	p := MonsterMoveLinear(g, 1, 2, 3, 4, 5, 6, 900)
	b := p.Payload
	off := skipPackedGUID(b, 0)
	if len(b)-off != 12+4+1+4+4+4+12 {
		t.Fatalf("body len %d", len(b)-off)
	}
	f := func(o int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(b[off+o:])) }
	if f(0) != 1 || f(4) != 2 || f(8) != 3 {
		t.Fatal("start")
	}
	if b[off+16] != 0 {
		t.Fatal("type")
	}
	if binary.LittleEndian.Uint32(b[off+17:]) != 0x100 || binary.LittleEndian.Uint32(b[off+21:]) != 900 || binary.LittleEndian.Uint32(b[off+25:]) != 1 {
		t.Fatal("flags/duration/count")
	}
	if f(29) != 4 || f(33) != 5 || f(37) != 6 {
		t.Fatal("dest")
	}
	s := MonsterMoveStop(g, 1, 2, 3)
	if s.Payload[len(s.Payload)-1] != 1 {
		t.Fatal("stop")
	}
}
