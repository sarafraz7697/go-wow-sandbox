package combat

import (
	"math"
	"testing"
)

type fixed struct {
	roll uint32
	norm float32
}

func (f fixed) Urand(lo, hi uint32) uint32 {
	if f.roll < lo {
		return lo
	}
	return min(f.roll, hi)
}
func (f fixed) Norm() float32 { return f.norm }

func TestRollOrderMissDodgeParryGlancingBlockCritCrush(t *testing.T) {
	att := Side{Level: 10, IsPlayer: true, WeaponSkill: 50, MaxSkill: 50}
	vic := Side{Level: 12, Defense: 60, MaxSkill: 60}
	c := Chances{Miss: 500, Crit: 500, Dodge: 500, Parry: 500, Block: 500}
	// skillBonus = 4*(50-60) = -40, so dodge/parry/block are 540 wide each.
	cases := []struct {
		roll uint32
		want Outcome
	}{
		{0, HitMiss}, {499, HitMiss},
		{500, HitDodge}, {1039, HitDodge},
		{1040, HitParry}, {1579, HitParry},
		// glancing: (10 + (60-50))*100 = 2000 wide
		{1580, HitGlancing}, {3579, HitGlancing},
		{3580, HitBlock}, {4119, HitBlock},
		{4120, HitCrit}, {4619, HitCrit},
		{4620, HitNormal}, {10000, HitNormal},
	}
	for _, tc := range cases {
		if got := Roll(fixed{roll: tc.roll}, att, vic, c, false); got != tc.want {
			t.Errorf("roll %d: got %v want %v", tc.roll, got, tc.want)
		}
	}
	// From behind: parry and block are impossible, dodge still applies to creatures.
	if got := Roll(fixed{roll: 1100}, att, vic, c, true); got == HitParry || got == HitBlock {
		t.Errorf("from behind got %v", got)
	}
}

func TestCrushingBlowOnlyForCreatureAttackers(t *testing.T) {
	att := Side{Level: 63, MaxSkill: 315}
	vic := Side{Level: 60, IsPlayer: true, Defense: 300, MaxSkill: 300}
	// diff 15 -> 1500 wide; with no other chances roll 0 crushes.
	if got := Roll(fixed{roll: 0}, att, vic, Chances{}, false); got != HitCrushing {
		t.Fatalf("got %v", got)
	}
	att.IsPlayer = true
	if got := Roll(fixed{roll: 0}, att, vic, Chances{}, false); got != HitNormal {
		t.Fatalf("player attacker: got %v", got)
	}
}

func TestMissChanceAndArmor(t *testing.T) {
	if got := MissChance(300, 300, false); got != 5 {
		t.Fatalf("even skill miss %v", got)
	}
	// skill diff -15 against a creature: 5 - ((-15+10)*0.4 - 2) = 9
	if got := MissChance(300, 315, false); math.Abs(float64(got)-9) > 1e-4 {
		t.Fatalf("skill diff -15: %v", got)
	}
	if got := MissChance(0, 2000, true); got != 60 {
		t.Fatalf("cap %v", got)
	}
	if got := ArmorReduced(60, 0, 100); got != 100 {
		t.Fatalf("no armor %d", got)
	}
	// 75% cap and minimum of 1
	if got := ArmorReduced(1, 1_000_000, 100); got != 25 {
		t.Fatalf("capped armor %d", got)
	}
	if got := ArmorReduced(1, 1_000_000, 1); got != 1 {
		t.Fatalf("min damage %d", got)
	}
}

func TestCalculateCritBlockGlancing(t *testing.T) {
	att := Side{Level: 10, IsPlayer: true, Class: 1, WeaponSkill: 50, MaxSkill: 50}
	vic := Side{Level: 10, Defense: 50, MaxSkill: 50, ShieldBlock: 7}
	// all chance zero except crit 100%: roll 0 -> crit, damage doubled
	r := Calculate(fixed{roll: 0}, att, vic, Chances{Crit: 10000}, false, 10, 10, false, 0)
	if r.Outcome != HitCrit || r.Damage != 20 || r.HitInfo&HitInfoCritical == 0 || r.TargetState != VictimNormal {
		t.Fatalf("crit %+v", r)
	}
	r = Calculate(fixed{roll: 0}, att, vic, Chances{Block: 10000}, false, 10, 10, false, 0)
	if r.Outcome != HitBlock || r.Damage != 3 || r.Blocked != 7 {
		t.Fatalf("block %+v", r)
	}
	r = Calculate(fixed{roll: 0}, att, vic, Chances{Block: 10000}, false, 5, 5, false, 0)
	if r.TargetState != VictimBlocks || r.Damage != 0 || r.Blocked != 5 {
		t.Fatalf("full block %+v", r)
	}
	r = Calculate(fixed{roll: 0}, att, vic, Chances{Miss: 10000}, false, 10, 10, false, 0)
	if r.HitInfo&HitInfoMiss == 0 || r.Damage != 0 {
		t.Fatalf("miss %+v", r)
	}
}

func TestRageAndArc(t *testing.T) {
	// level 1, 10 damage, 2.0s hit: factor 7 -> ((10/conv*7.5)+7)/2
	f := RageWeaponFactor(2000, false)
	if f != 7 {
		t.Fatalf("factor %d", f)
	}
	if got := RageReward(1, 10, f, 1); got == 0 || got > 100 {
		t.Fatalf("rage %d", got)
	}
	if !HasInArc(0, 0, 0, 10, 0, math.Pi/2) || HasInArc(0, 0, math.Pi, 10, 0, math.Pi/2) {
		t.Fatal("arc facing")
	}
}
