// Package stats calculates the base/equipment login snapshot. Spell aura
// evaluation remains a separate dependency, not guessed from spell names.
package stats

import (
	"fmt"
	"wowserver/internal/domain/character"
	"wowserver/internal/gamedata"
)

// AddItemStatRating maps one ItemStatType of the ITEM_MOD_*_RATING family
// to the CR_* combat ratings exactly like Player::_ApplyItemBonuses: the
// combined stats (hit, crit, hit taken, resilience, haste) feed several
// ratings at once. It reports whether the kind is a known rating.
func AddItemStatRating(kind uint32, amount int32, ratings *[24]int32) bool {
	add := func(cr uint32) { ratings[cr] += amount }
	switch kind {
	case 12: // defense skill
		add(gamedata.CRDefenseSkill)
	case 13: // dodge
		add(gamedata.CRDodge)
	case 14: // parry
		add(gamedata.CRParry)
	case 15: // block
		add(gamedata.CRBlock)
	case 16: // hit melee
		add(gamedata.CRHitMelee)
	case 17: // hit ranged
		add(gamedata.CRHitRanged)
	case 18: // hit spell
		add(gamedata.CRHitSpell)
	case 19: // crit melee
		add(gamedata.CRCritMelee)
	case 20: // crit ranged
		add(gamedata.CRCritRanged)
	case 21: // crit spell
		add(gamedata.CRCritSpell)
	case 22: // hit taken melee
		add(gamedata.CRHitTakenMelee)
	case 23: // hit taken ranged
		add(gamedata.CRHitTakenRanged)
	case 24: // hit taken spell
		add(gamedata.CRHitTakenSpell)
	case 25: // crit taken melee
		add(gamedata.CRCritTakenMelee)
	case 26: // crit taken ranged
		add(gamedata.CRCritTakenRanged)
	case 27: // crit taken spell
		add(gamedata.CRCritTakenSpell)
	case 28: // haste melee
		add(gamedata.CRHasteMelee)
	case 29: // haste ranged
		add(gamedata.CRHasteRanged)
	case 30: // haste spell
		add(gamedata.CRHasteSpell)
	case 31: // hit (melee + ranged)
		add(gamedata.CRHitMelee)
		add(gamedata.CRHitRanged)
	case 32: // crit (melee + ranged)
		add(gamedata.CRCritMelee)
		add(gamedata.CRCritRanged)
	case 33: // hit taken (all)
		add(gamedata.CRHitTakenMelee)
		add(gamedata.CRHitTakenRanged)
		add(gamedata.CRHitTakenSpell)
	case 34, 35: // crit taken / resilience (all)
		add(gamedata.CRCritTakenMelee)
		add(gamedata.CRCritTakenRanged)
		add(gamedata.CRCritTakenSpell)
	case 36: // haste (melee + ranged)
		add(gamedata.CRHasteMelee)
		add(gamedata.CRHasteRanged)
	case 37: // expertise
		add(gamedata.CRExpertise)
	default:
		return false
	}
	return true
}

