// Package regen ports Player::RegenerateAll / Regenerate / RegenerateHealth
// (PlayerRegen.cpp) as pure functions. Aura-derived modifiers arrive through
// Mods; until auras exist they are all neutral.
package regen

import "math"

// TickInterval is REGEN_TIME_FULL (Unit.h): regeneration runs every 2 s.
const TickInterval = 2000 // milliseconds

// Power indexes follow Powers: 0 mana, 1 rage, 2 focus, 3 energy, 4 happiness.
const (
	Mana = iota
	Rage
	Focus
	Energy
	Happiness
)

// State is the regenerating part of a player.
type State struct {
	Health, MaxHealth uint32
	Power, MaxPower   [5]uint32
	InCombat          bool
	Sitting           bool // !IsStandState()
	RecentManaUse     bool // IsUnderLastManaUseEffect()
	// OCTRegenHP is Player::OCTRegenHPPerSpirit(); ManaRegen and
	// ManaRegenInterrupt are PLAYER_FIELD_MOD_MANA_REGEN(_INTERRUPT).
	OCTRegenHP, ManaRegen, ManaRegenInterrupt float32
}

// Mods are the aura inputs of the regeneration formulas. The zero value is
// "no auras": multipliers of 0 are treated as 1.
type Mods struct {
	Polymorphed         bool
	RegenDuringCombat   int32      // total SPELL_AURA_MOD_REGEN_DURING_COMBAT (0 = none)
	HealthRegenInCombat int32      // total SPELL_AURA_MOD_HEALTH_REGEN_IN_COMBAT
	HealthRegenPct      float32    // product of (100+amount)/100 of MOD_HEALTH_REGEN_PERCENT
	InterruptRegen      bool       // SPELL_AURA_INTERRUPT_REGEN
	PowerRegenPct       [5]float32 // product per power of MOD_POWER_REGEN_PERCENT
}

// Rates are the Rate.* world config values (all 1 by default).
type Rates struct{ Health, Mana, RageLoss, Energy float32 }

// DefaultRates mirrors mangosd.conf defaults.
var DefaultRates = Rates{1, 1, 1, 1}

func mul(v float32) float32 {
	if v == 0 {
		return 1
	}
	return v
}

// Tick runs RegenerateAll once (the caller owns the 2 s timer) and mutates s.
func Tick(s *State, m Mods, r Rates) {
	if !s.InCombat || m.RegenDuringCombat != 0 || m.HealthRegenInCombat != 0 || m.Polymorphed {
		health(s, m, r)
		if !s.InCombat && !m.InterruptRegen {
			power(s, m, r, Rage)
		}
	}
	power(s, m, r, Energy)
	power(s, m, r, Mana)
}

func power(s *State, m Mods, r Rates, p int) {
	cur, max := s.Power[p], s.MaxPower[p]
	var add float32
	switch p {
	case Mana:
		if s.RecentManaUse {
			add = s.ManaRegenInterrupt * r.Mana * 2
		} else {
			add = s.ManaRegen * r.Mana * 2
		}
	case Rage:
		add = 20 * r.RageLoss
	case Energy:
		add = 20 * r.Energy
	}
	if p != Mana {
		add *= mul(m.PowerRegenPct[p])
	}
	if p != Rage {
		cur += uint32(add)
		if cur > max {
			cur = max
		}
	} else if cur <= uint32(add) {
		cur = 0
	} else {
		cur -= uint32(add)
	}
	s.Power[p] = cur
}

func health(s *State, m Mods, r Rates) {
	if s.Health >= s.MaxHealth {
		return
	}
	var add float32
	switch {
	case m.Polymorphed:
		add = float32(s.MaxHealth) / 3
	case !s.InCombat || m.RegenDuringCombat != 0:
		add = s.OCTRegenHP * r.Health
		if !s.InCombat {
			add *= mul(m.HealthRegenPct)
		} else {
			add *= float32(m.RegenDuringCombat) / 100
		}
		if s.Sitting {
			add *= 1.5
		}
	}
	add += float32(m.HealthRegenInCombat)
	if add < 0 {
		add = 0
	}
	// ModifyHealth(int32(add)): clamp to max.
	n := uint32(int32(add))
	if s.Health+n > s.MaxHealth {
		n = s.MaxHealth - s.Health
	}
	s.Health += n
}

// ManaRegenValues ports Player::UpdateManaRegen without auras: sqrt(INT) *
// OCTRegenMPPerSpirit (= spirit * gtRegenMPPerSpt). Both results are per second.
func ManaRegenValues(intellect, spirit uint32, mpPerSpirit float32) (regen, interrupt float32) {
	r := float32(math.Sqrt(float64(intellect))) * (float32(spirit) * mpPerSpirit)
	return r, r * 0
}

// OCTRegenHPPerSpirit ports Player::OCTRegenHPPerSpirit.
func OCTRegenHPPerSpirit(spirit uint32, base, more float32) float32 {
	s := float32(spirit)
	b := s
	if b > 50 {
		b = 50
	}
	return b*base + (s-b)*more
}
