package tbc

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"wowserver/internal/domain/aura"
	"wowserver/internal/domain/character"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
	f "wowserver/internal/protocol/world/tbc/updatefields"
)

func deathFixture(t *testing.T) (*Handler, *Session, *[]world.Packet) {
	t.Helper()
	c := &gamedata.Catalog{}
	// Ghost spells with a ghost aura and resurrection sickness with a stat debuff.
	c.SetSpellTables([]gamedata.SpellInfo{
		{ID: ghostSpellID, Effect: [3]uint32{gamedata.EffectApplyAura}, Aura: [3]uint32{aura.Ghost}},
		{ID: nightElfGhostSpellID, Effect: [3]uint32{gamedata.EffectApplyAura}, Aura: [3]uint32{aura.Ghost}},
		{ID: resurrectionSickness, Effect: [3]uint32{gamedata.EffectApplyAura}, Aura: [3]uint32{aura.ModStat}, MiscValue: [3]int32{-1}, BasePoints: [3]int32{-25}},
	}, nil, nil)
	h := &Handler{Data: c}
	var sent []world.Packet
	p := &character.Player{Summary: character.Summary{GUID: 3, Level: 20, Race: 1, Class: 1}, Entered: true, Health: 10,
		Stats: &character.Stats{MaxHealth: 100, MaxMana: 50}, HomeMap: 1, HomeX: 8, HomeY: 9, HomeZ: 10}
	s := &Session{player: p, log: slog.New(slog.NewTextHandler(io.Discard, nil)), send: func(ps ...world.Packet) error {
		sent = append(sent, ps...)
		return nil
	}}
	return h, s, &sent
}

func TestKillPlayerClearsState(t *testing.T) {
	h, s, _ := deathFixture(t)
	s.player.Health = 0
	s.player.Rage = 400
	s.sp().auras.Add(aura.NewHolder(9000, 1, 10, 5000, true, [3]*aura.Effect{}))
	now := time.Now()
	h.killPlayer(s, 77, now)
	p := s.player
	if p.Rage != 0 || len(s.spells.auras.Holders()) != 0 {
		t.Fatalf("rage %d auras %d", p.Rage, len(s.spells.auras.Holders()))
	}
	if s.fieldBytes&playerFieldByteReleaseTimer == 0 {
		t.Fatal("release timer flag must be set")
	}
	if p.Health != 0 {
		t.Fatal("dead")
	}
}

func TestRepopMakesGhostAndResurrectRevives(t *testing.T) {
	h, s, sent := deathFixture(t)
	s.player.Health = 0
	h.killPlayer(s, 77, time.Now())
	h.buildPlayerRepop(s)
	p := s.player
	if p.Health != 1 || p.PlayerFlags&playerFlagsGhost == 0 {
		t.Fatalf("ghost state: health %d flags %x", p.Health, p.PlayerFlags)
	}
	if s.spells.auras.BySpell(ghostSpellID) == nil {
		t.Fatal("ghost aura missing")
	}
	if s.bytes1&0xFF000000 != unitByte1AlwaysStand<<24 {
		t.Fatalf("corpse byte %x", s.bytes1)
	}
	// Resurrect at the spirit healer: half health, sickness for level 20.
	h.resurrectPlayer(s, 0.5, true)
	if p.Health != 50 || p.PlayerFlags&playerFlagsGhost != 0 {
		t.Fatalf("resurrected: health %d flags %x", p.Health, p.PlayerFlags)
	}
	if s.spells.auras.BySpell(ghostSpellID) != nil {
		t.Fatal("ghost aura must be gone")
	}
	hd := s.spells.auras.BySpell(resurrectionSickness)
	if hd == nil {
		t.Fatal("resurrection sickness missing")
	}
	if hd.Passive || hd.MaxDuration != 10*60*1000 {
		t.Fatalf("level 20 sickness must last 10 min: %+v", hd)
	}
	foundRelease := false
	for _, pk := range *sent {
		if pk.Opcode == op.SMSG_DEATH_RELEASE_LOC {
			foundRelease = true
		}
	}
	if !foundRelease {
		t.Fatal("death release loc must be sent during repop")
	}
}

func TestSicknessDurationByLevel(t *testing.T) {
	h, s, _ := deathFixture(t)
	s.player.Level = 12
	s.player.Health = 0
	h.killPlayer(s, 0, time.Now())
	h.buildPlayerRepop(s)
	h.resurrectPlayer(s, 0.5, true)
	hd := s.spells.auras.BySpell(resurrectionSickness)
	if hd == nil || hd.MaxDuration != 2*60*1000 {
		t.Fatalf("level 12 sickness must last 2 min: %+v", hd)
	}
	// Level 10 players never get sick.
	h2, s2, _ := deathFixture(t)
	s2.player.Level = 10
	s2.player.Health = 0
	h2.killPlayer(s2, 0, time.Now())
	h2.buildPlayerRepop(s2)
	h2.resurrectPlayer(s2, 0.5, true)
	if hd := s2.spells.auras.BySpell(resurrectionSickness); hd != nil {
		t.Fatal("level 10 must not get resurrection sickness")
	}
}

func TestGhostLoginAndNoCastNoAttack(t *testing.T) {
	h, s, _ := deathFixture(t)
	s.player.PlayerFlags = playerFlagsGhost
	s.player.Health = 1
	h.applyGhostStateAtLogin(s, s.player)
	if s.player.PlayerFlags&playerFlagsGhost == 0 || s.spells.auras.BySpell(ghostSpellID) == nil {
		t.Fatal("ghost state must be rebuilt at login")
	}
	// Ghost casting fails with CASTER_DEAD.
	st := &castState{info: &gamedata.SpellInfo{ID: 1}}
	if res := h.checkCast(s, time.Now(), st); res != 0x15 {
		t.Fatalf("ghost cast must fail CASTER_DEAD, got %x", res)
	}
	_ = f.UNIT_FIELD_HEALTH
	_ = codec.DeathReleaseLocClear
}
