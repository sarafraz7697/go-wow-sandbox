package codec

import (
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

// CreatureGUID builds the 64-bit ObjectGuid of a creature spawn
// (HIGHGUID_UNIT with entry).
func CreatureGUID(entry, spawnGUID uint32) uint64 {
	return makeFullGUID(HighGUIDUnit, entry, spawnGUID)
}

// DecodeCreatureGUID splits a HIGHGUID_UNIT ObjectGuid into its template
// entry and `creature`.guid counter (ObjectGuid::GetEntry / GetCounter).
func DecodeCreatureGUID(guid uint64) (entry, spawnGUID uint32, ok bool) {
	if guid>>48 != HighGUIDUnit {
		return 0, 0, false
	}
	return uint32(guid >> 24 & 0xFFFFFF), uint32(guid & 0xFFFFFF), true
}

// GameObjectGUID builds the full HIGHGUID_GAMEOBJECT ObjectGuid of a spawn.
func GameObjectGUID(entry, spawnGUID uint32) uint64 {
	return makeFullGUID(HighGUIDGameObject, entry, spawnGUID)
}

// DestroyObjectGUID builds SMSG_DESTROY_OBJECT for a full 64-bit guid
// (Object::DestroyForPlayer: body is the raw uint64 guid).
func DestroyObjectGUID(guid uint64) world.Packet {
	w := wire.NewWriter(8)
	w.Uint64(guid)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_DESTROY_OBJECT, Payload: b}
}
