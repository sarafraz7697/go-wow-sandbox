package gamedata

import "sort"

// SpellInfo is the subset of SpellEntry (DBCStructure.h, Spell.dbc) used by
// casting and auras. Column numbers follow the reference struct comments.
type SpellInfo struct {
	ID, Category, Mechanic                  uint32
	Attributes, AttributesEx, AttributesExB uint32
	Targets, CastingTimeIndex               uint32
	Recovery, CategoryRecovery              uint32
	InterruptFlags, AuraInterruptFlags      uint32
	ProcCharges, MaxLevel, BaseLevel        uint32
	SpellLevel, DurationIndex, PowerType    uint32
	ManaCost, ManaCostPerLevel              uint32
	ManaPerSecond, RangeIndex               uint32
	Speed                                   float32
	CumulativeAura                          uint32
	Effect                                  [3]uint32
	DieSides                                [3]int32
	BaseDice                                [3]uint32
	RealPointsPerLevel                      [3]float32
	DicePerLevel                            [3]float32
	BasePoints                              [3]int32
	TargetA, TargetB                        [3]uint32
	Aura, AuraPeriod                        [3]uint32
	MiscValue                               [3]int32
	TriggerSpell                            [3]uint32
	VisualID, ManaCostPct                   uint32
	StartRecoveryCategory                   uint32
	StartRecoveryTime, SchoolMask           uint32
}

// Spell attribute bits used by the cast code (SharedDefines.h).
const (
	SpellAttrRanged              = 0x00000002
	SpellAttrAbility             = 0x00000010
	SpellAttrPassive             = 0x00000040
	SpellAttrDisabledWhileActive = 0x02000000
	SpellAttrCantCancel          = 0x80000000
)

// Aura interrupt flags (Unit.h).
const (
	AuraInterruptMove      = 0x00000008
	AuraInterruptTurning   = 0x00000010
	AuraInterruptChangeMap = 0x00080000
	AuraInterruptNotSeated = 0x00040000
)

// Spell effects and aura names this server evaluates.
const (
	EffectSchoolDamage  = 2
	EffectApplyAura     = 6
	EffectTeleportUnits = 5
	EffectScriptEffect  = 77
	EffectHeal          = 10
	EffectEnergize      = 30

	AuraPeriodicDamage       = 3
	AuraPeriodicHeal         = 8
	AuraModResistance        = 22
	AuraPeriodicEnergize     = 24
	AuraModStat              = 29
	AuraModIncreaseHealth    = 34
	AuraModIncreaseEnergy    = 35
	AuraModPowerRegenPercent = 110
	AuraModRegenDuringCombat = 116
)

// CastTime and Duration mirror SpellCastTimes.dbc / SpellDuration.dbc rows.
type CastTime struct{ Base, PerLevel, Minimum int32 }
type Duration struct{ Base, PerLevel, Max int32 }

// Spell returns the Spell.dbc entry or nil.
func (c *Catalog) Spell(id uint32) *SpellInfo { return c.spellInfo[id] }

// CastTimeMS ports GetSpellCastTime without spell mods: the table base value,
// 0 when the index is unknown.
func (c *Catalog) CastTimeMS(s *SpellInfo) uint32 {
	if t, ok := c.castTimes[s.CastingTimeIndex]; ok && t.Base > 0 {
		return uint32(t.Base)
	}
	return 0
}

// DurationMS ports GetSpellDuration: the base duration, -1 for an infinite
// aura (index 21 / unknown rows carry -1), without combo points or mods.
func (c *Catalog) DurationMS(s *SpellInfo) int32 {
	if d, ok := c.durations[s.DurationIndex]; ok {
		return d.Base
	}
	return 0
}

// SetSpellTables installs the spell tables (also used by tests).
func (c *Catalog) SetSpellTables(spells []SpellInfo, times map[uint32]CastTime, durations map[uint32]Duration) {
	c.spellInfo = make(map[uint32]*SpellInfo, len(spells))
	for i := range spells {
		c.spellInfo[spells[i].ID] = &spells[i]
	}
	c.castTimes, c.durations = times, durations
}

// Implicit spell targets used by the teleport effect (SharedDefines.h).
const (
	TargetInnkeeperCoordinates = 9
	TargetTableXYZCoordinates  = 17
)

// SpellsInCategory ports sSpellCategoryStore: every spell sharing a cooldown
// category, in ascending id order.
func (c *Catalog) SpellsInCategory(cat uint32) []uint32 {
	c.catOnce.Do(func() {
		ids := make([]uint32, 0, len(c.spellInfo))
		for id := range c.spellInfo {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		c.byCat = map[uint32][]uint32{}
		for _, id := range ids {
			if k := c.spellInfo[id].Category; k != 0 {
				c.byCat[k] = append(c.byCat[k], id)
			}
		}
	})
	return c.byCat[cat]
}
