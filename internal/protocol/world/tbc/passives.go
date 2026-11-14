package tbc

import (
	"wowserver/internal/domain/aura"
	"wowserver/internal/domain/spell"
	"wowserver/internal/gamedata"
)

// applyPassiveAuras ports the passive part of Player::addSpell/_LoadSpells:
// every known spell with SPELL_ATTR_PASSIVE is applied as an infinite passive
// holder (IsNeedCastPassiveLikeSpellAtLearn -> CastSpell(this, id, true)).
// Stance/form- and caster-aura-state-dependent passives apply like every
// other passive here (no forms exist yet).
func (h *Handler) applyPassiveAuras(s *Session) {
	sp := s.sp()
	for _, id := range s.player.Spells {
		info := h.Data.Spell(id)
		if info == nil || info.Attributes&gamedata.SpellAttrPassive == 0 {
			continue
		}
		if sp.auras.BySpell(id) != nil {
			continue
		}
		if hd := h.newPassiveHolder(s, info); hd != nil {
			sp.auras.Add(hd)
		}
	}
}

// newPassiveHolder builds the infinite positive holder of a passive spell.
func (h *Handler) newPassiveHolder(s *Session, info *gamedata.SpellInfo) *aura.Holder {
	var effects [3]*aura.Effect
	found := false
	for i := 0; i < 3; i++ {
		if info.Effect[i] != gamedata.EffectApplyAura || info.Aura[i] == 0 {
			continue
		}
		e := &aura.Effect{Name: info.Aura[i], Amount: spell.EffectValue(info, i, uint32(s.player.Level), irand), Misc: info.MiscValue[i]}
		if isPeriodic(e.Name) {
			e.Period = int32(info.AuraPeriod[i])
		}
		effects[i] = e
		found = true
	}
	if !found {
		return nil
	}
	hd := aura.NewHolder(info.ID, uint64(s.player.GUID), uint8(s.player.Level), -1, true, effects)
	hd.Passive = true
	hd.CantCancel = true
	return hd
}

// applyEquipAuras ports Player::CastItemEquipSpell (ITEM_SPELLTRIGGER_ON_EQUIP
// spells) plus the SPELL enchantments of SpellItemEnchantment (effect 3): the
// spells of the equipped items are passive holders; holders of equipment that
// was removed are dropped. Broken items (zero durability with a durability
// maximum) contribute nothing, like the reference item stat path.
func (h *Handler) applyEquipAuras(s *Session) {
	if s.player == nil || h.Data == nil {
		return
	}
	want := map[uint32]bool{}
	for _, item := range s.player.Items {
		if item.Bag != 0 || item.Slot >= 23 {
			continue
		}
		if item.Fields[59] > 0 && item.Fields[58] == 0 {
			continue
		}
		details, ok := h.Data.ItemPrototype(item.Entry)
		if !ok {
			continue
		}
		for _, is := range details.Spells {
			if is.SpellID != 0 && is.SpellTrigger == 1 {
				want[is.SpellID] = true
			}
		}
		for i := 0; i < 11; i++ {
			id := item.Fields[22+i*3]
			if id == 0 {
				continue
			}
			enchant, exists := h.Data.Enchantment(id)
			if !exists || enchant.Condition != 0 {
				continue
			}
			for j, effect := range enchant.Effects {
				if effect == 3 && enchant.Arguments[j] != 0 { // ITEM_ENCHANTMENT_TYPE_SPELL
					want[enchant.Arguments[j]] = true
				}
			}
		}
	}
	sp := s.sp()
	changed := false
	for id := range sp.equipAuras {
		if want[id] {
			continue
		}
		if hd := sp.auras.BySpell(id); hd != nil && hd.Passive {
			sp.auras.Remove(hd.Slot)
			delete(sp.equipAuras, id)
			changed = true
		}
	}
	for id := range want {
		if sp.equipAuras[id] {
			continue
		}
		info := h.Data.Spell(id)
		if info == nil {
			continue
		}
		hd := h.newPassiveHolder(s, info)
		if hd == nil {
			continue
		}
		if _, ok := sp.auras.Add(hd); !ok {
			continue
		}
		if sp.equipAuras == nil {
			sp.equipAuras = map[uint32]bool{}
		}
		sp.equipAuras[id] = true
		changed = true
	}
	if changed {
		h.syncAuraFields(s)
	}
}
