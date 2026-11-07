package gamedata

import "testing"

func TestLevelExtrapolationAndClamp(t *testing.T) {
	c := &Catalog{maxLevel: 70, levels: map[levelKey]Level{}, classLevels: map[classLevelKey]ClassLevel{}}
	c.levels[levelKey{Pair{1, 1}, 70}] = Level{Pair: Pair{1, 1}, Level: 70, Stats: [5]uint32{100, 50, 90, 20, 30}}
	c.classLevels[classLevelKey{1, 70}] = ClassLevel{Class: 1, Level: 70, Health: 5000}
	v, ok := c.Level(1, 1, 80)
	if !ok || v.Level != 80 {
		t.Fatalf("level 80: %+v %v", v, ok)
	}
	// warrior above 38: +2 str/sta, +1 agi, odd/even intellect and spirit; 11 steps (reference loop runs max-1..level)
	if v.Stats[0] != 100+22 || v.Stats[2] != 90+22 || v.Stats[1] != 50+11 {
		t.Fatalf("stats %v", v.Stats)
	}
	if cl, ok := c.ClassLevel(1, 80); !ok || cl.Health != 5000 {
		t.Fatalf("class level %+v %v", cl, ok)
	}
	if _, ok := c.Level(1, 1, 0); ok {
		t.Fatal("level 0 must fail")
	}
}
