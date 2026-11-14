package codec

import (
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

// SpellCastTargetFlags (DBCEnums.h).
const (
	TargetFlagSelf           = 0x00000000
	TargetFlagUnit           = 0x00000002
	TargetFlagItem           = 0x00000010
	TargetFlagSourceLocation = 0x00000020
	TargetFlagDestLocation   = 0x00000040
	TargetFlagObjectUnk      = 0x00000080
	TargetFlagPvPCorpse      = 0x00000200
	TargetFlagObject         = 0x00000800
	TargetFlagTradeItem      = 0x00001000
	TargetFlagString         = 0x00002000
	TargetFlagGameObjectItem = 0x00004000
	TargetFlagCorpse         = 0x00008000
	TargetFlagUnk2           = 0x00010000
)

// Cast flags used on the wire (Spell.h).
const (
	CastFlagUnknown2 = 0x00000002
	CastFlagUnknown9 = 0x00000100
)

// SpellTargets is SpellCastTargets.
type SpellTargets struct {
	Mask                   uint32
	Unit, GO, Item, Corpse uint64
	Src, Dest              [3]float32
	Str                    string
}

// ReadSpellTargets ports SpellCastTargets::read (self mask = 0 targets the caster).
func ReadSpellTargets(r *wire.Reader, caster uint64) SpellTargets {
	var t SpellTargets
	t.Mask = r.Uint32()
	if t.Mask == TargetFlagSelf {
		t.Unit = caster
		return t
	}
	if t.Mask&(TargetFlagUnit|TargetFlagUnk2) != 0 {
		t.Unit = r.PackedGUID()
	}
	if t.Mask&(TargetFlagObject|TargetFlagObjectUnk|TargetFlagGameObjectItem) != 0 {
		t.GO = r.PackedGUID()
	}
	if t.Mask&(TargetFlagItem|TargetFlagTradeItem) != 0 {
		t.Item = r.PackedGUID()
	}
	if t.Mask&TargetFlagSourceLocation != 0 {
		t.Src = [3]float32{r.Float32(), r.Float32(), r.Float32()}
	}
	if t.Mask&TargetFlagDestLocation != 0 {
		t.Dest = [3]float32{r.Float32(), r.Float32(), r.Float32()}
	}
	if t.Mask&TargetFlagString != 0 {
		t.Str = r.CString(256)
	}
	if t.Mask&(TargetFlagCorpse|TargetFlagPvPCorpse) != 0 {
		t.Corpse = r.PackedGUID()
	}
	return t
}

// Write ports SpellCastTargets::write.
func (t SpellTargets) Write(w *wire.Writer) {
	w.Uint32(t.Mask)
	if t.Mask&(TargetFlagUnit|TargetFlagPvPCorpse|TargetFlagObject|TargetFlagCorpse|TargetFlagUnk2) != 0 {
		switch {
		case t.Mask&TargetFlagUnit != 0:
			w.PackedGUID(t.Unit)
		case t.Mask&(TargetFlagObject|TargetFlagObjectUnk) != 0:
			w.PackedGUID(t.GO)
		case t.Mask&(TargetFlagCorpse|TargetFlagPvPCorpse) != 0:
			w.PackedGUID(t.Corpse)
		default:
			w.Uint8(0)
		}
	}
	if t.Mask&(TargetFlagItem|TargetFlagTradeItem) != 0 {
		w.PackedGUID(t.Item)
	}
	if t.Mask&TargetFlagSourceLocation != 0 {
		w.Float32(t.Src[0])
		w.Float32(t.Src[1])
		w.Float32(t.Src[2])
	}
	if t.Mask&TargetFlagDestLocation != 0 {
		w.Float32(t.Dest[0])
		w.Float32(t.Dest[1])
		w.Float32(t.Dest[2])
	}
	if t.Mask&TargetFlagString != 0 {
		w.CString(t.Str, 256)
	}
}

func build(opcode uint32, size int, fill func(*wire.Writer)) world.Packet {
	w := wire.NewWriter(size)
	fill(w)
	b, _ := w.Result()
	return world.Packet{Opcode: opcode, Payload: b}
}

// SpellStart is Spell::SendSpellStart (no ammo block: ranged spells are not cast).
func SpellStart(source, caster uint64, spell uint32, count uint8, timer uint32, t SpellTargets) world.Packet {
	return build(op.SMSG_SPELL_START, 40, func(w *wire.Writer) {
		w.PackedGUID(source)
		w.PackedGUID(caster)
		w.Uint32(spell)
		w.Uint8(count)
		w.Uint16(CastFlagUnknown2)
		w.Uint32(timer)
		t.Write(w)
	})
}

// SpellGo is Spell::SendSpellGo with every target hit (no misses).
func SpellGo(source, caster uint64, spell uint32, timestamp uint32, hit []uint64, t SpellTargets) world.Packet {
	return SpellGoMiss(source, caster, spell, timestamp, hit, nil, t)
}

// SpellMissEntry is one entry of the miss list of WriteSpellGoTargets.
type SpellMissEntry struct {
	Target        uint64
	Reason        uint8
	ReflectResult uint8
}

// SpellGoMiss is Spell::SendSpellGo with hit and miss target lists.
func SpellGoMiss(source, caster uint64, spell uint32, timestamp uint32, hit []uint64, miss []SpellMissEntry, t SpellTargets) world.Packet {
	return build(op.SMSG_SPELL_GO, 50, func(w *wire.Writer) {
		w.PackedGUID(source)
		w.PackedGUID(caster)
		w.Uint32(spell)
		w.Uint16(CastFlagUnknown9)
		w.Uint32(timestamp)
		w.Uint8(uint8(len(hit)))
		for _, g := range hit {
			w.Uint64(g)
		}
		w.Uint8(uint8(len(miss)))
		for _, m := range miss {
			w.Uint64(m.Target)
			w.Uint8(m.Reason)
			if m.Reason == 11 { // SPELL_MISS_REFLECT
				w.Uint8(m.ReflectResult)
			}
		}
		t.Write(w)
	})
}

// SpellNonMeleeDamageLog is Unit::SendSpellNonMeleeDamageLog.
func SpellNonMeleeDamageLog(target, attacker uint64, spell, damage uint32, school uint8, absorb, resist uint32, physicalLog bool, blocked, hitInfo uint32) world.Packet {
	return build(op.SMSG_SPELLNONMELEEDAMAGELOG, 48, func(w *wire.Writer) {
		w.PackedGUID(target)
		w.PackedGUID(attacker)
		w.Uint32(spell)
		w.Uint32(damage)
		w.Uint8(school)
		w.Uint32(absorb)
		w.Uint32(resist)
		if physicalLog {
			w.Uint8(1)
		} else {
			w.Uint8(0)
		}
		w.Uint8(0) // unused
		w.Uint32(blocked)
		w.Uint32(hitInfo)
		w.Uint8(0) // flag to use extend data
	})
}

// SetExtraAuraInfoNeedUpdate is SpellAuraHolder::SendAuraDurationForCaster.
func SetExtraAuraInfoNeedUpdate(target uint64, slot uint8, spell, max, remaining uint32) world.Packet {
	return build(op.SMSG_SET_EXTRA_AURA_INFO_NEED_UPDATE, 25, func(w *wire.Writer) {
		w.PackedGUID(target)
		w.Uint8(slot)
		w.Uint32(spell)
		w.Uint32(max)
		w.Uint32(remaining)
	})
}

// PeriodicDamageLog is Unit::SendPeriodicAuraLog for SPELL_AURA_PERIODIC_DAMAGE.
func PeriodicDamageLog(target, caster uint64, spell, damage, school, absorb, resist uint32) world.Packet {
	return build(op.SMSG_PERIODICAURALOG, 44, func(w *wire.Writer) {
		w.PackedGUID(target)
		w.PackedGUID(caster)
		w.Uint32(spell)
		w.Uint32(1)
		w.Uint32(3) // SPELL_AURA_PERIODIC_DAMAGE
		w.Uint32(damage)
		w.Uint32(school)
		w.Uint32(absorb)
		w.Uint32(resist)
	})
}

// ClearExtraAuraInfo is the SMSG_CLEAR_EXTRA_AURA_INFO sent for SPELL_CAST_OK.
func ClearExtraAuraInfo(caster uint64, spell uint32) world.Packet {
	return build(op.SMSG_CLEAR_EXTRA_AURA_INFO, 12, func(w *wire.Writer) { w.PackedGUID(caster); w.Uint32(spell) })
}

// CastFailed is SMSG_CAST_FAILED (no result specific trailer).
func CastFailed(spell uint32, result, count uint8) world.Packet {
	return build(op.SMSG_CAST_FAILED, 6, func(w *wire.Writer) { w.Uint32(spell); w.Uint8(result); w.Uint8(count) })
}

// SpellFailure and SpellFailedOther are Spell::SendInterrupted.
func SpellFailure(caster uint64, spell uint32, result uint8) world.Packet {
	return build(op.SMSG_SPELL_FAILURE, 14, func(w *wire.Writer) { w.PackedGUID(caster); w.Uint32(spell); w.Uint8(result) })
}
func SpellFailedOther(caster uint64, spell uint32) world.Packet {
	return build(op.SMSG_SPELL_FAILED_OTHER, 12, func(w *wire.Writer) { w.PackedGUID(caster); w.Uint32(spell) })
}

// SpellHealLog is Unit::SendHealSpellLog.
func SpellHealLog(victim, caster uint64, spell, heal uint32, crit bool) world.Packet {
	return build(op.SMSG_SPELLHEALLOG, 26, func(w *wire.Writer) {
		w.PackedGUID(victim)
		w.PackedGUID(caster)
		w.Uint32(spell)
		w.Uint32(heal)
		if crit {
			w.Uint8(1)
		} else {
			w.Uint8(0)
		}
		w.Uint8(0)
	})
}

// SpellEnergizeLog is Unit::SendEnergizeSpellLog.
func SpellEnergizeLog(victim, caster uint64, spell, power, amount uint32) world.Packet {
	return build(op.SMSG_SPELLENERGIZELOG, 29, func(w *wire.Writer) {
		w.PackedGUID(victim)
		w.PackedGUID(caster)
		w.Uint32(spell)
		w.Uint32(power)
		w.Uint32(amount)
	})
}

// PeriodicAuraLog is SMSG_PERIODICAURALOG for a periodic heal or energize tick
// (Aura::HandlePeriodicHeal / HandlePeriodicEnergize).
func PeriodicHealLog(target, caster uint64, spell, amount uint32) world.Packet {
	return build(op.SMSG_PERIODICAURALOG, 36, func(w *wire.Writer) {
		w.PackedGUID(target)
		w.PackedGUID(caster)
		w.Uint32(spell)
		w.Uint32(1) // count
		w.Uint32(8) // SPELL_AURA_PERIODIC_HEAL
		w.Uint32(amount)
	})
}
func PeriodicEnergizeLog(target, caster uint64, spell, power, amount uint32) world.Packet {
	return build(op.SMSG_PERIODICAURALOG, 40, func(w *wire.Writer) {
		w.PackedGUID(target)
		w.PackedGUID(caster)
		w.Uint32(spell)
		w.Uint32(1)
		w.Uint32(24) // SPELL_AURA_PERIODIC_ENERGIZE
		w.Uint32(power)
		w.Uint32(amount)
	})
}

// UpdateComboPoints is Player::SendComboPoints (SMSG_UPDATE_COMBO_POINTS).
func UpdateComboPoints(target uint64, points uint8) world.Packet {
	return build(op.SMSG_UPDATE_COMBO_POINTS, 10, func(w *wire.Writer) {
		w.PackedGUID(target)
		w.Uint8(points)
	})
}

// DeathReleaseLoc is SMSG_DEATH_RELEASE_LOC: the spirit healer position on
// the minimap (map -1 clears it, Player::RepopAtGraveyard/ResurrectPlayer).
func DeathReleaseLoc(mapID uint32, x, y, z float32) world.Packet {
	return build(op.SMSG_DEATH_RELEASE_LOC, 16, func(w *wire.Writer) {
		w.Uint32(mapID)
		w.Float32(x)
		w.Float32(y)
		w.Float32(z)
	})
}

// DeathReleaseLocClear removes the spirit healer marker.
func DeathReleaseLocClear() world.Packet {
	return build(op.SMSG_DEATH_RELEASE_LOC, 16, func(w *wire.Writer) {
		w.Uint32(0xFFFFFFFF)
		w.Float32(0)
		w.Float32(0)
		w.Float32(0)
	})
}

// ResurrectRequest is Spell::SendResurrectRequest (SMSG_RESURRECT_REQUEST):
// the caster guid, the name length, the name, a zero byte and the is-npc flag.
func ResurrectRequest(caster uint64, name string, isNPC bool) world.Packet {
	return build(op.SMSG_RESURRECT_REQUEST, 15+len(name), func(w *wire.Writer) {
		w.Uint64(caster)
		w.Uint32(uint32(len(name)) + 1)
		w.CString(name, 256)
		w.Uint8(0)
		if isNPC {
			w.Uint8(1)
		} else {
			w.Uint8(0)
		}
	})
}

// CooldownEntry is one (spell, remaining ms) pair of SMSG_SPELL_COOLDOWN.
type CooldownEntry struct {
        Spell  uint32
        Remain uint32
}

// SpellCooldownPacket is the SMSG_SPELL_COOLDOWN form of Player::
// ProhibitSpellSchool: full guid, flags byte, then (spell, ms) pairs.
func SpellCooldownPacket(guid uint64, entries []CooldownEntry) world.Packet {
        return build(op.SMSG_SPELL_COOLDOWN, 9+len(entries)*8, func(w *wire.Writer) {
                w.Uint64(guid)
                w.Uint8(0) // flags: 0x1 pet, 0x2 unk
                for _, e := range entries {
                        w.Uint32(e.Spell)
                        w.Uint32(e.Remain)
                }
        })
}

// ClearCooldownPacket is Player::SendClearCooldown (SMSG_CLEAR_COOLDOWN).
func ClearCooldownPacket(spell uint32, guid uint64) world.Packet {
        return build(op.SMSG_CLEAR_COOLDOWN, 12, func(w *wire.Writer) {
                w.Uint32(spell)
                w.Uint64(guid)
        })
}

// UpdateAuraDuration and SetExtraAuraInfo are SpellAuraHolder::UpdateAuraDuration.
func UpdateAuraDuration(slot uint8, ms uint32) world.Packet {
	return build(op.SMSG_UPDATE_AURA_DURATION, 5, func(w *wire.Writer) { w.Uint8(slot); w.Uint32(ms) })
}
func SetExtraAuraInfo(target uint64, slot uint8, spell, max, remaining uint32) world.Packet {
	return build(op.SMSG_SET_EXTRA_AURA_INFO, 25, func(w *wire.Writer) {
		w.PackedGUID(target)
		w.Uint8(slot)
		w.Uint32(spell)
		w.Uint32(max)
		w.Uint32(remaining)
	})
}
