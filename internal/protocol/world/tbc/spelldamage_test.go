package tbc

import (
	"testing"

	"wowserver/internal/domain/aura"
	"wowserver/internal/domain/character"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world/tbc/codec"
)

func TestEffectTargetsEnemy(t *testing.T) {
	info := &gamedata.SpellInfo{Effect: [3]uint32{gamedata.EffectSchoolDamage, gamedata.EffectApplyAura, 0}, TargetA: [3]uint32{6, 1, 6}}
	if !effectTargetsEnemy(info, 0) || effectTargetsEnemy(info, 1) || effectTargetsEnemy(info, 2) || !spellHasEnemyEffect(info) {
		t.Fatal("hostile target selection")
	}
}

func TestWeaponDamageEffect(t *testing.T) {
	c := &gamedata.Catalog{}
	c.SetSpellTables([]gamedata.SpellInfo{{ID: 1, EquippedItemClass: -1}}, nil, nil)
	h := &Handler{Data: c}
	p := &character.Player{Summary: character.Summary{GUID: 1, Level: 10}, Stats: &character.Stats{}}
	p.Stats.DamageMin[0], p.Stats.DamageMax[0] = 10, 10
	s := &Session{player: p}
	info := &gamedata.SpellInfo{ID: 78, Effect: [3]uint32{gamedata.EffectWeaponDamageNoSchool}, BasePoints: [3]int32{10}, DieSides: [3]int32{1}, BaseDice: [3]uint32{1}}
	// weapon 10 + (base 10 + 1 die) = 21
	if got := h.effectWeaponDamage(s, info, 1); got != 21 {
		t.Fatalf("heroic strike style damage %d", got)
	}
	// +20% physical damage buff
	s.sp().auras.Add(aura.NewHolder(1, 1, 10, -1, true, [3]*aura.Effect{{Name: aura.ModDamagePercentDone, Amount: 20, Misc: 1}}))
	if got := h.effectWeaponDamage(s, info, 1); got != 25 {
		t.Fatalf("with buff %d", got)
	}
}

func TestCalcSpellDamageArmorAndTaken(t *testing.T) {
	h := &Handler{Data: &gamedata.Catalog{}}
	p := &character.Player{Summary: character.Summary{GUID: 1, Level: 60}, Stats: &character.Stats{}}
	s := &Session{player: p}
	v := creatureView{}
	v.spawn.Level = 60
	info := &gamedata.SpellInfo{ID: 1, SchoolMask: 4, DefenseType: gamedata.DamageClassMagic, AttributesExB: spellAttrExBCantCrit}
	foe := &aura.Set{}
	foe.Add(aura.NewHolder(2, 1, 60, -1, false, [3]*aura.Effect{{Name: aura.ModDamagePercentTaken, Amount: 10, Misc: 4}}))
	d := h.calcSpellDamage(s, info, v, foe, 100, false)
	if d.damage != 110 {
		t.Fatalf("damage taken +10%%: %d", d.damage)
	}
	phys := &gamedata.SpellInfo{ID: 3, SchoolMask: 1, DefenseType: gamedata.DamageClassMelee, AttributesExB: spellAttrExBCantCrit}
	v.info.Armor = 3000
	if d := h.calcSpellDamage(s, phys, v, &aura.Set{}, 100, true); d.damage >= 100 {
		t.Fatalf("armor must reduce physical damage: %d", d.damage)
	}
}

func TestSpellGoMissAndLogLayout(t *testing.T) {
	p := codec.SpellGoMiss(1, 1, 5, 0, []uint64{2}, []codec.SpellMissEntry{{Target: 3, Reason: 1}}, codec.SpellTargets{})
	q := codec.SpellGo(1, 1, 5, 0, []uint64{2}, codec.SpellTargets{})
	if len(p.Payload) != len(q.Payload)+9 {
		t.Fatalf("miss entry is 9 bytes: %d vs %d", len(p.Payload), len(q.Payload))
	}
	if l := codec.SpellNonMeleeDamageLog(0x0102, 0x0304, 9, 50, 1, 0, 0, false, 0, 0); len(l.Payload) != 3+3+4+4+1+4+4+1+1+4+4+1 {
		t.Fatalf("damage log %d", len(l.Payload))
	}
}
