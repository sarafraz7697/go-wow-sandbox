package tbc

import (
	"time"

	"wowserver/internal/domain/aura"
	"wowserver/internal/domain/combat"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world/tbc/codec"
	f "wowserver/internal/protocol/world/tbc/updatefields"
)

// Aura flag values of UNIT_FIELD_AURAFLAGS (SpellAuraDefines.h).
const (
	auraFlagNegative = 0x09
	auraFlagPositive = 0x1F
)

// addAura ports Unit::AddSpellAuraHolder + SpellAuraHolder::_AddSpellAuraHolder
// for an aura cast by the player on itself.
func (h *Handler) addAura(s *Session, now time.Time, st *castState, effects [3]*aura.Effect) {
	sp := s.sp()
	info := st.info
	positive := effectsPositive(effects)
	// Applying a form removes the previous one (stance switching).
	for _, e := range effects {
		if e != nil && e.Name == aura.ModShapeshift {
			for _, hd := range sp.auras.Holders() {
				if hd.SpellID != info.ID {
					for _, oe := range hd.Effects {
						if oe != nil && oe.Name == aura.ModShapeshift {
							sp.auras.Remove(hd.Slot)
							break
						}
					}
				}
			}
		}
	}
	duration := h.Data.DurationMS(info)
	hd := aura.NewHolder(info.ID, uint64(s.player.GUID), uint8(s.player.Level), duration, positive, effects)
	hd.InterruptFlags = info.AuraInterruptFlags
	hd.CantCancel = info.Attributes&gamedata.SpellAttrCantCancel != 0
	sp.auraAt = nowIfZero(sp.auraAt, now)
	if _, ok := sp.auras.Add(hd); !ok {
		return // no free slot
	}
	h.syncAuraFields(s)
	_ = s.send(codec.UpdateAuraDuration(hd.Slot, uint32(hd.Duration)),
		codec.SetExtraAuraInfo(uint64(s.player.GUID), hd.Slot, hd.SpellID, uint32(hd.MaxDuration), uint32(hd.Duration)))
	// Sitdown on apply of an aura that requires being seated.
	if info.AuraInterruptFlags&gamedata.AuraInterruptNotSeated != 0 && !s.sitting {
		h.setStandState(s, codec.StandStateSit)
	}
	h.afterAuraChange(s)
}

func nowIfZero(t, now time.Time) time.Time {
	if t.IsZero() {
		return now
	}
	return t
}

// auraWords builds the UNIT_FIELD_AURA* words for an aura set.
func (h *Handler) auraWords(set *aura.Set) map[int]uint32 {
	want := map[int]uint32{}
	for i := 0; i < aura.MaxAuras; i++ {
		want[f.UNIT_FIELD_AURA+i] = 0
	}
	for i := 0; i < 14; i++ {
		want[f.UNIT_FIELD_AURAFLAGS+i] = 0
		want[f.UNIT_FIELD_AURALEVELS+i] = 0
		want[f.UNIT_FIELD_AURAAPPLICATIONS+i] = 0
	}
	for _, hd := range set.Holders() {
		slot := int(hd.Slot)
		want[f.UNIT_FIELD_AURA+slot] = hd.SpellID
		shift := uint(slot%4) * 8
		flag := uint32(auraFlagNegative)
		if hd.Positive {
			flag = auraFlagPositive
		}
		want[f.UNIT_FIELD_AURAFLAGS+slot/4] |= flag << shift
		want[f.UNIT_FIELD_AURALEVELS+slot/4] |= uint32(hd.Level) << shift
		// application field expects count-1 (stack 1; proc charges multiply)
		if info := h.Data.Spell(hd.SpellID); info != nil && info.ProcCharges > 1 {
			want[f.UNIT_FIELD_AURAAPPLICATIONS+slot/4] |= (info.ProcCharges - 1) << shift
		}
	}
	return want
}

// syncAuraFields sends the changed UNIT_FIELD_AURA* words.
func (h *Handler) syncAuraFields(s *Session) {
	sp := s.sp()
	want := h.auraWords(&sp.auras)
	var ch []codec.FieldChange
	for idx, v := range want {
		if sp.sentFields[idx] != v {
			sp.sentFields[idx] = v
			ch = append(ch, codec.FieldChange{Index: idx, Value: v})
		}
	}
	if len(ch) > 0 {
		h.sendFields(s, ch...)
	}
}

