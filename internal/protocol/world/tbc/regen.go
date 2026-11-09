package tbc

import (
	"math"
	"time"

	"wowserver/internal/application/character/stats"
	"wowserver/internal/domain/aura"
	"wowserver/internal/domain/character"
	"wowserver/internal/domain/regen"
	"wowserver/internal/protocol/world/tbc/codec"
	f "wowserver/internal/protocol/world/tbc/updatefields"
)

// manaUseDelay is the 5 s "recent cast" window of Unit::IsUnderLastManaUseEffect.
const manaUseDelay = 5 * time.Second

// regenState builds the regen snapshot of the session's player.
func (h *Handler) regenState(s *Session, now time.Time) regen.State {
	p := s.player
	st := regen.State{Health: p.Health, MaxHealth: p.Stats.MaxHealth, Sitting: s.sitting, RecentManaUse: now.Before(s.manaUseUntil)}
	st.Power = [5]uint32{p.Mana, p.Rage, p.Focus, p.Energy, p.Happiness}
	st.MaxPower = [5]uint32{p.Stats.MaxMana, 1000, 100, 100, 1000000}
	hpBase, hpSpirit, mpSpirit := h.Data.RegenRatios(uint32(p.Class), uint32(p.Level))
	spirit, intellect := p.Stats.Attributes[4], p.Stats.Attributes[3]
	st.OCTRegenHP = regen.OCTRegenHPPerSpirit(spirit, hpBase, hpSpirit)
	st.ManaRegen, st.ManaRegenInterrupt = regen.ManaRegenValues(intellect, spirit, mpSpirit)
	return st
}

// regenTick ports the Player::Update regen timer + RegenerateAll: every
// REGEN_TIME_FULL the values are recomputed and only changed fields are sent.
func (h *Handler) regenTick(s *Session, now time.Time) {
	p := s.player
	if p == nil || !p.Entered || p.Stats == nil || p.Health == 0 {
		return
	}
	if s.regenAt.IsZero() {
		s.regenAt = now.Add(regen.TickInterval * time.Millisecond)
		return
	}
	if now.Before(s.regenAt) {
		return
	}
	s.regenAt = now.Add(regen.TickInterval * time.Millisecond)
	st := h.regenState(s, now)
	before := st
	mods := regen.Mods{}
	if s.spells != nil {
		mods = s.spells.auras.RegenMods()
	}
	regen.Tick(&st, mods, regen.DefaultRates)
	var ch []codec.FieldChange
	if st.Health != before.Health {
		p.Health = st.Health
		ch = append(ch, codec.FieldChange{Index: f.UNIT_FIELD_HEALTH, Value: st.Health})
	}
	for i := range st.Power {
		if st.Power[i] == before.Power[i] {
			continue
		}
		ch = append(ch, codec.FieldChange{Index: f.UNIT_FIELD_POWER1 + i, Value: st.Power[i]})
	}
	p.Mana, p.Rage, p.Focus, p.Energy, p.Happiness = st.Power[0], st.Power[1], st.Power[2], st.Power[3], st.Power[4]
	if len(ch) > 0 {
		_ = s.send(codec.ValuesUpdate(uint64(p.GUID), ch))
	}
}

