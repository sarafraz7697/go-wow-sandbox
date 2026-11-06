package codec

import (
	"math"
	"time"

	"wowserver/internal/entity"
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

// GAMEOBJECT_TYPE_* values that change the create-block layout.
const (
	goTypeTrap        = 6
	goTypeTransport   = 11
	goTypeDuelArbiter = 16
	goTypeFlagStand   = 24
	goTypeFlagDrop    = 26
)

// makeFullGUID combines HIGHGUID, entry and counter into a uint64 using
// the reference ObjectGuid bit layout (ObjectGuid.h):
//
//	ObjectGuid(hi, entry, counter) = counter | (entry << 24) | (hi << 48)
//
// Creature and GameObject GUIDs carry the template entry in bits 24..47.
// The HIGHGUID lives in bits 48..63, so creatures are
// 0xF130xxxxxxxxxxxx and gameobjects 0xF110xxxxxxxxxxxx — the client reads
// the object type from the top 16 bits.
// makeFullGUID mirrors ObjectGuid(HighGuid, entry, counter): for guids that
// carry an entry the counter only owns the low 24 bits (ObjectGuid.h), the
// entry sits at bits 24..47 and the high type at bits 48..63.
func makeFullGUID(high, entry, counter uint32) uint64 {
	return uint64(counter&0xFFFFFF) | uint64(entry&0xFFFFFF)<<24 | uint64(high)<<48
}

// CreateObjectForCreature builds an SMSG_UPDATE_OBJECT packet with a
// UPDATETYPE_CREATE_OBJECT2 block for a creature visible to a player.
//
// Reference: Object::BuildCreateUpdateBlockForPlayer for TYPEID_UNIT (3),
// fed by Creature::LoadFromDB + Creature::SelectLevel — the resolved
// spawn carries the level/health/mana/model/scale/bounding/speed values
// exactly as the reference computes them (gamedata.ResolveCreatureSpawn).
//
// updateFlags = LIVING(0x20) | HAS_POSITION(0x40) | HIGHGUID(0x10) = 0x70
// (Unit::Unit). The UPDATEFLAG_HIGHGUID section for TYPEID_UNIT is the
// default uint32(0) dword — 0x0000000B belongs to the LOWGUID section,
// which is NOT part of flag set 0x70.
func CreateObjectForCreature(spawn gamedata.ResolvedCreatureSpawn, clock uint32) world.Packet {
	fields := make([]uint32, f.UNIT_END)
	guid := makeFullGUID(HighGUIDUnit, spawn.Entry, spawn.GUID)
	fields[f.OBJECT_FIELD_GUID] = uint32(guid)
	fields[f.OBJECT_FIELD_GUID+1] = uint32(guid >> 32)
	fields[f.OBJECT_FIELD_TYPE] = 0x0009 // TYPE_MASK_UNIT = OBJECT | UNIT
	// OBJECT_FIELD_ENTRY (Object::SetEntry): the client reads the template
	// entry from here to send CMSG_CREATURE_QUERY. Without it the name is
	// never resolved and every NPC shows as "Unknown".
	fields[f.OBJECT_FIELD_ENTRY] = spawn.Entry
	fields[f.OBJECT_FIELD_SCALE_X] = math.Float32bits(spawn.Scale)
	// UNIT_FIELD_BYTES_0: race(0) | class(0) | gender(0) | powerType(0) — all 0 for creature
	fields[f.UNIT_FIELD_LEVEL] = spawn.Level
	fields[f.UNIT_FIELD_FACTIONTEMPLATE] = spawn.Faction
	fields[f.UNIT_FIELD_MAXHEALTH] = spawn.MaxHealth
	fields[f.UNIT_FIELD_HEALTH] = spawn.Health
	fields[f.UNIT_FIELD_MAXPOWER1] = spawn.MaxMana
	fields[f.UNIT_FIELD_POWER1] = spawn.Mana
	fields[f.UNIT_FIELD_FLAGS] = spawn.UnitFlags
	fields[f.UNIT_FIELD_BYTES_1] = 0 // stand state 0 (stand)
	fields[f.UNIT_FIELD_BYTES_2] = 0x2800
	fields[f.UNIT_MOD_CAST_SPEED] = math.Float32bits(1)
	fields[f.UNIT_FIELD_BOUNDINGRADIUS] = math.Float32bits(spawn.BoundingRadius)
	fields[f.UNIT_FIELD_COMBATREACH] = math.Float32bits(spawn.CombatReach)
	fields[f.UNIT_FIELD_DISPLAYID] = spawn.DisplayID
	fields[f.UNIT_FIELD_NATIVEDISPLAYID] = spawn.DisplayID
	fields[f.UNIT_FIELD_BASEATTACKTIME] = 2000
	fields[f.UNIT_FIELD_BASEATTACKTIME+1] = 2000
	// NPC flags (UNIT_NPC_FLAGS = 168) come straight from the template so the
	// client shows gossip/vendor/quest markers (the reference sets them when
	// loading the creature before the first values update).
	fields[f.UNIT_NPC_FLAGS] = spawn.NpcFlags

	w := wire.NewWriter(MaxServerPayload)
	w.Uint32(1) // block count: one object
	w.Uint8(0)  // hasTransport
	w.Uint8(2)  // UPDATETYPE_CREATE_OBJECT (creatures use CREATE_OBJECT, not CREATE_OBJECT2)
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
	// Speeds: walk, run, run_back, swim, swim_back, flight, flight_back,
	// turn_rate — creature walk/run come from the template rates over the
	// base movement speeds (UnitSpeed.cpp).
	walk, run := spawn.SpeedWalk, spawn.SpeedRun
	if walk <= 0 {
		walk = 2.5
	}
	if run <= 0 {
		run = 7
	}
	for _, speed := range []float32{walk, run, 4.5, 4.722222, 2.5, 7, 4.5, math.Pi} {
		w.Float32(speed)
	}
	w.Uint32(0) // UPDATEFLAG_HIGHGUID section: TYPEID_UNIT uses the default 0 dword
	writeValues(w, fields)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_UPDATE_OBJECT, Payload: b}
}

