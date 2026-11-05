package codec

import (
	"encoding/binary"
	"testing"

	"wowserver/internal/entity"
	"wowserver/internal/gamedata"
)

// skipPackedGUID returns the offset just past a packed GUID at off.
func skipPackedGUID(b []byte, off int) int {
	mask := b[off]
	off++
	for i := 0; i < 8; i++ {
		if mask&(1<<uint(i)) != 0 {
			off++
		}
	}
	return off
}

// entryFromMask reads OBJECT_FIELD_ENTRY (index 3) out of a values block
// starting at off, returning ok=false when the field is not in the mask.
func entryFromMask(b []byte, off int) (uint32, bool) {
	blocks := int(b[off])
	off++
	mask := make([]uint32, blocks)
	for i := range mask {
		mask[i] = binary.LittleEndian.Uint32(b[off:])
		off += 4
	}
	if mask[0]&(1<<3) == 0 {
		return 0, false
	}
	// Fields 0..2 precede entry; count set bits below bit 3.
	idx := 0
	for bit := 0; bit < 3; bit++ {
		if mask[0]&(1<<uint(bit)) != 0 {
			idx++
		}
	}
	return binary.LittleEndian.Uint32(b[off+idx*4:]), true
}

func TestCreateObjectForCreatureCarriesEntry(t *testing.T) {
	pkt := CreateObjectForCreature(gamedata.ResolvedCreatureSpawn{GUID: 7, Entry: 1234, Level: 1, Scale: 1}, 1)
	b := pkt.Payload
	if b[5] != 2 {
		t.Fatalf("update type = %d, want 2 (CREATE_OBJECT)", b[5])
	}
	off := skipPackedGUID(b, 6)
	off += 2 // typeid + updateFlags
	off += 4 + 1 + 4 + 16 + 4 + 32 + 4
	entry, ok := entryFromMask(b, off)
	if !ok || entry != 1234 {
		t.Fatalf("OBJECT_FIELD_ENTRY = %d (present=%v), want 1234", entry, ok)
	}
}

func TestCreateObjectForGameObjectCarriesEntry(t *testing.T) {
	spawn := gamedata.GameObjectSpawnRef{GUID: 9, Data: entity.GameObjectData{ID: 321}}
	pkt := CreateObjectForGameObject(9, spawn, entity.GameObjectInfo{ID: 321, Type: 5, DisplayID: 10})
	b := pkt.Payload
	if b[5] != 2 {
		t.Fatalf("update type = %d, want 2", b[5])
	}
	off := skipPackedGUID(b, 6)
	off += 2 + 16 + 4 + 4
	entry, ok := entryFromMask(b, off)
	if !ok || entry != 321 {
		t.Fatalf("OBJECT_FIELD_ENTRY = %d (present=%v), want 321", entry, ok)
	}
	trap := CreateObjectForGameObject(9, spawn, entity.GameObjectInfo{ID: 321, Type: 6})
	if trap.Payload[5] != 3 {
		t.Fatalf("trap update type = %d, want 3", trap.Payload[5])
	}
}
