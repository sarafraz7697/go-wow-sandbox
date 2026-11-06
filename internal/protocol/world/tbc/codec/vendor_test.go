package codec

import (
	"bytes"
	"encoding/binary"
	"testing"

	op "wowserver/internal/protocol/world/tbc/opcodes"
)

func TestListInventoryLayoutMatchesReference(t *testing.T) {
	guid := CreatureGUID(1234, 77)
	pkt := ListInventory(guid, []VendorRow{
		{ItemID: 25, DisplayID: 1, MaxCount: 0xFFFFFFFF, Price: 90, MaxDurability: 10, BuyCount: 1, ExtendedCost: 0},
		{ItemID: 26, DisplayID: 2, MaxCount: 5, Price: 7, MaxDurability: 0, BuyCount: 20, ExtendedCost: 123},
	})
	if pkt.Opcode != op.SMSG_LIST_INVENTORY {
		t.Fatalf("opcode %#x", pkt.Opcode)
	}
	b := pkt.Payload
	if len(b) != 8+1+2*32 {
		t.Fatalf("len = %d, want %d", len(b), 8+1+2*32)
	}
	if binary.LittleEndian.Uint64(b) != guid || b[8] != 2 {
		t.Fatalf("header wrong: %x", b[:9])
	}
	u := func(off int) uint32 { return binary.LittleEndian.Uint32(b[off:]) }
	// slot numbers count from 1, then item, display, maxcount, price, durability, buycount, extcost
	want := []uint32{1, 25, 1, 0xFFFFFFFF, 90, 10, 1, 0, 2, 26, 2, 5, 7, 0, 20, 123}
	for i, w := range want {
		if got := u(9 + i*4); got != w {
			t.Fatalf("field %d = %d, want %d", i, got, w)
		}
	}
}

func TestListInventoryEmptyIsGuidPlusTwoZeroBytes(t *testing.T) {
	pkt := ListInventory(0xF130000001000005, nil)
	if len(pkt.Payload) != 10 || !bytes.Equal(pkt.Payload[8:], []byte{0, 0}) {
		t.Fatalf("payload %x", pkt.Payload)
	}
}

func TestSellErrorCantFindVendor(t *testing.T) {
	pkt := SellError(SellErrCantFindVendor, 0, 0, 0)
	if pkt.Opcode != op.SMSG_SELL_ITEM || len(pkt.Payload) != 17 || pkt.Payload[16] != 3 {
		t.Fatalf("payload %x", pkt.Payload)
	}
	withParam := SellError(SellErrCantSellItem, 1, 2, 9)
	if len(withParam.Payload) != 21 || withParam.Payload[20] != 2 {
		t.Fatalf("payload %x", withParam.Payload)
	}
}

func TestCreatureGUIDRoundTrip(t *testing.T) {
	g := CreatureGUID(0xABCDE, 0x123456)
	if g>>48 != 0xF130 {
		t.Fatalf("high %x", g>>48)
	}
	entry, counter, ok := DecodeCreatureGUID(g)
	if !ok || entry != 0xABCDE || counter != 0x123456 {
		t.Fatalf("decode = %x %x %v", entry, counter, ok)
	}
	if _, _, ok := DecodeCreatureGUID(makeFullGUID(HighGUIDGameObject, 1, 1)); ok {
		t.Fatal("gameobject guid decoded as creature")
	}
}
