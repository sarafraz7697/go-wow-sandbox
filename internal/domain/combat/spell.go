package combat

import "math"

// SpellMiss is SpellMissInfo (SharedDefines.h).
type SpellMiss uint8

const (
	SpellMissNone SpellMiss = iota
	SpellMissMiss
	SpellMissResist
	SpellMissDodge
	SpellMissParry
	SpellMissBlock
	SpellMissEvade
	SpellMissImmune
	SpellMissImmune2
	SpellMissDeflect
	SpellMissAbsorb
	SpellMissReflect
)

// SpellHitType flags of SpellNonMeleeDamage::HitInfo.
const SpellHitCrit = 0x2

// MagicHit ports Unit::MagicSpellHitResult for a damaging spell: the level
// difference table, the caster hit bonus (percent) and the victim's hit-taken
// bonus (percent, added like SPELL_AURA_MOD_ATTACKER_SPELL_HIT_CHANCE).
// cantMiss is SPELL_ATTR_EX3_CANT_MISS.
func MagicHit(r Rand, casterLevel, victimLevel uint32, victimIsPlayer bool, hitBonus int32, spellHitPct float32, cantMiss bool) SpellMiss {
	lchance := int32(11)
	if victimIsPlayer {
		lchance = 7
	}
	leveldif := int32(victimLevel) - int32(casterLevel)
	var mod int32
	if leveldif < 3 {
		mod = 96 - leveldif
	} else {
		mod = 94 - (leveldif-2)*lchance
	}
	mod += hitBonus
	hit := mod*100 + int32(spellHitPct*100)
	hit = min(max(hit, 100), 9900)
	tmp := int32(10000 - hit)
	if cantMiss {
		tmp = 0
	}
	if int32(r.Urand(0, 10000)) < tmp {
		return SpellMissResist
	}
	return SpellMissNone
}

// MeleeSpellMissChance ports Unit::MeleeSpellMissChance (percent).
// modHit is m_modMeleeHitChance / m_modRangedHitChance.
func MeleeSpellMissChance(skillDiff int32, victimIsPlayer bool, modHit float32) float32 {
	var hit float32
	switch {
	case victimIsPlayer:
		hit = 95 + float32(skillDiff)*0.04
	case skillDiff < -10:
		hit = 93 + float32(skillDiff+10)*0.4
	default:
		hit = 95 + float32(skillDiff)*0.1
	}
	miss := 100 - hit - modHit
	return min(max(miss, 0), 60)
}

// MeleeSpellInput describes one yellow-hit roll against a creature or player.
type MeleeSpellInput struct {
	AttackerSkill  int32   // weapon skill, or max skill for level when no weapon is required
	VictimMaxSkill int32   // GetMaxSkillValueForLevel of the victim
	VictimDefense  int32   // GetDefenseSkillValue of the victim
	VictimIsPlayer bool    // victim is a player
	BothPlayers    bool    // attacker and victim are players
	ModHit         float32 // attacker m_mod*HitChance
	CantMiss       bool    // SPELL_ATTR_EX3_CANT_MISS
	NoDodgeParry   bool    // SPELL_ATTR_IMPOSSIBLE_DODGE_PARRY_BLOCK
	Ranged         bool
	FromBehind     bool
	VictimNoParry  bool    // CREATURE_FLAG_EXTRA_NO_PARRY
	DodgePct       float32 // GetUnitDodgeChance of the victim
	ParryPct       float32 // GetUnitParryChance of the victim
}

// MeleeSpellHit ports Unit::MeleeSpellHitResult (no mechanic resistance,
// ignore-combat-result auras or expertise).
func MeleeSpellHit(r Rand, in MeleeSpellInput) SpellMiss {
	skillDiff := in.AttackerSkill - in.VictimMaxSkill
	fullSkillDiff := in.AttackerSkill - in.VictimDefense
	roll := int32(r.Urand(0, 10000))
	missChance := int32(MeleeSpellMissChance(fullSkillDiff, in.VictimIsPlayer, in.ModHit) * 100)
	tmp := missChance
	if in.CantMiss {
		tmp = 0
	}
	if roll < tmp {
		return SpellMissMiss
	}
	if in.NoDodgeParry || in.Ranged {
		return SpellMissNone
	}
	canDodge, canParry := true, true
	if in.FromBehind {
		if in.BothPlayers {
			canDodge = false
		}
		canParry = false
	}
	if !in.VictimIsPlayer && in.VictimNoParry {
		canParry = false
	}
	if canDodge {
		dodge := int32(in.DodgePct*100) - skillDiff*4
		tmp += max(dodge, 0)
		if roll < tmp {
			return SpellMissDodge
		}
	}
	if canParry {
		parry := int32(in.ParryPct*100) - skillDiff*4
		tmp += max(parry, 0)
		if roll < tmp {
			return SpellMissParry
		}
	}
	return SpellMissNone
}

// ResistedDamage ports the resistance part of CalculateDamageAbsorbAndResist
// for a non-physical school: the binomial partial resist. resistance is the
// victim's resistance plus the caster's target-resistance modifier.
func ResistedDamage(r Rand, resistance float32, victimLevel uint32, damage uint32, dot bool) uint32 {
	if damage == 0 {
		return 0
	}
	t := resistance * (0.15 / float32(victimLevel))
	t = min(max(t, 0), 0.75)
	ran := float32(r.Urand(0, 100))
	faq := [4]float32{24, 6, 4, 6}
	m := uint32(0)
	var binom float32
	for i := 0; i < 4; i++ {
		binom += 2400 * (float32(math.Pow(float64(t), float64(i))) * float32(math.Pow(float64(1-t), float64(4-i)))) / faq[i]
		if ran > binom {
			m++
		} else {
			break
		}
	}
	var resist uint32
	if dot && m == 4 {
		resist = damage - 1
	} else {
		resist = damage * m / 4
	}
	return min(resist, damage)
}

// CritDamage ports SpellCriticalDamageBonus without spell mods: +100% for
// melee/ranged damage classes, +50% for magic; pctMod is the victim's
// SPELL_AURA_MOD_ATTACKER_*_CRIT_DAMAGE sum.
func CritDamage(damage uint32, physicalClass bool, pctMod int32) uint32 {
	bonus := int32(damage)
	if !physicalClass {
		bonus = int32(damage) / 2
	}
	if pctMod != 0 {
		bonus = int32(float32(bonus) * float32(100+pctMod) / 100)
	}
	return damage + uint32(max(bonus, 0))
}
