// Package aura is the pure core of SpellAuraHolder/Aura: slot bookkeeping,
// durations, periodic ticks and the modifiers auras feed back into stats and
// regeneration. It performs no I/O.
package aura

import "wowserver/internal/domain/regen"

// Slot layout (SpellAuraDefines.h): positive auras take the first 40 slots,
// negative ones the remaining 16.
const (
	MaxAuras         = 56
	MaxPositiveAuras = 40
)

// Aura names evaluated here (SpellAuraDefines.h).
const (
	PeriodicDamage                   = 3
	ModDamageDone                    = 13
	ModDamageTaken                   = 14
	ModCritPercent                   = 52
	ModHitChance                     = 54
	ModSpellHitChance                = 55
	ModSpellCritChance               = 57
	ModSpellCritChanceSchool         = 71
	ModDamagePercentDone             = 79
	ModDamagePercentTaken            = 87
	ModAttackerMeleeHitChance        = 184
	ModAttackerRangedHitChance       = 185
	ModAttackerSpellHitChance        = 186
	ModAttackerMeleeCritChance       = 187
	ModAttackerRangedCritChance      = 188
	ModAttackerSpellWeaponCritChance = 197
	ModAttackerSpellCritChance       = 179
	PeriodicHeal                     = 8
	ModResistance                    = 22
	PeriodicEnergize                 = 24
	ModStat                          = 29
	ModIncreaseHealth                = 34
	ModIncreaseEnergy                = 35
	ModPowerRegenPercent             = 110
	ModRegenDuringCombat             = 116
)

// Effect is one Aura of a holder (one per spell effect index).
type Effect struct {
	Name   uint32 // SPELL_AURA_*
	Amount int32
	Misc   int32
	Period int32 // ms, 0 = not periodic
	Timer  int32 // ms until the next tick
}

// Holder ports SpellAuraHolder for the effects of one spell on one unit.
type Holder struct {
	SpellID        uint32
	Caster         uint64
	Level          uint8
	Slot           uint8
	Duration       int32 // remaining ms, -1 = infinite
	MaxDuration    int32
	Positive       bool
	Passive        bool
	InterruptFlags uint32
	CantCancel     bool
	Effects        [3]*Effect
}

// Kind of Event returned by Update.
type Kind uint8

const (
	Tick Kind = iota
	Expired
)

// Event is something Update asks the caller to apply.
type Event struct {
	Kind   Kind
	Holder *Holder
	Effect int
}

// Set is the aura list of one unit.
type Set struct{ slots [MaxAuras]*Holder }

// NewHolder builds a holder; periodic effects start their timer at one period.
func NewHolder(spellID uint32, caster uint64, level uint8, duration int32, positive bool, effects [3]*Effect) *Holder {
	h := &Holder{SpellID: spellID, Caster: caster, Level: level, Duration: duration, MaxDuration: duration, Positive: positive, Effects: effects}
	for _, e := range h.Effects {
		if e != nil && e.Period > 0 {
			e.Timer = e.Period
		}
	}
	return h
}

// Add inserts the holder. An existing holder of the same spell and caster is
// removed first and returned (AddSpellAuraHolder replaces it). ok is false
// when no slot is free.
func (s *Set) Add(h *Holder) (replaced *Holder, ok bool) {
	for i, o := range s.slots {
		if o != nil && o.SpellID == h.SpellID && o.Caster == h.Caster && !h.Passive {
			replaced = o
			s.slots[i] = nil
			break
		}
	}
	lo, hi := 0, MaxPositiveAuras
	if !h.Positive {
		lo, hi = MaxPositiveAuras, MaxAuras
	}
	for i := lo; i < hi; i++ {
		if s.slots[i] == nil {
			h.Slot = uint8(i)
			s.slots[i] = h
			return replaced, true
		}
	}
	return replaced, false
}

// Remove clears a slot and returns its holder.
func (s *Set) Remove(slot uint8) *Holder {
	if int(slot) >= MaxAuras {
		return nil
	}
	h := s.slots[slot]
	s.slots[slot] = nil
	return h
}

// BySpell returns the first holder of a spell.
func (s *Set) BySpell(id uint32) *Holder {
	for _, h := range s.slots {
		if h != nil && h.SpellID == id {
			return h
		}
	}
	return nil
}

// Holders lists the holders in slot order.
func (s *Set) Holders() []*Holder {
	var out []*Holder
	for _, h := range s.slots {
		if h != nil {
			out = append(out, h)
		}
	}
	return out
}

