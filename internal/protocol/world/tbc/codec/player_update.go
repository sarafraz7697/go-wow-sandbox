package codec

import (
	"math"
	"wowserver/internal/domain/character"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
	f "wowserver/internal/protocol/world/tbc/updatefields"
)

// InitialSelfUpdate sends only this player's objects. No shared world state or
// visibility is implied. Indices are generated from the pinned UpdateFields.h.
func InitialSelfUpdate(p *character.Player, data *gamedata.Catalog, clock uint32) (world.Packet, error) {
	if _, ok := data.Map(p.Map); !ok {
		return world.Packet{}, character.ErrPlayerUnsupported
	}
	race, ok := data.Race(uint32(p.Race))
	if !ok {
		return world.Packet{}, character.ErrPlayerUnsupported
	}
	class, ok := data.Class(uint32(p.Class))
	if !ok || class.Power > 4 || p.Gender > 1 {
		return world.Packet{}, character.ErrPlayerUnsupported
	}
	stats := p.Stats
	if stats == nil {
		return world.Packet{}, character.ErrPlayerUnsupported
	}
	base, ok := data.ClassLevel(uint32(p.Class), uint32(p.Level))
	if !ok {
		return world.Packet{}, character.ErrPlayerUnsupported
	}
	xp, ok := data.XP(uint32(p.Level))
	if !ok && p.Level < 70 {
		return world.Packet{}, character.ErrPlayerUnsupported
	}
	fields := make([]uint32, f.PLAYER_END)
	fields[f.OBJECT_FIELD_GUID] = p.GUID
	fields[f.OBJECT_FIELD_TYPE] = 25
	fields[f.OBJECT_FIELD_SCALE_X] = math.Float32bits(1)
	fields[f.UNIT_FIELD_BYTES_0] = uint32(p.Race) | uint32(p.Class)<<8 | uint32(p.Gender)<<16 | class.Power<<24
	fields[f.UNIT_FIELD_LEVEL] = uint32(p.Level)
	fields[f.UNIT_FIELD_FACTIONTEMPLATE] = race.Faction
	fields[f.UNIT_FIELD_MAXHEALTH] = stats.MaxHealth
	fields[f.UNIT_FIELD_HEALTH] = min(p.Health, stats.MaxHealth)
	fields[f.UNIT_FIELD_POWER2] = min(p.Rage, 1000)
	fields[f.UNIT_FIELD_MAXPOWER2] = 1000
	maxMana := stats.MaxMana
	fields[f.UNIT_FIELD_POWER1] = min(p.Mana, maxMana)
	fields[f.UNIT_FIELD_MAXPOWER1] = maxMana
	fields[f.UNIT_FIELD_POWER3] = min(p.Focus, 100)
	fields[f.UNIT_FIELD_MAXPOWER3] = 100
	fields[f.UNIT_FIELD_POWER4] = min(p.Energy, 100)
	fields[f.UNIT_FIELD_MAXPOWER4] = 100
	fields[f.UNIT_FIELD_POWER5] = min(p.Happiness, 1000000)
	fields[f.UNIT_FIELD_MAXPOWER5] = 1000000
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
	fields[f.UNIT_FIELD_BASEATTACKTIME] = stats.AttackTime[0]
	fields[f.UNIT_FIELD_BASEATTACKTIME+1] = stats.AttackTime[1]
	fields[f.UNIT_FIELD_RANGEDATTACKTIME] = stats.AttackTime[2]
	for i, v := range stats.Attributes {
		fields[f.UNIT_FIELD_STAT0+i] = v
		fields[f.PLAYER_FIELD_POSSTAT0+i] = stats.Positive[i]
		fields[f.PLAYER_FIELD_NEGSTAT0+i] = stats.Negative[i]
	}
	for i, v := range stats.Resistances {
		fields[f.UNIT_FIELD_RESISTANCES+i] = v
	}
	fields[f.UNIT_FIELD_ATTACK_POWER] = uint32(stats.AttackPower)
	fields[f.UNIT_FIELD_RANGED_ATTACK_POWER] = uint32(stats.RangedAttackPower)
	fields[f.PLAYER_SHIELD_BLOCK] = stats.ShieldBlock
	for i, at := range []int{f.UNIT_FIELD_MINDAMAGE, f.UNIT_FIELD_MINOFFHANDDAMAGE, f.UNIT_FIELD_MINRANGEDDAMAGE} {
		fields[at] = math.Float32bits(stats.DamageMin[i])
		fields[at+1] = math.Float32bits(stats.DamageMax[i])
	}
	for i := 0; i < 7; i++ {
		fields[f.PLAYER_FIELD_MOD_DAMAGE_DONE_PCT+i] = math.Float32bits(1)
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
	items := make(map[uint32]character.PlayerItem, len(p.Items))
	for _, item := range p.Items {
		if _, duplicate := items[item.GUID]; duplicate {
			return world.Packet{}, character.ErrPlayerUnsupported
		}
		items[item.GUID] = item
	}
	w.Uint32(uint32(1 + len(p.Items)))
	w.Uint8(0)
	for _, item := range p.Items {
		details, ok := data.ItemPrototype(item.Entry)
		if !ok {
			return world.Packet{}, character.ErrPlayerUnsupported
		}
		itemFields := append([]uint32(nil), item.Fields[:]...)
		objectType := uint8(1)
		if details.ContainerSlots > 0 {
			if details.ContainerSlots > 36 || item.Bag != 0 {
				return world.Packet{}, character.ErrPlayerUnsupported
			}
			objectType = 2
			itemFields = append(itemFields, make([]uint32, f.CONTAINER_END-f.ITEM_END)...)
			itemFields[f.OBJECT_FIELD_TYPE] = 7
			itemFields[f.CONTAINER_FIELD_NUM_SLOTS] = details.ContainerSlots
			for _, child := range p.Items {
				if child.Bag == item.GUID {
					if uint32(child.Slot) >= details.ContainerSlots {
						return world.Packet{}, character.ErrPlayerUnsupported
					}
					at := f.CONTAINER_FIELD_SLOT_1 + int(child.Slot)*2
					itemFields[at], itemFields[at+1] = child.GUID, 0x40000000
				}
			}
		}
		guid := uint64(item.GUID) | uint64(0x40000000)<<32
		if item.Bag == 0 {
			if item.Slot >= 118 {
				return world.Packet{}, character.ErrPlayerUnsupported
			}
			fields[f.PLAYER_FIELD_INV_SLOT_HEAD+int(item.Slot)*2] = item.GUID
			fields[f.PLAYER_FIELD_INV_SLOT_HEAD+int(item.Slot)*2+1] = 0x40000000
			itemFields[f.ITEM_FIELD_CONTAINED], itemFields[f.ITEM_FIELD_CONTAINED+1] = p.GUID, 0
		} else {
			parent, exists := items[item.Bag]
			parentDetails, valid := data.ItemPrototype(parent.Entry)
			if !exists || !valid || parent.Bag != 0 || uint32(item.Slot) >= parentDetails.ContainerSlots {
				return world.Packet{}, character.ErrPlayerUnsupported
			}
			itemFields[f.ITEM_FIELD_CONTAINED], itemFields[f.ITEM_FIELD_CONTAINED+1] = item.Bag, 0x40000000
		}
		if item.Bag == 0 && item.Slot < 19 {
			at := f.PLAYER_VISIBLE_ITEM_1_0 + int(item.Slot)*16
			fields[at] = item.Entry
			for i := 0; i < 7; i++ {
				fields[at+1+i] = item.Fields[22+i*3]
			}
			fields[at+12] = item.Fields[56]
		}
		w.Uint8(2)
		w.PackedGUID(guid)
		w.Uint8(objectType)
		w.Uint8(0x18)
		w.Uint32(item.GUID)
		w.Uint32(0x4000)
		writeValues(w, itemFields)
	}
	w.Uint8(3)
	w.PackedGUID(uint64(p.GUID))
	w.Uint8(4)
	w.Uint8(0x71)
	// Unrooted movement snapshot: flags, extra flags, clock, position,
	// fall time, eight movement speeds, then UPDATEFLAG_HIGHGUID's zero word.
	w.Uint32(0)
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
