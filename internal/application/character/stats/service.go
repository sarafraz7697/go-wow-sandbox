// Package stats calculates the base/equipment login snapshot. Spell aura
// evaluation remains a separate dependency, not guessed from spell names.
package stats

import (
	"fmt"
	"wowserver/internal/domain/character"
	"wowserver/internal/gamedata"
)

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
			if amount != 0 {
				limit("combat ratings or other item stat modifiers")
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
