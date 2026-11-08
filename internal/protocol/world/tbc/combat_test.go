package tbc

import (
	"testing"
	"time"

	"wowserver/internal/gamedata"
)

func TestCreatureLifeDecayAndRespawn(t *testing.T) {
	h := &Handler{}
	now := time.Now()
	ref := gamedata.CreatureSpawnRef{GUID: 5}
	h.life.m = map[uint32]*creatureLife{5: {spawn: ref, dead: true, decayAt: now.Add(10 * time.Second), respawnAt: now.Add(20 * time.Second)}}
	if hp, set, vis := h.creatureVisibleState(5); hp != 0 || !set || !vis {
		t.Fatalf("corpse state %v %v %v", hp, set, vis)
	}
	if _, set, vis := h.creatureVisibleState(6); set || !vis {
		t.Fatal("untouched creature must be visible with default health")
	}
	h.tickCreatureLife(now.Add(11 * time.Second))
	if _, _, vis := h.creatureVisibleState(5); vis {
		t.Fatal("decayed corpse must be hidden")
	}
	h.tickCreatureLife(now.Add(13 * time.Second)) // throttled to 1 s, still hidden
	h.tickCreatureLife(now.Add(21 * time.Second))
	if _, set, vis := h.creatureVisibleState(5); set || !vis {
		t.Fatal("respawned creature must be back with full health")
	}
}

func TestDamagedLivingCreatureIsNotRespawned(t *testing.T) {
	h := &Handler{}
	now := time.Now()
	ref := gamedata.CreatureSpawnRef{GUID: 9}
	h.life.m = map[uint32]*creatureLife{9: {spawn: ref, health: 40}}
	h.tickCreatureLife(now.Add(5 * time.Second))
	if hp, set, vis := h.creatureVisibleState(9); hp != 40 || !set || !vis {
		t.Fatalf("a wounded living creature must keep its health: %v %v %v", hp, set, vis)
	}
}

func TestCorpseDecayByRank(t *testing.T) {
	want := map[uint32]time.Duration{0: 300 * time.Second, 1: 600 * time.Second, 2: 1200 * time.Second, 3: 3600 * time.Second, 4: 900 * time.Second}
	for rank, d := range want {
		if got := corpseDecay(rank); got != d {
			t.Errorf("rank %d: %v want %v", rank, got, d)
		}
	}
}

func TestSwingPacketsLayout(t *testing.T) {
	if p := attackStartPacket(1, 2); len(p.Payload) != 16 {
		t.Fatalf("attack start %d", len(p.Payload))
	}
	if p := attackStopPacket(7, 0); len(p.Payload) != 2+1+4 {
		t.Fatalf("attack stop %d", len(p.Payload))
	}
}
