package codec

import (
	"math"

	domainworld "wowserver/internal/domain/world"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
	f "wowserver/internal/protocol/world/tbc/updatefields"
)

// CreateObjectForObserver builds an SMSG_UPDATE_OBJECT packet with a single
// UPDATETYPE_CREATE_OBJECT2 block for a player visible to another player.
// Unlike InitialSelfUpdate, this does NOT include inventory item blocks or
// the UPDATEFLAG_SELF bit — the receiving client renders the player as a
// "foreign" object.
//
// The packet format mirrors MaNGOS Object::BuildCreateUpdateBlockForPlayer
// + BuildMovementUpdate + BuildValuesUpdate for a TYPEID_PLAYER target !=
// self:
//
//	SMSG_UPDATE_OBJECT:
//	  uint32 count (1)
//	  uint8  hasTransport (0)
//	  uint8  UPDATETYPE_CREATE_OBJECT2 (3)
//	  packedGUID
//	  uint8  typeId (4 = TYPEID_PLAYER)
//	  uint8  updateFlags (0x70 = LIVING|HAS_POSITION|HIGHGUID, no SELF)
//	  MovementInfo: flags(0) extra(0) time x y z o fallTime(0)
//	  Speeds: walk run runBack swim swimBack flight flightBack turnRate
//	  uint32 highGUID (0 for player)
//	  UpdateMask + values (public visible fields only)
func CreateObjectForObserver(p domainworld.PlayerPresence, clock uint32) world.Packet {
	fields := make([]uint32, f.PLAYER_END)
	fields[f.OBJECT_FIELD_GUID] = p.GUID
	fields[f.OBJECT_FIELD_TYPE] = 25
	fields[f.OBJECT_FIELD_SCALE_X] = math.Float32bits(1)
	fields[f.UNIT_FIELD_BYTES_0] = uint32(p.Race) | uint32(p.Class)<<8 | uint32(p.Gender)<<16 | uint32(p.Power)<<24
	fields[f.UNIT_FIELD_LEVEL] = uint32(p.Level)
	fields[f.UNIT_FIELD_MAXHEALTH] = p.MaxHealth
	fields[f.UNIT_FIELD_HEALTH] = min(p.Health, p.MaxHealth)
	fields[f.UNIT_FIELD_POWER1] = min(p.Mana, p.MaxMana)
	fields[f.UNIT_FIELD_MAXPOWER1] = p.MaxMana
	fields[f.UNIT_FIELD_POWER2] = min(p.Rage, 1000)
	fields[f.UNIT_FIELD_MAXPOWER2] = 1000
	fields[f.UNIT_FIELD_POWER4] = min(p.Energy, 100)
	fields[f.UNIT_FIELD_MAXPOWER4] = 100
	fields[f.UNIT_FIELD_FLAGS] = 8
	fields[f.UNIT_FIELD_BYTES_2] = 0x2800
	fields[f.UNIT_MOD_CAST_SPEED] = math.Float32bits(1)
	fields[f.UNIT_FIELD_BOUNDINGRADIUS] = math.Float32bits(0.388999998569489)
	fields[f.UNIT_FIELD_COMBATREACH] = math.Float32bits(1.5)
	fields[f.UNIT_FIELD_DISPLAYID] = p.DisplayID
	fields[f.UNIT_FIELD_NATIVEDISPLAYID] = p.DisplayID
	fields[f.PLAYER_FLAGS] = p.PlayerFlags
	fields[f.PLAYER_BYTES] = p.PlayerBytes
	fields[f.PLAYER_BYTES_2] = p.PlayerBytes2
	fields[f.PLAYER_BYTES_3] = uint32(p.Gender)
	fields[f.PLAYER_FIELD_MAX_LEVEL] = 70
	fields[f.PLAYER_CHARACTER_POINTS2] = 2
	fields[f.PLAYER_FIELD_WATCHED_FACTION_INDEX] = 0xffffffff
	// Visible equipment (slots 0-18). Each visible item field is 16 uint32s
	// apart; only the entry ID is sent — enchant/padding fields are 0.
	for slot, entry := range p.VisibleItems {
		if entry != 0 {
			fields[f.PLAYER_VISIBLE_ITEM_1_0+slot*16] = entry
		}
	}

	w := wire.NewWriter(MaxServerPayload)
	w.Uint32(1) // block count: one object
	w.Uint8(0)  // hasTransport
	w.Uint8(3)  // UPDATETYPE_CREATE_OBJECT2
	w.PackedGUID(uint64(p.GUID))
	w.Uint8(4)    // TYPEID_PLAYER
	w.Uint8(0x70) // updateFlags: LIVING(0x20) | HAS_POSITION(0x40) | HIGHGUID(0x10)
	// MovementInfo: unrooted standing snapshot
	w.Uint32(0)     // movement flags
	w.Uint8(0)      // extra flags
	w.Uint32(clock) // time
	w.Float32(p.X)
	w.Float32(p.Y)
	w.Float32(p.Z)
	w.Float32(p.Orientation)
	w.Uint32(0) // fall time
	// Speeds: walk, run, run_back, swim, swim_back, flight, flight_back, turn_rate
	for _, speed := range []float32{2.5, 7, 4.5, 4.722222, 2.5, 7, 4.5, math.Pi} {
		w.Float32(speed)
	}
	w.Uint32(0) // HIGHGUID field (0 for player)
	writeValues(w, fields)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_UPDATE_OBJECT, Payload: b}
}

// DestroyObject builds an SMSG_DESTROY_OBJECT packet that tells the client
// to remove the object with the given GUID from its scene.
//
// Reference: Object::DestroyForPlayer — body is just the uint64 GUID.
func DestroyObject(guid uint32) world.Packet {
	w := wire.NewWriter(8)
	w.Uint64(uint64(guid))
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_DESTROY_OBJECT, Payload: b}
}