// CreateObjectForGameObject builds an SMSG_UPDATE_OBJECT packet with a
// UPDATETYPE_CREATE_OBJECT2 block for a gameobject visible to a player.
//
// Reference: Object::BuildCreateUpdateBlockForPlayer for TYPEID_GAMEOBJECT (5).
// GameObject::GameObject sets m_updateFlag = LOWGUID | HIGHGUID |
// HAS_POSITION = 0x08|0x10|0x40 = 0x58. GOs are not LIVING (no
// MovementInfo/speeds): the create block is flags, position, low GUID,
// high GUID.
//
// Layout:
//
//	uint8  updateFlags (0x58)
//	float  x, y, z, facing  (HAS_POSITION, not LIVING)
//	uint32 lowGUID          (UPDATEFLAG_LOWGUID: GetGUIDLow())
//	uint32 highGUID         (UPDATEFLAG_HIGHGUID: 0xF110)
//	uint8  maskBlockCount, mask, values
//
// GAMEOBJECT_STATE is a values field, NOT a create-block dword.
func CreateObjectForGameObject(guid uint32, data gamedata.GameObjectSpawnRef, tmpl entity.GameObjectInfo) world.Packet {
	fields := make([]uint32, f.GAMEOBJECT_END)
	fullGUID := makeFullGUID(HighGUIDGameObject, data.Data.ID, guid)
	fields[f.OBJECT_FIELD_GUID] = uint32(fullGUID)
	fields[f.OBJECT_FIELD_GUID+1] = uint32(fullGUID >> 32)
	fields[f.OBJECT_FIELD_TYPE] = 0x0021 // TYPE_MASK_GAMEOBJECT = OBJECT | GAMEOBJECT
	// OBJECT_FIELD_ENTRY: lets the client issue CMSG_GAMEOBJECT_QUERY.
	fields[f.OBJECT_FIELD_ENTRY] = data.Data.ID
	scale := tmpl.Size
	if scale <= 0 {
		scale = 1
	}
	fields[f.OBJECT_FIELD_SCALE_X] = math.Float32bits(scale)
	fields[f.GAMEOBJECT_DISPLAYID] = tmpl.DisplayID
	fields[f.GAMEOBJECT_FLAGS] = tmpl.Flags
	// GAMEOBJECT_ROTATION: 4 floats (rotation0..3)
	fields[f.GAMEOBJECT_ROTATION] = math.Float32bits(data.Data.Rotation0)
	fields[f.GAMEOBJECT_ROTATION+1] = math.Float32bits(data.Data.Rotation1)
	fields[f.GAMEOBJECT_ROTATION+2] = math.Float32bits(data.Data.Rotation2)
	fields[f.GAMEOBJECT_ROTATION+3] = math.Float32bits(data.Data.Rotation3)
	fields[f.GAMEOBJECT_STATE] = data.Data.State
	fields[f.GAMEOBJECT_POS_X] = math.Float32bits(data.Data.PosX)
	fields[f.GAMEOBJECT_POS_Y] = math.Float32bits(data.Data.PosY)
	fields[f.GAMEOBJECT_POS_Z] = math.Float32bits(data.Data.PosZ)
	fields[f.GAMEOBJECT_FACING] = math.Float32bits(data.Data.Orientation)
	fields[f.GAMEOBJECT_FACTION] = tmpl.Faction
	fields[f.GAMEOBJECT_TYPE_ID] = tmpl.Type
	fields[f.GAMEOBJECT_ANIMPROGRESS] = data.Data.AnimProgress
	if data.Data.AnimProgress == 0 {
		fields[f.GAMEOBJECT_ANIMPROGRESS] = 100
	}

	// Object::BuildCreateUpdateBlockForPlayer: gameobjects use CREATE_OBJECT2
	// only for traps, duel arbiters and flag stands/drops; TRANSPORT-type
	// gameobjects additionally carry UPDATEFLAG_TRANSPORT (0x02) and a
	// trailing uint32 time.
	updateType := uint8(2) // UPDATETYPE_CREATE_OBJECT
	updateFlags := uint8(0x58)
	switch tmpl.Type {
	case goTypeTrap, goTypeDuelArbiter, goTypeFlagStand, goTypeFlagDrop:
		updateType = 3 // UPDATETYPE_CREATE_OBJECT2
	case goTypeTransport:
		updateFlags |= 0x02
	}

	w := wire.NewWriter(MaxServerPayload)
	w.Uint32(1) // block count
	w.Uint8(0)  // hasTransport
	w.Uint8(updateType)
	w.PackedGUID(fullGUID)
	w.Uint8(5) // TYPEID_GAMEOBJECT
	w.Uint8(updateFlags)
	// Position for non-living objects: 4 floats
	w.Float32(data.Data.PosX)
	w.Float32(data.Data.PosY)
	w.Float32(data.Data.PosZ)
	w.Float32(data.Data.Orientation)
	// UPDATEFLAG_LOWGUID section: GetGUIDLow() for TYPEID_GAMEOBJECT
	w.Uint32(guid)
	// UPDATEFLAG_HIGHGUID section: GetGUIDHigh() for TYPEID_GAMEOBJECT
	w.Uint32(HighGUIDGameObject)
	if updateFlags&0x02 != 0 {
		// UPDATEFLAG_TRANSPORT section: ms time for non-MO transports.
		w.Uint32(uint32(time.Now().UnixMilli()))
	}
	writeValues(w, fields)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_UPDATE_OBJECT, Payload: b}
}
