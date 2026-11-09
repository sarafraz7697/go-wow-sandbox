package gamedata

import (
	"math"
	"testing"
)

func TestRatingMultiplierAndBonus(t *testing.T) {
	c := &Catalog{combatRatings: make([]float32, 24*100)}
	// gtCombatRatings row cr*100+level-1: ~15.77 hit rating points per
	// percent for CR_HIT_MELEE at level 70 (TBC values).
	c.combatRatings[CRHitMelee*100+69] = 15.77
	if m := c.RatingMultiplier(CRHitMelee, 70); math.Abs(float64(m)-1/15.77) > 1e-6 {
		t.Fatalf("multiplier %f", m)
	}
	if b := c.RatingBonus(CRHitMelee, 70, 16); math.Abs(float64(b*100)-100*16/15.77) > 0.01 {
		t.Fatalf("bonus %f", b)
	}
	// The level is clamped to GT_MAX_LEVEL.
	if m := c.RatingMultiplier(CRHitMelee, 170); m != 1 {
		t.Fatalf("clamped level must hit the unset row 100 fallback: %f", m)
	}
	// A missing table (or row) yields multiplier 1 like the reference.
	c2 := &Catalog{}
	if m := c2.RatingMultiplier(CRHitMelee, 70); m != 1 {
		t.Fatalf("missing table multiplier %f", m)
	}
}

func TestDodgeFromAgility(t *testing.T) {
	// Warrior (class 1) at level 70: base 0.75% plus agility * ratio * 1.1.
	c := &Catalog{meleeCritRatio: make([]float32, 11*100)}
	c.meleeCritRatio[(1-1)*100+69] = 0.00003750
	d := c.DodgeFromAgility(1, 70, 100)
	want := float32((0.0075 + 100*0.0000375*1.1) * 100)
	if math.Abs(float64(d-want)) > 1e-6 {
		t.Fatalf("warrior dodge %f want %f", d, want)
	}
	// Unknown class and out-of-range tables yield 0.
	if c.DodgeFromAgility(0, 70, 100) != 0 || c.DodgeFromAgility(12, 70, 100) != 0 {
		t.Fatal("class bounds")
	}
	if (&Catalog{}).DodgeFromAgility(1, 70, 100) != 0 {
		t.Fatal("missing ratio table must yield 0")
	}
}

func TestPlayerCanParryAndBlock(t *testing.T) {
	// A parry-granting passive spell (SPELL_EFFECT_PARRY = 22) plus an
	// unrelated spell and a block-granting one (SPELL_EFFECT_BLOCK = 23).
	c := &Catalog{spellInfo: map[uint32]*SpellInfo{
		10: {ID: 10, Effect: [3]uint32{22, 0, 0}},
		11: {ID: 11, Effect: [3]uint32{0, 0, 0}},
		12: {ID: 12, Effect: [3]uint32{0, 23, 0}},
	}}
	if !c.PlayerCanParry([]uint32{10, 11}) {
		t.Fatal("spell 10 grants parry")
	}
	if c.PlayerCanParry([]uint32{11}) {
		t.Fatal("spell 11 grants nothing")
	}
	if !c.PlayerCanBlock([]uint32{11, 12}) {
		t.Fatal("spell 12 grants block")
	}
	if c.PlayerCanBlock([]uint32{10, 11}) {
		t.Fatal("spell 10 does not grant block")
	}
}
