package combat

import "testing"

type fixedRand struct{ u uint32 }

func (r fixedRand) Urand(lo, hi uint32) uint32 { return min(max(r.u, lo), hi) }
func (r fixedRand) Norm() float32              { return float32(r.u) / 100 }

func TestMagicHitTable(t *testing.T) {
	// equal level: 96% hit -> 400 of 10000 miss
	if MagicHit(fixedRand{399}, 60, 60, false, 0, 0, false) != SpellMissResist {
		t.Fatal("roll 399 must resist at equal level")
	}
	if MagicHit(fixedRand{400}, 60, 60, false, 0, 0, false) != SpellMissNone {
		t.Fatal("roll 400 must hit at equal level")
	}
	// +3 levels on a creature: 94-(3-2)*11 = 83% -> 1700 miss
	if MagicHit(fixedRand{1699}, 60, 63, false, 0, 0, false) != SpellMissResist || MagicHit(fixedRand{1700}, 60, 63, false, 0, 0, false) != SpellMissNone {
		t.Fatal("level difference table")
	}
	if MagicHit(fixedRand{0}, 60, 63, false, 0, 0, true) != SpellMissNone {
		t.Fatal("cant-miss spells always hit")
	}
}

func TestMeleeSpellHit(t *testing.T) {
	in := MeleeSpellInput{AttackerSkill: 300, VictimMaxSkill: 300, VictimDefense: 300, DodgePct: 5, ParryPct: 5}
	// 5% miss, then 5% dodge, then 5% parry
	if MeleeSpellHit(fixedRand{499}, in) != SpellMissMiss || MeleeSpellHit(fixedRand{999}, in) != SpellMissDodge ||
		MeleeSpellHit(fixedRand{1499}, in) != SpellMissParry || MeleeSpellHit(fixedRand{1500}, in) != SpellMissNone {
		t.Fatal("order miss/dodge/parry")
	}
	in.FromBehind = true
	if MeleeSpellHit(fixedRand{1499}, in) != SpellMissDodge && MeleeSpellHit(fixedRand{1499}, in) != SpellMissNone {
		t.Fatal("behind: no parry")
	}
	in.NoDodgeParry = true
	if MeleeSpellHit(fixedRand{600}, in) != SpellMissNone {
		t.Fatal("unavoidable ability")
	}
}

func TestResistAndCrit(t *testing.T) {
	if ResistedDamage(fixedRand{100}, 0, 60, 100, false) != 0 {
		t.Fatal("no resistance, no partial resist")
	}
	if r := ResistedDamage(fixedRand{100}, 1000, 60, 100, false); r != 100 {
		t.Fatalf("full resist with top roll: %d", r)
	}
	if r := ResistedDamage(fixedRand{100}, 1000, 60, 100, true); r != 99 {
		t.Fatalf("dot never fully resisted: %d", r)
	}
	if CritDamage(100, true, 0) != 200 || CritDamage(100, false, 0) != 150 || CritDamage(100, true, 1) != 201 {
		t.Fatal("crit bonus")
	}
}
