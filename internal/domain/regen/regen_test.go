package regen

import "testing"

func TestHealthAndMana(t *testing.T) {
	s := State{Health: 50, MaxHealth: 100, MaxPower: [5]uint32{100, 1000, 0, 100, 0}, OCTRegenHP: 3}
	s.Power[Mana], s.ManaRegen = 10, 4
	Tick(&s, Mods{}, DefaultRates)
	if s.Health != 53 || s.Power[Mana] != 18 {
		t.Fatalf("health %d mana %d", s.Health, s.Power[Mana])
	}
	s.Sitting = true
	Tick(&s, Mods{}, DefaultRates)
	if s.Health != 57 { // int(3*1.5)=4
		t.Fatalf("sitting health %d", s.Health)
	}
	s.RecentManaUse, s.ManaRegenInterrupt = true, 0
	m := s.Power[Mana]
	Tick(&s, Mods{}, DefaultRates)
	if s.Power[Mana] != m {
		t.Fatal("interrupted mana regenerated")
	}
}

func TestRageDecayEnergyCombat(t *testing.T) {
	s := State{Health: 10, MaxHealth: 100, MaxPower: [5]uint32{0, 1000, 0, 100, 0}, OCTRegenHP: 5}
	s.Power[Rage], s.Power[Energy] = 50, 90
	s.InCombat = true
	Tick(&s, Mods{}, DefaultRates)
	if s.Health != 10 || s.Power[Rage] != 50 || s.Power[Energy] != 100 {
		t.Fatalf("combat: %+v", s)
	}
	s.InCombat = false
	Tick(&s, Mods{}, DefaultRates)
	if s.Power[Rage] != 30 || s.Health != 15 {
		t.Fatalf("out of combat: %+v", s)
	}
	Tick(&s, Mods{}, DefaultRates)
	Tick(&s, Mods{}, DefaultRates)
	if s.Power[Rage] != 0 {
		t.Fatalf("rage %d", s.Power[Rage])
	}
}