// afterAuraRemoved refreshes fields and stats after holders left the set.
func (h *Handler) afterAuraRemoved(s *Session) {
	h.syncAuraFields(s)
	h.afterAuraChange(s)
}

func (h *Handler) afterAuraChange(s *Session) {
	h.refreshStats(s)
}

// auraTick ports SpellAuraHolder::Update/Aura::PeriodicTick for the player.
func (h *Handler) auraTick(s *Session, now time.Time) {
	sp := s.spells
	if sp.auraAt.IsZero() || len(sp.auras.Holders()) == 0 {
		sp.auraAt = now
		return
	}
	diff := int32(now.Sub(sp.auraAt).Milliseconds())
	if diff <= 0 {
		return
	}
	sp.auraAt = now
	events := sp.auras.Update(diff)
	p, caster := s.player, uint64(s.player.GUID)
	expired := false
	for _, e := range events {
		if e.Kind == aura.Expired {
			expired = true
			continue
		}
		eff := e.Holder.Effects[e.Effect]
		amount := uint32(max(eff.Amount, 0))
		switch eff.Name {
		case aura.PeriodicDamage:
			if p.Health == 0 || p.PlayerFlags&playerFlagsGhost != 0 {
				continue
			}
			info := h.Data.Spell(e.Holder.SpellID)
			if info == nil {
				continue
			}
			h.unitInCombat(s, now)
			damage := eff.Amount
			if damage < 0 {
				damage = 0
			}
			d := spellDamage{damage: uint32(damage)}
			if d.damage > 0 && info.SchoolMask&schoolMaskNormal != 0 {
				d.damage = combat.ArmorReduced(uint32(e.Holder.Level), p.Stats.Resistances[0], d.damage)
			}
			if info.SchoolMask&schoolMaskNormal == 0 && d.damage > 0 {
				res := int32(0)
				for school := 1; school < 7; school++ {
					if info.SchoolMask&(1<<school) != 0 {
						res = int32(p.Stats.Resistances[school])
						break
					}
				}
				d.resist = combat.ResistedDamage(rng{}, float32(res), uint32(p.Level), d.damage, true)
				d.damage -= d.resist
			}
			h.sendToSet(s, codec.PeriodicDamageLog(caster, e.Holder.Caster, e.Holder.SpellID, d.damage, info.SchoolMask, d.absorb, d.resist))
			if d.damage > 0 {
				p.Health = sub(p.Health, d.damage)
				h.sendFields(s, codec.FieldChange{Index: f.UNIT_FIELD_HEALTH, Value: p.Health})
				h.checkPlayerDeath(s, e.Holder.Caster, now)
			}
		case aura.PeriodicHeal:
			if p.Health == 0 {
				continue
			}
			h.sendToSet(s, codec.PeriodicHealLog(caster, e.Holder.Caster, e.Holder.SpellID, amount))
			if np := min(p.Health+amount, p.Stats.MaxHealth); np != p.Health {
				p.Health = np
				h.sendFields(s, codec.FieldChange{Index: f.UNIT_FIELD_HEALTH, Value: np})
			}
		case aura.PeriodicEnergize:
			if p.Health == 0 || eff.Misc < 0 || eff.Misc >= 5 {
				continue
			}
			rs := h.regenState(s, now)
			if rs.MaxPower[eff.Misc] == 0 {
				continue
			}
			h.sendToSet(s, codec.PeriodicEnergizeLog(caster, e.Holder.Caster, e.Holder.SpellID, uint32(eff.Misc), amount))
			h.setPower(s, int(eff.Misc), min(rs.Power[eff.Misc]+amount, rs.MaxPower[eff.Misc]))
		}
	}
	if expired {
		h.afterAuraRemoved(s)
	}
}

// interruptAurasOnMove ports the AURA_INTERRUPT_FLAG_MOVE removal.
func (h *Handler) interruptAurasOnMove(s *Session) {
	h.removeAurasWithInterrupt(s, gamedata.AuraInterruptMove)
}

// effectsPositive is the positive/negative split used for aura slots.
func effectsPositive(effects [3]*aura.Effect) bool {
	for _, e := range effects {
		if e == nil {
			continue
		}
		if (e.Name == aura.ModStat || e.Name == aura.ModResistance) && e.Amount < 0 || e.Name == aura.PeriodicDamage {
			return false
		}
	}
	return true
}