// refreshStats recomputes the equipment-derived stats after the item set
// changed (Player::_ApplyItemMods / UpdateAllStats) and sends the changed
// fields, including PLAYER_FIELD_MOD_MANA_REGEN(_INTERRUPT).
func (h *Handler) refreshStats(s *Session) {
	p := s.player
	if p == nil || p.Stats == nil {
		return
	}
	old := p.Stats
	ns, err := stats.Calculate(p, h.Data)
	if err != nil {
		s.log.Warn("stat recompute failed", "guid", p.GUID, "error", err)
		return
	}
	if s.spells != nil {
		applyAuraStats(ns, s.spells.auras.StatMods())
	}
	p.Stats = ns
	var ch []codec.FieldChange
	set := func(i int, v uint32) { ch = append(ch, codec.FieldChange{Index: i, Value: v}) }
	flt := func(i int, a, b float32) {
		if a != b {
			set(i, math.Float32bits(b))
		}
	}
	for i := 0; i < 5; i++ {
		if old.Attributes[i] != ns.Attributes[i] {
			set(f.UNIT_FIELD_STAT0+i, ns.Attributes[i])
		}
		if old.Positive[i] != ns.Positive[i] {
			set(f.PLAYER_FIELD_POSSTAT0+i, ns.Positive[i])
		}
		if old.Negative[i] != ns.Negative[i] {
			set(f.PLAYER_FIELD_NEGSTAT0+i, ns.Negative[i])
		}
	}
	for i := range ns.Resistances {
		if old.Resistances[i] != ns.Resistances[i] {
			set(f.UNIT_FIELD_RESISTANCES+i, ns.Resistances[i])
		}
	}
	if old.MaxHealth != ns.MaxHealth {
		set(f.UNIT_FIELD_MAXHEALTH, ns.MaxHealth)
	}
	if old.MaxMana != ns.MaxMana {
		set(f.UNIT_FIELD_MAXPOWER1, ns.MaxMana)
	}
	if p.Health > ns.MaxHealth {
		p.Health = ns.MaxHealth
		set(f.UNIT_FIELD_HEALTH, p.Health)
	}
	if p.Mana > ns.MaxMana {
		p.Mana = ns.MaxMana
		set(f.UNIT_FIELD_POWER1, p.Mana)
	}
	if old.AttackPower != ns.AttackPower {
		set(f.UNIT_FIELD_ATTACK_POWER, uint32(ns.AttackPower))
	}
	if old.RangedAttackPower != ns.RangedAttackPower {
		set(f.UNIT_FIELD_RANGED_ATTACK_POWER, uint32(ns.RangedAttackPower))
	}
	for i := 0; i < 3; i++ {
		if old.AttackTime[i] != ns.AttackTime[i] {
			set(f.UNIT_FIELD_BASEATTACKTIME+i, ns.AttackTime[i])
		}
	}
	for i, at := range []int{f.UNIT_FIELD_MINDAMAGE, f.UNIT_FIELD_MINOFFHANDDAMAGE, f.UNIT_FIELD_MINRANGEDDAMAGE} {
		flt(at, old.DamageMin[i], ns.DamageMin[i])
		flt(at+1, old.DamageMax[i], ns.DamageMax[i])
	}
	if old.ShieldBlock != ns.ShieldBlock {
		set(f.PLAYER_SHIELD_BLOCK, ns.ShieldBlock)
	}
	// Combat ratings and the derived character-sheet percentages.
	for cr, v := range ns.Ratings {
		if v != 0 {
			set(f.PLAYER_FIELD_COMBAT_RATING_1+cr, uint32(v))
		}
	}
	flt(f.PLAYER_BLOCK_PERCENTAGE, old.BlockPct, ns.BlockPct)
	flt(f.PLAYER_DODGE_PERCENTAGE, old.DodgePct, ns.DodgePct)
	flt(f.PLAYER_PARRY_PERCENTAGE, old.ParryPct, ns.ParryPct)
	flt(f.PLAYER_CRIT_PERCENTAGE, old.CritPct, ns.CritPct)
	flt(f.PLAYER_RANGED_CRIT_PERCENTAGE, old.RangedCritPct, ns.RangedCritPct)
	flt(f.PLAYER_OFFHAND_CRIT_PERCENTAGE, old.OffhandCritPct, ns.OffhandCritPct)
	for i := range ns.SpellCritPct {
		flt(f.PLAYER_SPELL_CRIT_PERCENTAGE1+i, old.SpellCritPct[i], ns.SpellCritPct[i])
	}
	if old.Expertise != ns.Expertise {
		set(f.PLAYER_EXPERTISE, uint32(ns.Expertise))
		set(f.PLAYER_OFFHAND_EXPERTISE, uint32(ns.Expertise))
	}
	st := h.regenState(s, time.Now())
	oldRegen, oldInt := manaRegenOf(h, old, p)
	if st.ManaRegen != oldRegen {
		set(f.PLAYER_FIELD_MOD_MANA_REGEN, math.Float32bits(st.ManaRegen))
	}
	if st.ManaRegenInterrupt != oldInt {
		set(f.PLAYER_FIELD_MOD_MANA_REGEN_INTERRUPT, math.Float32bits(st.ManaRegenInterrupt))
	}
	if len(ch) > 0 {
		_ = s.send(codec.ValuesUpdate(uint64(p.GUID), ch))
	}
}

func manaRegenOf(h *Handler, st *character.Stats, p *character.Player) (float32, float32) {
	_, _, mp := h.Data.RegenRatios(uint32(p.Class), uint32(p.Level))
	return regen.ManaRegenValues(st.Attributes[3], st.Attributes[4], mp)
}

// applyAuraStats adds the aura stat sums (HandleAuraModStat, ModResistance,
// ModIncreaseHealth) to freshly calculated stats. Derived values (max health
// from stamina, attack power from strength) are not recomputed from them.
func applyAuraStats(ns *character.Stats, m aura.StatMods) {
	for i := 0; i < 5; i++ {
		ns.Positive[i] += m.Pos[i]
		ns.Negative[i] += m.Neg[i]
		v := int64(ns.Attributes[i]) + int64(m.Pos[i]) - int64(m.Neg[i])
		ns.Attributes[i] = uint32(max(v, 0))
	}
	for i := range ns.Resistances {
		if i < len(m.Resist) {
			ns.Resistances[i] = uint32(max(int64(ns.Resistances[i])+int64(m.Resist[i]), 0))
		}
	}
	ns.MaxHealth = uint32(max(int64(ns.MaxHealth)+int64(m.MaxHealth), 1))
}
