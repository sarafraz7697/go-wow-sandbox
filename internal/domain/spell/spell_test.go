package spell

import (
	"testing"

	"wowserver/internal/gamedata"
)

func TestEffectValueAndCost(t *testing.T) {
	s := &gamedata.SpellInfo{BasePoints: [3]int32{60}, DieSides: [3]int32{1}, BaseDice: [3]uint32{1}, ManaCost: 50, ManaCostPct: 10}
	if v := EffectValue(s, 0, 5, nil); v != 61 {
		t.Fatalf("value %d", v) // base points + 1 die
	}
	s.DieSides[0], s.BaseDice[0] = 5, 1
	if v := EffectValue(s, 0, 5, func(lo, hi int32) int32 { return hi }); v != 65 {
		t.Fatalf("random value %d", v)
	}
	if c := PowerCost(s, 0, 0, 200, [5]uint32{}, [5]uint32{}); c != 70 {
		t.Fatalf("cost %d", c)
	}
}