// RemoveWithInterrupt removes every holder whose spell has one of the aura
// interrupt flags (Unit::RemoveAurasWithInterruptFlags).
func (s *Set) RemoveWithInterrupt(flags uint32) []*Holder {
	var out []*Holder
	for i, h := range s.slots {
		if h != nil && h.InterruptFlags&flags != 0 {
			out = append(out, h)
			s.slots[i] = nil
		}
	}
	return out
}

// Update advances all holders by diff ms. Periodic ticks come first; holders
// whose duration ran out are removed and reported as Expired.
func (s *Set) Update(diff int32) []Event {
	var ev []Event
	for i, h := range s.slots {
		if h == nil {
			continue
		}
		if h.Duration >= 0 {
			h.Duration -= diff
		}
		for k, e := range h.Effects {
			if e == nil || e.Period <= 0 {
				continue
			}
			e.Timer -= diff
			for e.Timer <= 0 {
				ev = append(ev, Event{Tick, h, k})
				e.Timer += e.Period
			}
		}
		if h.MaxDuration >= 0 && h.Duration <= 0 {
			ev = append(ev, Event{Expired, h, 0})
			s.slots[i] = nil
		}
	}
	return ev
}

// StatMods is the sum of stat related aura effects.
type StatMods struct {
	Pos, Neg  [5]uint32
	Resist    [7]int32
	MaxHealth int32
	MaxEnergy int32
}

// StatMods ports the HandleAuraModStat / ModResistance / ModIncrease* sums.
func (s *Set) StatMods() StatMods {
	var m StatMods
	addStat := func(i int, v int32) {
		if v >= 0 {
			m.Pos[i] += uint32(v)
		} else {
			m.Neg[i] += uint32(-v)
		}
	}
	for _, h := range s.slots {
		if h == nil {
			continue
		}
		for _, e := range h.Effects {
			if e == nil {
				continue
			}
			switch e.Name {
			case ModStat:
				if e.Misc < 0 {
					for i := 0; i < 5; i++ {
						addStat(i, e.Amount)
					}
				} else if e.Misc < 5 {
					addStat(int(e.Misc), e.Amount)
				}
			case ModResistance:
				for sc := 0; sc < 7; sc++ {
					if e.Misc&(1<<sc) != 0 {
						m.Resist[sc] += e.Amount
					}
				}
			case ModIncreaseHealth:
				m.MaxHealth += e.Amount
			case ModIncreaseEnergy:
				if e.Misc == 3 {
					m.MaxEnergy += e.Amount
				}
			}
		}
	}
	return m
}

// RegenMods builds the aura inputs of regen.Tick.
func (s *Set) RegenMods() regen.Mods {
	var m regen.Mods
	for _, h := range s.slots {
		if h == nil {
			continue
		}
		for _, e := range h.Effects {
			if e == nil {
				continue
			}
			switch e.Name {
			case ModRegenDuringCombat:
				m.RegenDuringCombat += e.Amount
			case ModPowerRegenPercent:
				if e.Misc >= 0 && e.Misc < 5 {
					m.PowerRegenPct[e.Misc] = mulOne(m.PowerRegenPct[e.Misc]) * float32(100+e.Amount) / 100
				}
			}
		}
	}
	return m
}

func mulOne(v float32) float32 {
	if v == 0 {
		return 1
	}
	return v
}

// Total is Unit::GetTotalAuraModifier: the sum of the amounts of every effect
// with the aura name.
func (s *Set) Total(name uint32) int32 {
	var t int32
	s.each(func(_ *Holder, e *Effect) {
		if e.Name == name {
			t += e.Amount
		}
	})
	return t
}

// TotalByMisc is GetTotalAuraModifierByMiscMask: effects whose misc value
// shares a bit with mask.
func (s *Set) TotalByMisc(name uint32, mask uint32) int32 {
	var t int32
	s.each(func(_ *Holder, e *Effect) {
		if e.Name == name && uint32(e.Misc)&mask != 0 {
			t += e.Amount
		}
	})
	return t
}

// Multiplier is GetTotalAuraMultiplier: the product of (100+amount)/100.
func (s *Set) Multiplier(name uint32) float32 {
	m := float32(1)
	s.each(func(_ *Holder, e *Effect) {
		if e.Name == name {
			m *= (100 + float32(e.Amount)) / 100
		}
	})
	return m
}

// Each calls fn for every effect of every holder.
func (s *Set) Each(fn func(h *Holder, e *Effect)) { s.each(fn) }

func (s *Set) each(fn func(h *Holder, e *Effect)) {
	for _, h := range s.slots {
		if h == nil {
			continue
		}
		for _, e := range h.Effects {
			if e != nil {
				fn(h, e)
			}
		}
	}
}
