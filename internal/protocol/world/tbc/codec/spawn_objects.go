package codec

import (
	"math"

	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
	f "wowserver/internal/protocol/world/tbc/updatefields"
)

// HighGUID constants for packed GUID construction.
// Reference: ObjectGuid.h HIGHGUID_*.
const (
	HighGUIDUnit       = 0xF130
	HighGUIDGameObject = 0xF110
)

// makeFullGUID combines a HIGHGUID and a low GUID into a uint64.
// TBC ObjectGuid is (high << 48) | (entry << 32) | low, but for creatures
// the entry is 0 (only the low spawn GUID is used); for gameobjects same.
// The client reads the high GUID from the high 16 bits.
func makeFullGUID(high uint32, low uint32) uint64 {
	return uint64(high)<<32 | uint64(low)
}

// CreateObjectForCreature builds an SMSG_UPDATE_OBJECT packet with a
// UPDATETYPE_CREATE_OBJECT2 block for a creature visible to a player.
//
// Reference: Object::BuildCreateUpdateBlockForPlayer for TYPEID_UNIT (3).
// updateFlags = LIVING(0x20) | HAS_POSITION(0x40) | HIGHGUID(0x10) = 0x70.
// The HIGHGUID field for TYPEID_UNIT is 0x0000000B.
//
// MovementInfo for a creature: flags(0) extra(0) time x y z o fallTime(0)
// + 8 speeds + HIGHGUID word (0x0B for unit).
func CreateObjectForCreature(spawn gamedata.CreatureSpawn, tmpl gamedata.CreatureTemplate, clock uint32) world.Packet {
	fields := make([]uint32, f.UNIT_END)
	guid := makeFullGUID(HighGUIDUnit, spawn.GUID)
	fields[f.OBJECT_FIELD_GUID] = uint32(guid)
	fields[f.OBJECT_FIELD_GUID+1] = uint32(guid >> 32)
	fields[f.OBJECT_FIELD_TYPE] = 0x0009 // TYPE_MASK_UNIT = OBJECT | UNIT
	fields[f.OBJECT_FIELD_SCALE_X] = math.Float32bits(1)
	// UNIT_FIELD_BYTES_0: race(0) | class(0) | gender(0) | powerType(0) — all 0 for creature
	fields[f.UNIT_FIELD_LEVEL] = tmpl.MinLevel
	fields[f.UNIT_FIELD_FACTIONTEMPLATE] = tmpl.FactionAlliance
	fields[f.UNIT_FIELD_MAXHEALTH] = tmpl.MaxHealth
	health := spawn.Health
	if health == 0 || health > tmpl.MaxHealth {
		health = tmpl.MaxHealth
	}
	fields[f.UNIT_FIELD_HEALTH] = health
	fields[f.UNIT_FIELD_MAXPOWER1] = tmpl.MaxMana
	fields[f.UNIT_FIELD_POWER1] = spawn.Mana
	fields[f.UNIT_FIELD_FLAGS] = tmpl.UnitFlags
	fields[f.UNIT_FIELD_BYTES_1] = 0 // stand state 0 (stand)
	fields[f.UNIT_FIELD_BYTES_2] = 0x2800
	fields[f.UNIT_MOD_CAST_SPEED] = math.Float32bits(1)
	fields[f.UNIT_FIELD_BOUNDINGRADIUS] = math.Float32bits(0.5)
	fields[f.UNIT_FIELD_COMBATREACH] = math.Float32bits(1.5)
	fields[f.UNIT_FIELD_DISPLAYID] = tmpl.ModelID
	fields[f.UNIT_FIELD_NATIVEDISPLAYID] = tmpl.ModelID
	fields[f.UNIT_FIELD_BASEATTACKTIME] = 2000
	fields[f.UNIT_FIELD_BASEATTACKTIME+1] = 2000
	// NPC_FLAGS (at UNIT_NPC_FLAGS)
	// UNIT_NPC_FLAGS = 0x94+... actually at offset 95 (UNIT_FIELD_FLAGS_2? no)
	// From UpdateFields.h: UNIT_NPC_FLAGS = 95, but our fields_gen uses
	// UNIT_FIELD_FLAGS at 43. For simplicity we set NPC flags at the
	// correct field. Let me check the field index.

	w := wire.NewWriter(MaxServerPayload)
	w.Uint32(1) // block count: one object
	w.Uint8(0)  // hasTransport
	w.Uint8(3)  // UPDATETYPE_CREATE_OBJECT2
	w.PackedGUID(guid)
	w.Uint8(3)    // TYPEID_UNIT
	w.Uint8(0x70) // updateFlags: LIVING|HAS_POSITION|HIGHGUID
	// MovementInfo: unrooted standing snapshot
	w.Uint32(0)     // movement flags
	w.Uint8(0)      // extra flags
	w.Uint32(clock) // time
	w.Float32(spawn.X)
	w.Float32(spawn.Y)
	w.Float32(spawn.Z)
	w.Float32(spawn.Orientation)
	w.Uint32(0) // fall time
	// Speeds: walk, run, run_back, swim, swim_back, flight, flight_back, turn_rate
	for _, speed := range []float32{2.5, 7, 4.5, 4.722222, 2.5, 7, 4.5, math.Pi} {
		w.Float32(speed)
	}
	w.Uint32(0x0000000B) // HIGHGUID field for TYPEID_UNIT
	writeValues(w, fields)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_UPDATE_OBJECT, Payload: b}
}

