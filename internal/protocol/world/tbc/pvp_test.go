package tbc

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"wowserver/internal/domain/aura"
	"wowserver/internal/domain/character"
	"wowserver/internal/domain/combat"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

func pvpFixture(t *testing.T) (*Handler, *Session, *Session) {
	t.Helper()
	c := &gamedata.Catalog{}
	// 700: fire school damage + periodic damage aura (shadow word: pain like).
	c.SetSpellTables([]gamedata.SpellInfo{
		{ID: 700, SchoolMask: 4, DefenseType: gamedata.DamageClassMagic, Speed: 0,
			Effect: [3]uint32{gamedata.EffectSchoolDamage}, BasePoints: [3]int32{20}, DieSides: [3]int32{1}, BaseDice: [3]uint32{1},
			TargetA: [3]uint32{targetSingleEnemy}, CastingTimeIndex: 1, DurationIndex: 2},
		{ID: 701, SchoolMask: 32, DefenseType: gamedata.DamageClassMagic,
			Effect: [3]uint32{gamedata.EffectApplyAura}, Aura: [3]uint32{aura.PeriodicDamage}, AuraPeriod: [3]uint32{3000},
			BasePoints: [3]int32{5}, DieSides: [3]int32{1}, BaseDice: [3]uint32{1}, TargetA: [3]uint32{targetSingleEnemy},
			CastingTimeIndex: 1, DurationIndex: 2},
	}, map[uint32]gamedata.CastTime{1: {Base: 0}}, map[uint32]gamedata.Duration{2: {Base: 18000}})
	c.SetLevelForTest(1, 1, 10, 10, gamedata.Level{Stats: [5]uint32{20, 20, 20, 20, 20}})
	c.SetClassLevelForTest(1, 10, gamedata.ClassLevel{Health: 40})
	c.SetRaceForTest(1, gamedata.Race{ID: 1, Team: teamAlliance, Faction: 1})
	c.SetRaceForTest(2, gamedata.Race{ID: 2, Team: teamHorde, Faction: 2})
	h := &Handler{Data: c}

	mk := func(guid uint32, race uint8, x float32) *Session {
		p := &character.Player{Summary: character.Summary{GUID: guid, Name: "p", Level: 10, Race: race, Class: 1, Map: 1, X: x, Y: 0, Z: 0},
			Entered: true, Health: 100, Stats: &character.Stats{MaxHealth: 100, Resistances: [7]uint32{10, 0, 0, 0, 0, 0, 0}, DodgePct: 5, ParryPct: 5, BlockPct: 5},
			Spells: []uint32{700, 701}}
		s := &Session{player: p, log: slog.New(slog.NewTextHandler(io.Discard, nil)), send: func(ps ...world.Packet) error { return nil }}
		h.pvpRegister(s)
		return s
	}
	attacker := mk(11, 1, 0) // human (Alliance)
	victim := mk(22, 2, 2)   // orc (Horde)
	victim.player.X = 2 // within melee reach
	return h, attacker, victim
}

func TestPvpRegistryLookupAndSnapshot(t *testing.T) {
	h, _, victim := pvpFixture(t)
	snap, ok := h.pvpLookup(22)
	if !ok || snap.guid != 22 || snap.team != teamHorde || snap.health != 100 {
		t.Fatalf("snapshot %+v ok=%v", snap, ok)
	}
	if _, ok := h.pvpLookup(99); ok {
		t.Fatal("unknown player must not resolve")
	}
	h.pvpUnregister(victim)
	if _, ok := h.pvpLookup(22); ok {
		t.Fatal("unregistered player must not resolve")
	}
}

func TestPvpMeleeSwingQueuesDamageAndKills(t *testing.T) {
	h, attacker, victim := pvpFixture(t)
	now := time.Now()
	h.attackPlayer(attacker, uint64(22), now)
	if !attacker.combat.active || attacker.combat.victim != 22 {
		t.Fatal("attack state")
	}
	// Land swings until the victim dies; each swing queues one damage event.
	for i := 0; i < 200 && victim.player.Health != 0; i++ {
		h.combatTick(attacker, now.Add(time.Duration(i)*time.Second))
		h.drainPvpEvents(victim, now.Add(time.Duration(i)*time.Second))
		h.pvpUpdateSnapshot(victim)
	}
	if victim.player.Health != 0 {
		t.Fatalf("victim must die, health %d", victim.player.Health)
	}
	if victim.player.PlayerFlags&playerFlagsGhost != 0 {
		t.Fatal("death alone must not ghost the player (release comes after)")
	}
	// The attacker stops swinging at the corpse.
	h.combatTick(attacker, now.Add(time.Minute))
	if attacker.combat.active {
		t.Fatal("attacker must leave combat when the victim died")
	}
}

func TestPvpSpellDamageAndDotKill(t *testing.T) {
	h, attacker, victim := pvpFixture(t)
	now := time.Now()
	// Direct damage spell on the player target.
	tg := codec.SpellTargets{Mask: codec.TargetFlagUnit, Unit: 22}
	h.prepareSpell(attacker, now, h.Data.Spell(700), 1, tg, nil, false)
	h.drainPvpEvents(victim, now.Add(50*time.Millisecond))
	if victim.player.Health >= 100 {
		t.Fatalf("spell damage must land, health %d", victim.player.Health)
	}
	// A DoT: the aura holder must live on the victim session and tick. The
	// victim is wounded to 20 so one pain can finish the job.
	victim.player.Health = 20
	h.pvpUpdateSnapshot(victim)
	h.prepareSpell(attacker, now.Add(time.Second), h.Data.Spell(701), 1, tg, nil, false)
	h.drainPvpEvents(victim, now.Add(1100*time.Millisecond))
	hd := victim.spells.auras.BySpell(701)
	if hd == nil || hd.Caster != 11 {
		t.Fatalf("DoT holder must sit on the victim: %+v", hd)
	}
	for i := 1; i <= 100 && victim.player.Health != 0; i++ {
		h.spellTick(victim, now.Add(time.Duration(i)*3*time.Second))
	}
	if victim.player.Health != 0 {
		t.Fatalf("the DoT must be able to kill, health %d", victim.player.Health)
	}
	if victim.spells.auras.BySpell(701) == nil && victim.player.Health != 0 {
		t.Fatal("aura vanished before expiry or death")
	}
}

func TestPvpGhostVictimsCannotBeAttacked(t *testing.T) {
	h, attacker, victim := pvpFixture(t)
	now := time.Now()
	victim.player.PlayerFlags |= playerFlagsGhost
	victim.player.Health = 1
	h.pvpUpdateSnapshot(victim)
	h.attackPlayer(attacker, uint64(22), now)
	h.combatTick(attacker, now.Add(100*time.Millisecond))
	if attacker.combat.active {
		t.Fatal("ghost victims stop the attack")
	}
	// Ghost casting is rejected with CASTER_DEAD for the attacker too.
	attacker.player.PlayerFlags |= playerFlagsGhost
	st := &castState{info: h.Data.Spell(700), targets: codec.SpellTargets{Mask: codec.TargetFlagUnit, Unit: 22}}
	if res := h.checkCast(attacker, now, st); res != 0x15 {
		t.Fatalf("ghost cast must fail CASTER_DEAD, got %x", res)
	}
	_ = op.SMSG_ATTACKSTART
	_ = combat.SpellMissNone
}
