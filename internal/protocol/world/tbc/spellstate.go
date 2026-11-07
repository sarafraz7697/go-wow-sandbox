package tbc

import (
	"sort"
	"time"

	"wowserver/internal/domain/aura"
	"wowserver/internal/domain/character"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world/tbc/codec"
)

// restoreSpellState ports Player::_LoadAuras and SpellCooldownMgr::LoadFromDB:
// a fresh spell state is built from the saved rows.
func (h *Handler) restoreSpellState(s *Session, p *character.Player, now time.Time) {
	s.spells = nil
	sp := s.sp()
	for _, c := range p.Cooldowns {
		if h.Data.Spell(c.Spell) == nil || c.End <= now.Unix() {
			continue
		}
		sp.cooldowns[c.Spell] = time.Unix(c.End, 0)
	}
	for _, a := range p.Auras {
		info := h.Data.Spell(a.Spell)
		if info == nil {
			continue
		}
		var effects [3]*aura.Effect
		found := false
		for i := 0; i < 3; i++ {
			if a.EffectMask&(1<<i) == 0 || info.Aura[i] == 0 {
				continue
			}
			e := &aura.Effect{Name: info.Aura[i], Amount: a.Amount[i], Misc: info.MiscValue[i]}
			if isPeriodic(e.Name) {
				e.Period = int32(a.Period[i])
			}
			effects[i] = e
			found = true
		}
		if !found {
			continue
		}
		positive := effectsPositive(effects)
		remain := a.Remain
		// Only negative timed auras age while offline (reference quirk).
		if remain != -1 && !positive {
			if remain/1000 <= int32(p.OfflineSeconds) {
				continue
			}
			remain -= int32(p.OfflineSeconds) * 1000
		}
		hd := aura.NewHolder(info.ID, a.Caster, uint8(p.Level), remain, positive, effects)
		hd.MaxDuration = a.MaxDuration
		hd.InterruptFlags = info.AuraInterruptFlags
		hd.CantCancel = info.Attributes&gamedata.SpellAttrCantCancel != 0
		if _, ok := sp.auras.Add(hd); ok {
			sp.auraAt = now
		}
	}
}

// snapshotSpellState copies the live cooldowns and auras into the player so
// they are sent in SMSG_INITIAL_SPELLS and written by the next save
// (Player::_SaveAuras, SpellCooldownMgr::SaveToDB).
func (h *Handler) snapshotSpellState(s *Session, p *character.Player, now time.Time) {
	p.Cooldowns, p.Auras = nil, nil
	sp := s.spells
	if sp == nil {
		return
	}
	end := map[uint32]time.Time{}
	for id, t := range sp.cooldowns {
		if t.After(now) {
			end[id] = t
		}
	}
	// A category cooldown stamps every spell of the category.
	for cat, t := range sp.catCooldowns {
		if !t.After(now) {
			continue
		}
		for _, id := range h.Data.SpellsInCategory(cat) {
			if old, ok := end[id]; !ok || t.After(old) {
				end[id] = t
			}
		}
	}
	for id, t := range end {
		p.Cooldowns = append(p.Cooldowns, character.SavedCooldown{Spell: id, End: t.Unix()})
	}
	sort.Slice(p.Cooldowns, func(i, j int) bool { return p.Cooldowns[i].Spell < p.Cooldowns[j].Spell })
	for _, hd := range sp.auras.Holders() {
		if hd.Passive {
			continue
		}
		a := character.SavedAura{Caster: hd.Caster, Spell: hd.SpellID, Stack: 1, Remain: hd.Duration, MaxDuration: hd.MaxDuration}
		if info := h.Data.Spell(hd.SpellID); info != nil {
			a.Charges = info.ProcCharges
		}
		for i, e := range hd.Effects {
			if e == nil {
				continue
			}
			a.Amount[i] = e.Amount
			a.Period[i] = uint32(max(e.Period, 0))
			a.EffectMask |= 1 << i
		}
		if a.EffectMask != 0 {
			p.Auras = append(p.Auras, a)
		}
	}
}

// sendAuraState sends the aura fields and durations to a client that just
// created the player object (login and worldport).
func (h *Handler) sendAuraState(s *Session) {
	if s.spells == nil {
		return
	}
	s.spells.sentFields = map[int]uint32{}
	h.syncAuraFields(s)
	for _, hd := range s.spells.auras.Holders() {
		_ = s.send(codec.UpdateAuraDuration(hd.Slot, uint32(hd.Duration)),
			codec.SetExtraAuraInfo(uint64(s.player.GUID), hd.Slot, hd.SpellID, uint32(hd.MaxDuration), uint32(hd.Duration)))
	}
}