// CreateObjectForGameObject builds an SMSG_UPDATE_OBJECT packet with a
// UPDATETYPE_CREATE_OBJECT2 block for a gameobject visible to a player.
//
// Reference: Object::BuildCreateUpdateBlockForPlayer for TYPEID_GAMEOBJECT (5).
// updateFlags = HAS_POSITION(0x40) | HIGHGUID(0x10) = 0x50.
// GOs are not LIVING (no MovementInfo/speeds); only position + rotation.
//
// Layout:
//
//	uint8  updateFlags (0x50)
//	float  x, y, z, facing  (only when HAS_POSITION and not LIVING)
//	uint32 highGUID         (0xF110 for gameobject)
//	uint32 gameObjectState   (GAMEOBJECT_STATE)
//	uint8  maskBlockCount, mask, values
func CreateObjectForGameObject(spawn gamedata.GameObjectSpawn, tmpl gamedata.GameObjectTemplate) world.Packet {
	fields := make([]uint32, f.GAMEOBJECT_END)
	guid := makeFullGUID(HighGUIDGameObject, spawn.GUID)
	fields[f.OBJECT_FIELD_GUID] = uint32(guid)
	fields[f.OBJECT_FIELD_GUID+1] = uint32(guid >> 32)
	fields[f.OBJECT_FIELD_TYPE] = 0x0021 // TYPE_MASK_GAMEOBJECT = OBJECT | GAMEOBJECT
	scale := tmpl.Size
	if scale <= 0 {
		scale = 1
	}
	fields[f.OBJECT_FIELD_SCALE_X] = math.Float32bits(scale)
	fields[f.GAMEOBJECT_DISPLAYID] = tmpl.DisplayID
	fields[f.GAMEOBJECT_FLAGS] = tmpl.Flags
	// GAMEOBJECT_ROTATION: 4 floats (rotation0..3)
	fields[f.GAMEOBJECT_ROTATION] = math.Float32bits(spawn.Rotation0)
	fields[f.GAMEOBJECT_ROTATION+1] = math.Float32bits(spawn.Rotation1)
	fields[f.GAMEOBJECT_ROTATION+2] = math.Float32bits(spawn.Rotation2)
	fields[f.GAMEOBJECT_ROTATION+3] = math.Float32bits(spawn.Rotation3)
	fields[f.GAMEOBJECT_STATE] = spawn.State
	fields[f.GAMEOBJECT_POS_X] = math.Float32bits(spawn.X)
	fields[f.GAMEOBJECT_POS_Y] = math.Float32bits(spawn.Y)
	fields[f.GAMEOBJECT_POS_Z] = math.Float32bits(spawn.Z)
	fields[f.GAMEOBJECT_FACING] = math.Float32bits(spawn.Orientation)
	fields[f.GAMEOBJECT_FACTION] = tmpl.Faction
	fields[f.GAMEOBJECT_TYPE_ID] = tmpl.Type
	fields[f.GAMEOBJECT_ANIMPROGRESS] = spawn.AnimProgress
	if spawn.AnimProgress == 0 {
		fields[f.GAMEOBJECT_ANIMPROGRESS] = 100
	}

	w := wire.NewWriter(MaxServerPayload)
	w.Uint32(1) // block count
	w.Uint8(0)  // hasTransport
	w.Uint8(3)  // UPDATETYPE_CREATE_OBJECT2
	w.PackedGUID(guid)
	w.Uint8(5)    // TYPEID_GAMEOBJECT
	w.Uint8(0x50) // updateFlags: HAS_POSITION | HIGHGUID (no LIVING)
	// Position for non-living objects: 4 floats
	w.Float32(spawn.X)
	w.Float32(spawn.Y)
	w.Float32(spawn.Z)
	w.Float32(spawn.Orientation)
	// HIGHGUID field for TYPEID_GAMEOBJECT
	w.Uint32(HighGUIDGameObject)
	// GAMEOBJECT_STATE (go_state) — written as uint32 after HIGHGUID
	w.Uint32(spawn.State)
	writeValues(w, fields)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_UPDATE_OBJECT, Payload: b}
}
