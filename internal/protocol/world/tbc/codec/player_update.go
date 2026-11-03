package codec

import (
	"math"
	"wowserver/internal/character"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
	f "wowserver/internal/protocol/world/tbc/updatefields"
)

// InitialSelfUpdate sends only this player's objects. No shared world state or
// visibility is implied. Indices are generated from the pinned UpdateFields.h.
func InitialSelfUpdate(p *character.Player, data *gamedata.Catalog, clock uint32) (world.Packet, error) {
	race, ok := data.Race(uint32(p.Race))
	if !ok {
		return world.Packet{}, character.ErrPlayerUnsupported
	}
	level, ok := data.Level(uint32(p.Race), uint32(p.Class), uint32(p.Level))
	if !ok {
		return world.Packet{}, character.ErrPlayerUnsupported
	}
	base, ok := data.ClassLevel(uint32(p.Class), uint32(p.Level))
	if !ok {
		return world.Packet{}, character.ErrPlayerUnsupported
	}
	xp, ok := data.XP(uint32(p.Level))
	if !ok {
		return world.Packet{}, character.ErrPlayerUnsupported
	}
	fields := make([]uint32, f.PLAYER_END)
	fields[f.OBJECT_FIELD_GUID] = p.GUID
	fields[f.OBJECT_FIELD_TYPE] = 25
	fields[f.OBJECT_FIELD_SCALE_X] = math.Float32bits(1)
	fields[f.UNIT_FIELD_BYTES_0] = uint32(p.Race) | uint32(p.Class)<<8 | uint32(p.Gender)<<16 | 1<<24
	fields[f.UNIT_FIELD_LEVEL] = uint32(p.Level)
	fields[f.UNIT_FIELD_FACTIONTEMPLATE] = race.Faction
	stamina := level.Stats[2]
	bonus := stamina
	if stamina > 20 {
		bonus = 20 + 10*(stamina-20)
	}
	fields[f.UNIT_FIELD_MAXHEALTH] = base.Health + bonus
	fields[f.UNIT_FIELD_HEALTH] = min(p.Health, base.Health+bonus)
	fields[f.UNIT_FIELD_POWER2] = min(p.Rage, 1000)
	fields[f.UNIT_FIELD_MAXPOWER2] = 1000
	fields[f.UNIT_FIELD_BASE_HEALTH] = base.Health
	fields[f.UNIT_FIELD_BASE_MANA] = base.Mana
	fields[f.UNIT_FIELD_FLAGS] = 8
	fields[f.UNIT_FIELD_BYTES_2] = 0x2800
	fields[f.UNIT_MOD_CAST_SPEED] = math.Float32bits(1)
	fields[f.UNIT_FIELD_BOUNDINGRADIUS] = math.Float32bits(0.388999998569489)
	fields[f.UNIT_FIELD_COMBATREACH] = math.Float32bits(1.5)
	display := race.MaleDisplay
	if p.Gender == 1 {
		display = race.FemaleDisplay
	}
	fields[f.UNIT_FIELD_DISPLAYID] = display
	fields[f.UNIT_FIELD_NATIVEDISPLAYID] = display
	fields[f.UNIT_FIELD_BASEATTACKTIME] = 2000
	fields[f.UNIT_FIELD_BASEATTACKTIME+1] = 2000
	fields[f.UNIT_FIELD_RANGEDATTACKTIME] = 2000
	for i, v := range level.Stats {
		fields[f.UNIT_FIELD_STAT0+i] = v
	}
	fields[f.PLAYER_FLAGS] = p.PlayerFlags
	fields[f.PLAYER_BYTES] = p.PlayerBytes
	fields[f.PLAYER_BYTES_2] = p.PlayerBytes2
	fields[f.PLAYER_BYTES_3] = uint32(p.Gender)
	fields[f.PLAYER_XP] = p.XP
	fields[f.PLAYER_NEXT_LEVEL_XP] = xp
	fields[f.PLAYER_FIELD_COINAGE] = p.Money
	fields[f.PLAYER_CHARACTER_POINTS2] = 2
	fields[f.PLAYER_FIELD_MAX_LEVEL] = 70
	fields[f.PLAYER_FIELD_WATCHED_FACTION_INDEX] = 0xffffffff
	for i, v := range p.Skills {
		if i >= 128 {
			return world.Packet{}, character.ErrPlayerUnsupported
		}
		fields[f.PLAYER_SKILL_INFO_1_1+i*3] = v.ID
		fields[f.PLAYER_SKILL_INFO_1_1+i*3+1] = v.Value | v.Max<<16
	}
	w := wire.NewWriter(MaxServerPayload)
	w.Uint32(uint32(1 + len(p.Items)))
	w.Uint8(0)
	for _, item := range p.Items {
		if _, ok := data.ItemDetails(item.Entry); !ok {
			return world.Packet{}, character.ErrPlayerUnsupported
		}
		guid := uint64(item.GUID) | uint64(0x40000000)<<32
		fields[f.PLAYER_FIELD_INV_SLOT_HEAD+int(item.Slot)*2] = item.GUID
		fields[f.PLAYER_FIELD_INV_SLOT_HEAD+int(item.Slot)*2+1] = 0x40000000
		if item.Slot < 19 {
			at := f.PLAYER_VISIBLE_ITEM_1_0 + int(item.Slot)*16
			fields[at] = item.Entry
			for i := 0; i < 7; i++ {
				fields[at+1+i] = item.Fields[22+i*3]
			}
			fields[at+12] = item.Fields[56]
		}
		w.Uint8(2)
		w.PackedGUID(guid)
		w.Uint8(1)
		w.Uint8(0x18)
		w.Uint32(item.GUID)
		w.Uint32(0x4000)
		writeValues(w, item.Fields[:])
	}
	w.Uint8(3)
	w.PackedGUID(uint64(p.GUID))
	w.Uint8(4)
	w.Uint8(0x71)
	// Rooted stationary movement snapshot: flags, extra flags, clock, position,
	// fall time, eight movement speeds, then UPDATEFLAG_HIGHGUID's zero word.
	w.Uint32(0x800)
	w.Uint8(0)
	w.Uint32(clock)
	w.Float32(p.X)
	w.Float32(p.Y)
	w.Float32(p.Z)
	w.Float32(p.Orientation)
	w.Uint32(0)
	for _, speed := range []float32{2.5, 7, 4.5, 4.722222, 2.5, 7, 4.5, math.Pi} {
		w.Float32(speed)
	}
	w.Uint32(0)
	writeValues(w, fields)
	b, err := w.Result()
	return world.Packet{Opcode: op.SMSG_UPDATE_OBJECT, Payload: b}, err
}
func writeValues(w *wire.Writer, values []uint32) {
	mask := make([]uint32, (len(values)+31)/32)
	for i, v := range values {
		if v != 0 {
			mask[i/32] |= 1 << uint(i%32)
		}
	}
	w.Uint8(uint8(len(mask)))
	for _, v := range mask {
		w.Uint32(v)
	}
	for _, v := range values {
		if v != 0 {
			w.Uint32(v)
		}
	}
}
