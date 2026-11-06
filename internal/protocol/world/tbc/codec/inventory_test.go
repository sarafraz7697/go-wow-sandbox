package codec

import (
	"encoding/binary"
	"testing"

	"wowserver/internal/domain/inventory"
)

func TestInventoryChangeFailureLayout(t *testing.T) {
	p := InventoryChangeFailure(inventory.Failure{Code: inventory.ErrItemNotFound})
	// code, guid1(8), guid2(8), uint8 0
	if len(p.Payload) != 1+8+8+1 || p.Payload[0] != 23 {
		t.Fatalf("%v", p.Payload)
	}
	p = InventoryChangeFailure(inventory.Failure{Code: inventory.ErrCantEquipLevel, Item1: 5, RequiredLevel: 40})
	if len(p.Payload) != 1+4+8+8+1 || binary.LittleEndian.Uint32(p.Payload[1:]) != 40 || binary.LittleEndian.Uint32(p.Payload[5:]) != 5 || binary.LittleEndian.Uint32(p.Payload[9:]) != 0x40000000 {
		t.Fatalf("%v", p.Payload)
	}
	if p = InventoryChangeFailure(inventory.Failure{}); len(p.Payload) != 1 {
		t.Fatal("OK must be a single byte")
	}
}

func TestValuesUpdateMask(t *testing.T) {
	p := ValuesUpdate(7, []FieldChange{{Index: 40, Value: 9}, {Index: 3, Value: 0}})
	b := p.Payload
	// count=1, hasTransport=0, type=0, packed guid(7) = mask 0x01, 0x07
	if binary.LittleEndian.Uint32(b) != 1 || b[4] != 0 || b[5] != 0 || b[6] != 1 || b[7] != 7 {
		t.Fatalf("header %v", b[:8])
	}
	if b[8] != 2 { // 41 words needs 2 mask blocks
		t.Fatalf("blocks %d", b[8])
	}
	m0, m1 := binary.LittleEndian.Uint32(b[9:]), binary.LittleEndian.Uint32(b[13:])
	if m0 != 1<<3 || m1 != 1<<(40-32) {
		t.Fatalf("mask %x %x", m0, m1)
	}
	if binary.LittleEndian.Uint32(b[17:]) != 0 || binary.LittleEndian.Uint32(b[21:]) != 9 || len(b) != 25 {
		t.Fatalf("values %v", b[17:])
	}
}
