package tbc

import (
	"testing"
	"time"

	"wowserver/internal/domain/spell"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world/tbc/codec"
)

func comboFixture(t *testing.T) (*Handler, *Session) {
	t.Helper()
	h, s, _ := castFixture(t)
	s.player.Spells = append(s.player.Spells, 600, 601, 602, 603)
	food := h.Data.Spell(433)
	// 600: builder (weapon damage + 1 combo point). 601: finisher (school
	// damage + points per combo). 602: interrupt (4 s lockout). 603: heal 30%.
	h.Data.SetSpellTables([]gamedata.SpellInfo{
		{ID: 600, DefenseType: gamedata.DamageClassMelee, CastingTimeIndex: 1, AttributesEx: 0x00100000,
			Effect: [3]uint32{gamedata.EffectAddComboPoints, gamedata.EffectWeaponDamageNoSchool}, BasePoints: [3]int32{1, 10}, DieSides: [3]int32{1, 1}, BaseDice: [3]uint32{1, 1}, TargetA: [3]uint32{targetSingleEnemy}},
		{ID: 601, DefenseType: gamedata.DamageClassMelee, CastingTimeIndex: 1, AttributesEx: 0x00100000,
			Effect: [3]uint32{gamedata.EffectSchoolDamage}, BasePoints: [3]int32{10}, DieSides: [3]int32{1}, BaseDice: [3]uint32{1},
			EffectPointsPerCombo: [3]float32{7}, TargetA: [3]uint32{targetSingleEnemy}},
		{ID: 602, SchoolMask: 4, CastingTimeIndex: 1, DurationIndex: 2,
			Effect: [3]uint32{gamedata.EffectInterruptCast}, BasePoints: [3]int32{1}, TargetA: [3]uint32{targetSingleEnemy}},
		{ID: 603, CastingTimeIndex: 1, Effect: [3]uint32{gamedata.EffectHealPct}, BasePoints: [3]int32{30}},
		*food,
	}, map[uint32]gamedata.CastTime{1: {Base: 0}}, map[uint32]gamedata.Duration{2: {Base: 18000}})
	return h, s
}

func TestComboPointsBuildStackAndReset(t *testing.T) {
	h, s := comboFixture(t)
	h.addComboPoints(s, 99, 1)
	h.addComboPoints(s, 99, 1)
	h.addComboPoints(s, 99, 1)
	if s.spells.comboPoints != 3 || s.spells.comboTarget != 99 {
		t.Fatalf("points %d target %d", s.spells.comboPoints, s.spells.comboTarget)
	}
	// New target resets.
	h.addComboPoints(s, 42, 2)
	if s.spells.comboPoints != 2 || s.spells.comboTarget != 42 {
		t.Fatalf("after retarget %d %d", s.spells.comboPoints, s.spells.comboTarget)
	}
	// Five is the cap.
	for i := 0; i < 6; i++ {
		h.addComboPoints(s, 42, 1)
	}
	if s.spells.comboPoints != 5 {
		t.Fatalf("cap %d", s.spells.comboPoints)
	}
	h.clearComboPoints(s)
	if s.spells.comboPoints != 0 || s.spells.comboTarget != 0 {
		t.Fatal("cleared")
	}
}

func TestFinisherRequiresAndConsumesComboPoints(t *testing.T) {
	h, s := comboFixture(t)
	now := time.Now()
	tg := codec.SpellTargets{Mask: codec.TargetFlagUnit, Unit: 99}
	// Without points the finisher fails with NO_COMBO_POINTS.
	st := &castState{info: h.Data.Spell(601), targets: tg}
	if res := h.checkSpellTarget(s, now, st, true); res != 0x49 {
		t.Fatalf("finisher without points must fail NO_COMBO_POINTS, got %x", res)
	}
	// Builder: the effect switches target via the enemy path only for real
	// creatures, so drive the state directly and verify the requirement clears.
	h.addComboPoints(s, 99, 3)
	if res := h.checkSpellTarget(s, now, st, true); res == 0x49 {
		t.Fatal("finisher with points must pass the combo requirement")
	}
	// EffectValue scales with the combo points (Unit::CalculateSpellDamage).
	info := h.Data.Spell(601)
	base := spell.EffectValue(info, 0, uint32(s.player.Level), irand)
	if v := spell.ComboScaled(info, 0, base, 3); v != base+7*3 {
		t.Fatalf("combo scaled damage %d (base %d)", v, base)
	}
}

func TestDamageInterruptsCastableSpell(t *testing.T) {
	h, s, _ := castFixture(t)
	now := time.Now()
	// A long cast with SPELL_INTERRUPT_FLAG_DAMAGE.
	info := h.Data.Spell(433)
	info.InterruptFlags = 0x02
	h.Data.SetSpellTables([]gamedata.SpellInfo{*info}, nil, nil)
	// Direct damage from another player interrupts it.
	ev := pvpEvent{kind: pvpDamage, attacker: 5, damage: 10}
	// First start a real cast: castMS>0 needed; fixture cast time is 0, so
	// emulate the pending state.
	s.sp().cast = &castState{info: info, count: 1}
	h.applyPvpEvent(s, ev, now)
	if s.spells.cast != nil {
		t.Fatal("damage must cancel the interruptible cast")
	}
	// A cast without the damage flag survives.
	s.sp().cast = &castState{info: &gamedata.SpellInfo{ID: 999, InterruptFlags: 0}, count: 1}
	h.applyPvpEvent(s, pvpEvent{kind: pvpDamage, attacker: 5, damage: 10}, now)
	if s.spells.cast == nil {
		t.Fatal("non-interruptible cast must survive damage")
	}
}
