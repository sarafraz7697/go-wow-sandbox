package codec

import (
	"sort"

	"wowserver/internal/domain/inventory"
	"wowserver/internal/entity"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
	f "wowserver/internal/protocol/world/tbc/updatefields"
)

const guidHighItemWord = 0x40000000

// ItemGUID is the full ObjectGuid of an item (HIGHGUID_ITEM = 0x4000).
func ItemGUID(low uint32) uint64 { return uint64(low) | uint64(guidHighItemWord)<<32 }

// ItemCreateUpdate ports Item::SendCreateUpdateToPlayer: one SMSG_UPDATE_OBJECT
// with a single CREATE_OBJECT block (update flags LOWGUID|HIGHGUID), the same
// block InitialSelfUpdate emits for carried items. bag lists the item guid per
// slot when the item is a container.
func ItemCreateUpdate(it inventory.Item, bag []uint32, details entity.ItemPrototype) world.Packet {
	itemFields := append([]uint32(nil), it.Fields[:]...)
	objectType := uint8(1)
	if details.ContainerSlots > 0 {
		objectType = 2
		itemFields = append(itemFields, make([]uint32, f.CONTAINER_END-f.ITEM_END)...)
		itemFields[f.OBJECT_FIELD_TYPE] = 7
		itemFields[f.CONTAINER_FIELD_NUM_SLOTS] = details.ContainerSlots
		for slot, g := range bag {
			if g != 0 {
				at := f.CONTAINER_FIELD_SLOT_1 + slot*2
				itemFields[at], itemFields[at+1] = g, guidHighItemWord
			}
		}
	}
	w := wire.NewWriter(MaxServerPayload)
	w.Uint32(1)
	w.Uint8(0)
	w.Uint8(2) // UPDATETYPE_CREATE_OBJECT
	w.PackedGUID(ItemGUID(it.GUID))
	w.Uint8(objectType)
	w.Uint8(0x18)
	w.Uint32(it.GUID)
	w.Uint32(0x4000)
	writeValues(w, itemFields)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_UPDATE_OBJECT, Payload: b}
}

// FieldChange is one changed update-field word.
type FieldChange struct {
	Index int
	Value uint32
}

// ValuesUpdate builds an SMSG_UPDATE_OBJECT with one UPDATETYPE_VALUES block:
// the mask covers exactly the changed words (zero values included), like
// Object::BuildValuesUpdateBlockForPlayer.
func ValuesUpdate(guid uint64, changes []FieldChange) world.Packet {
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Index < changes[j].Index })
	// last write wins for a repeated index
	uniq := changes[:0:0]
	for _, c := range changes {
		if n := len(uniq); n > 0 && uniq[n-1].Index == c.Index {
			uniq[n-1] = c
			continue
		}
		uniq = append(uniq, c)
	}
	blocks := 0
	if n := len(uniq); n > 0 {
		blocks = uniq[n-1].Index/32 + 1
	}
	mask := make([]uint32, blocks)
	for _, c := range uniq {
		mask[c.Index/32] |= 1 << uint(c.Index%32)
	}
	w := wire.NewWriter(MaxServerPayload)
	w.Uint32(1)
	w.Uint8(0)
	w.Uint8(0) // UPDATETYPE_VALUES
	w.PackedGUID(guid)
	w.Uint8(uint8(blocks))
	for _, m := range mask {
		w.Uint32(m)
	}
	for _, c := range uniq {
		w.Uint32(c.Value)
	}
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_UPDATE_OBJECT, Payload: b}
}

// InventoryChangeFailure ports Player::SendEquipError.
func InventoryChangeFailure(fl inventory.Failure) world.Packet {
	w := wire.NewWriter(24)
	w.Uint8(uint8(fl.Code))
	if fl.Code != inventory.ErrOK {
		if fl.Code == inventory.ErrCantEquipLevel {
			w.Uint32(fl.RequiredLevel)
		}
		var g1, g2 uint64
		if fl.Item1 != 0 {
			g1 = ItemGUID(fl.Item1)
		}
		if fl.Item2 != 0 {
			g2 = ItemGUID(fl.Item2)
		}
		w.Uint64(g1)
		w.Uint64(g2)
		w.Uint8(0) // bag type subclass
	}
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_INVENTORY_CHANGE_FAILURE, Payload: b}
}

// VisibleItemFields returns the PLAYER_VISIBLE_ITEM_* changes of
// SetVisibleItemSlot for a slot; entry == 0 clears them. enchants are the
// first seven enchantment ids exactly as InitialSelfUpdate writes them.
func VisibleItemFields(slot int, it *inventory.Item) []FieldChange {
	base := f.PLAYER_VISIBLE_ITEM_1_0 + slot*16
	out := make([]FieldChange, 0, 10)
	if it == nil {
		out = append(out, FieldChange{base, 0})
		for i := 0; i < 7; i++ {
			out = append(out, FieldChange{base + 1 + i, 0})
		}
		return append(out, FieldChange{base + 12, 0})
	}
	out = append(out, FieldChange{base, it.Entry})
	for i := 0; i < 7; i++ {
		out = append(out, FieldChange{base + 1 + i, it.Fields[22+i*3]})
	}
	return append(out, FieldChange{base + 12, it.Fields[56]})
}