func Calculate(p *character.Player, data *gamedata.Catalog) (*character.Stats, error) {
	level, ok := data.Level(uint32(p.Race), uint32(p.Class), uint32(p.Level))
	if !ok {
		return nil, fmt.Errorf("%w: no level stats for race=%d class=%d level=%d", character.ErrPlayerUnsupported, p.Race, p.Class, p.Level)
	}
	base, ok := data.ClassLevel(uint32(p.Class), uint32(p.Level))
	if !ok {
		return nil, fmt.Errorf("%w: no class level stats for class=%d level=%d", character.ErrPlayerUnsupported, p.Class, p.Level)
	}
	s := &character.Stats{AttackTime: [3]uint32{2000, 2000, 2000}}
	var values [5]int64
	var ratings [24]int32
	for i, v := range level.Stats {
		values[i] = int64(v)
	}
	var health, mana int64
	var weapon [3]bool
	var weaponMin, weaponMax [3]float32
	var damageBonus [3]float32
	weaponMin[0], weaponMax[0] = 1, 2
	limited := map[string]bool{}
	limit := func(reason string) {
		if !limited[reason] {
			limited[reason] = true
			s.Limitations = append(s.Limitations, reason)
		}
	}
	addStat := func(kind uint32, amount int32) {
		index := -1
		switch kind {
		case 0:
			mana += int64(amount)
		case 1:
			health += int64(amount)
		case 3:
			index = 1
		case 4:
			index = 0
		case 5:
			index = 3
		case 6:
			index = 4
		case 7:
			index = 2
		default:
			if amount != 0 && !AddItemStatRating(kind, amount, &ratings) {
				limit("unknown item stat modifiers")
			}
		}
		if index >= 0 {
			values[index] += int64(amount)
			if amount >= 0 {
				s.Positive[index] += uint32(amount)
			} else {
				s.Negative[index] += uint32(-int64(amount))
			}
		}
	}
	for _, item := range p.Items {
		// Bags can contribute item bonuses; backpack/bank contents cannot.
		if item.Bag != 0 || item.Slot >= 23 {
			continue
		}
		if item.Fields[59] > 0 && item.Fields[58] == 0 {
			continue
		}
		details, ok := data.ItemPrototype(item.Entry)
		if !ok {
			return nil, fmt.Errorf("missing equipped item template %d", item.Entry)
		}
		for _, stat := range details.ItemStat {
			addStat(stat.ItemType, stat.Value)
		}
		for i, value := range details.ResistanceValues() {
			s.Resistances[i] += value
		}

		if details.ArmorDamageModifier > 0 {
			s.Resistances[0] += uint32(details.ArmorDamageModifier)
		}
		s.ShieldBlock += details.Block
		if item.Slot >= 15 && item.Slot <= 17 && details.Class == 2 {
			i := int(item.Slot) - 15
			weapon[i] = true
			weaponMin[i], weaponMax[i] = details.Damage[0].DamageMin, details.Damage[0].DamageMax
			if details.Delay > 0 {
				s.AttackTime[i] = details.Delay
			}
		}
		for _, spell := range details.Spells {
			if spell.SpellID != 0 && spell.SpellTrigger == 1 {
				limit("item equip spell auras")
			}
		}
		if details.ItemSet != 0 {
			limit("item set bonuses")
		}
		for i := 0; i < 11; i++ {
			id := item.Fields[22+i*3]
			if id == 0 {
				continue
			}
			enchant, exists := data.Enchantment(id)
			if !exists {
				limit("unknown enchantment ids are ignored")
				continue
			}
			if enchant.Condition != 0 {
				limit("conditional meta-gem effects")
				continue
			}
			for j, effect := range enchant.Effects {
				amount, arg := enchant.Amounts[j], enchant.Arguments[j]
				if amount == 0 && item.Fields[56] != 0 {
					limit("random-suffix scaled enchantments")
					continue
				}
				switch effect {
				case 0, 1: // empty or on-hit proc (does not change idle stats)
				case 2:
					if item.Slot >= 15 && item.Slot <= 17 {
						damageBonus[item.Slot-15] += float32(amount)
					}
				case 4:
					if arg < 7 {
						s.Resistances[arg] += amount
					}
				case 5:
					addStat(arg, int32(amount))
				default:
					limit("spell-based enchantment effects")
				}
			}
		}
	}
	for i, v := range values {
		s.Attributes[i] = uint32(max(v, 0))
	}
	// ---- combat ratings and derived sheet values (StatSystem.cpp) ----
	playerLevel := uint32(p.Level)
	maxSkill := playerLevel * 5
	s.Ratings = ratings
	rb := func(cr uint32) float32 { return data.RatingBonus(cr, playerLevel, s.Ratings[cr]) }
	// Defense skill (SKILL_DEFENSE 95). A character without the skill row is
	// treated as fully skilled: this server only grants skill lines that come
	// from starting spells, and the reference display has no defense penalty.
	defense := int32(maxSkill)
	for _, sk := range p.Skills {
		if sk.ID == 95 && sk.Value > 0 {
			defense = int32(sk.Value)
			break
		}
	}
	defDiff := float32(defense-int32(maxSkill)) * 0.04
	// Weapon skill per attack slot (GetWeaponSkillValue of BASE/OFF/RANGED).
	skillOf := func(slot uint8) int32 {
		skill := uint32(162) // SKILL_UNARMED
		for _, it := range p.Items {
			if it.Bag != 0 || it.Slot != slot {
				continue
			}
			if d, ok := data.ItemPrototype(it.Entry); ok && d.Class == 2 && int(d.SubClass) < len(gamedata.ItemWeaponSkills) && gamedata.ItemWeaponSkills[d.SubClass] != 0 {
				skill = gamedata.ItemWeaponSkills[d.SubClass]
			}
		}
		for _, sk := range p.Skills {
			if sk.ID == skill {
				return int32(sk.Value)
			}
		}
		return 0
	}
	// UpdateAllCritPercentages + UpdateCritPercentage per attack type:
	// agility base, the crit rating bonus and the weapon-skill difference.
	critBase := data.MeleeCritFromAgility(uint32(p.Class), playerLevel, s.Attributes[1])
	critOf := func(attackSlot uint8, cr uint32) float32 {
		v := critBase + rb(cr) + float32(skillOf(attackSlot)-int32(maxSkill))*0.04
		return max(v, 0)
	}
	s.CritPct = critOf(15, gamedata.CRCritMelee)
	s.OffhandCritPct = critOf(16, gamedata.CRCritMelee)
	s.RangedCritPct = critOf(17, gamedata.CRCritRanged)
	// UpdateDodgePercentage: agility part, defense difference, dodge rating.
	s.DodgePct = max(data.DodgeFromAgility(uint32(p.Class), playerLevel, s.Attributes[1])+defDiff+rb(gamedata.CRDodge), 0)
	// UpdateParryPercentage / UpdateBlockPercentage: the 5% base applies
	// only while the ability is known (learn spells with the
	// parry/block spell effect).
	if data.PlayerCanParry(p.Spells) {
		s.ParryPct = max(5+defDiff+rb(gamedata.CRParry), 0)
	}
	if data.PlayerCanBlock(p.Spells) {
		s.BlockPct = max(5+defDiff+rb(gamedata.CRBlock), 0)
	}
	// UpdateSpellCritChance per school (the normal school stays 0).
	spellCrit := data.SpellCritFromIntellect(uint32(p.Class), playerLevel, s.Attributes[3]) + rb(gamedata.CRCritSpell)
	for school := 1; school < 7; school++ {
		s.SpellCritPct[school] = max(spellCrit, 0)
	}
	// UpdateExpertise: the CR_EXPERTISE rating bonus for both hands.
	s.Expertise = int32(rb(gamedata.CRExpertise))
	stamina, intellect := int64(s.Attributes[2]), int64(s.Attributes[3])
	s.MaxHealth = uint32(max(1, int64(base.Health)+health+min(stamina, 20)+max(stamina-20, 0)*10))
	if base.Mana > 0 {
		s.MaxMana = uint32(max(0, int64(base.Mana)+mana+min(intellect, 20)+max(intellect-20, 0)*15))
	}
	strength, agility, lvl := int32(s.Attributes[0]), int32(s.Attributes[1]), int32(p.Level)
	s.Resistances[0] += s.Attributes[1] * 2
	s.ShieldBlock = uint32(max(0, int32(s.ShieldBlock)+strength/20-1))
	s.RangedAttackPower = agility - 10
	switch p.Class {
	case 1, 2:
		s.AttackPower = lvl*3 + strength*2 - 20
	case 3, 4:
		s.AttackPower = lvl*2 + strength + agility - 20
	case 7:
		s.AttackPower = lvl*2 + strength*2 - 20
	case 11:
		s.AttackPower = strength*2 - 20
	default:
		s.AttackPower = strength - 10
	}
	switch p.Class {
	case 3:
		s.RangedAttackPower += lvl * 2
	case 1, 4:
		s.RangedAttackPower += lvl
	}
	for i := 0; i < 3; i++ {
		if i > 0 && !weapon[i] {
			continue
		}
		ap := s.AttackPower
		if i == 2 {
			ap = s.RangedAttackPower
		}
		bonus := float32(max(ap, 0)) / 14 * float32(s.AttackTime[i]) / 1000
		s.DamageMin[i], s.DamageMax[i] = weaponMin[i]+bonus+damageBonus[i], weaponMax[i]+bonus+damageBonus[i]
		if i == 1 {
			s.DamageMin[i] *= 0.5
			s.DamageMax[i] *= 0.5
		}
	}
	// Passive racial/class spells and persisted auras are not yet evaluated.
	limit("passive spells, talents, buffs and forms are not evaluated")
	return s, nil
}
