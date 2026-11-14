// Package spell holds the pure parts of Spell casting: the SpellCastResult
// codes sent in SMSG_CAST_FAILED, effect values (Unit::CalculateSpellDamage)
// and power cost (Spell::CalculatePowerCost).
package spell

import "wowserver/internal/gamedata"

// SpellCastResult values (SharedDefines.h) used by the server.
const (
	FailedBadTargets        = 0x0B
	FailedEquippedItemClass = 0x1B
	FailedNoComboPoints     = 0x49
	FailedOutOfRange        = 0x5C
	FailedTargetsDead       = 0x68
	FailedTargetFriendly    = 0x6E
	FailedTooClose          = 0x7A
	FailedUnitNotInfront    = 0x81
	FailedCasterDead        = 0x15
	FailedDontReport        = 0x19
	FailedError             = 0x1E
	FailedInterrupted       = 0x25
	FailedMoving            = 0x30
	FailedNotKnown          = 0x3B
	FailedNotReady          = 0x3F
	FailedNoPower           = 0x50
	FailedSpellInProgress   = 0x64
	FailedSpellUnavailable  = 0x66
	FailedCasterAuraState   = 0x16
	CastOK                  = 0xFF // custom value, never sent
)

// Power types (Powers).
const (
	PowerMana = iota
	PowerRage
	PowerFocus
	PowerEnergy
	PowerHappiness
	PowerHealth = 0xFFFFFFFE // POWER_HEALTH = -2 as uint32
)

// EffectValue ports Unit::CalculateSpellDamage for a player caster without
// spell mods or combo points. rnd(lo, hi) is irand.
func EffectValue(s *gamedata.SpellInfo, i int, casterLevel uint32, rnd func(lo, hi int32) int32) int32 {
	level := int32(casterLevel)
	if s.MaxLevel > 0 && level > int32(s.MaxLevel) {
		level = int32(s.MaxLevel)
	} else if level < int32(s.BaseLevel) {
		level = int32(s.BaseLevel)
	}
	level -= int32(s.SpellLevel)
	baseDice := int32(s.BaseDice[i])
	basePoints := s.BasePoints[i] + int32(float32(level)*s.RealPointsPerLevel[i])
	randomPoints := int32(float32(s.DieSides[i]) + float32(level)*s.DicePerLevel[i])
	switch randomPoints {
	case 0, 1:
		basePoints += baseDice
	default:
		if baseDice >= randomPoints {
			basePoints += rnd(randomPoints, baseDice)
		} else {
			basePoints += rnd(baseDice, randomPoints)
		}
	}
	return basePoints
}

// ComboScaled adds the EffectPointsPerCombo part of Unit::
// CalculateSpellDamage: finisher damage scales with the spent combo points.
func ComboScaled(s *gamedata.SpellInfo, i int, value int32, comboPoints int32) int32 {
	if s.EffectPointsPerCombo[i] != 0 && comboPoints > 0 {
		value += int32(s.EffectPointsPerCombo[i] * float32(comboPoints))
	}
	return value
}

// PowerCost ports Spell::CalculatePowerCost for a spell cast by a player
// without an item and without spell mods. createHealth/createMana stand for
// GetCreateHealth/GetCreateMana (the level base values).
func PowerCost(s *gamedata.SpellInfo, health, createHealth, createMana uint32, power [5]uint32, maxPower [5]uint32) uint32 {
	const drainAllPower = 0x00000002 // SPELL_ATTR_EX_DRAIN_ALL_POWER
	if s.AttributesEx&drainAllPower != 0 {
		if s.PowerType == PowerHealth {
			return health
		}
		if s.PowerType < 5 {
			return power[s.PowerType]
		}
		return 0
	}
	cost := int32(s.ManaCost)
	if s.ManaCostPct != 0 {
		switch {
		case s.PowerType == PowerHealth:
			cost += int32(s.ManaCostPct) * int32(createHealth) / 100
		case s.PowerType == PowerMana:
			cost += int32(s.ManaCostPct) * int32(createMana) / 100
		case s.PowerType < 5:
			cost += int32(s.ManaCostPct) * int32(maxPower[s.PowerType]) / 100
		default:
			return 0
		}
	}
	if cost < 0 {
		cost = 0
	}
	return uint32(cost)
}
