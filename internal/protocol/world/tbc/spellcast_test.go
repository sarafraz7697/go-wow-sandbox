package tbc

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"wowserver/internal/domain/character"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

// castFixture builds a handler whose catalog knows "Food" (spell 433): a
// seated 18 s periodic heal that is also instant-cast.
func castFixture(t *testing.T) (*Handler, *Session, *[]uint32) {
	t.Helper()
	c := &gamedata.Catalog{}
	food := gamedata.SpellInfo{
		ID: 433, CastingTimeIndex: 1, DurationIndex: 2, AuraInterruptFlags: gamedata.AuraInterruptNotSeated,
		StartRecoveryTime: 1500, StartRecoveryCategory: 133, Recovery: 5000,
		Effect: [3]uint32{gamedata.EffectApplyAura}, Aura: [3]uint32{8}, AuraPeriod: [3]uint32{3000},
		BasePoints: [3]int32{60}, DieSides: [3]int32{1}, BaseDice: [3]uint32{1}, TargetA: [3]uint32{1},
	}
	c.SetSpellTables([]gamedata.SpellInfo{food},
		map[uint32]gamedata.CastTime{1: {Base: 0}}, map[uint32]gamedata.Duration{2: {Base: 18000}})
	h := &Handler{Data: c}
	var ops []uint32
	p := &character.Player{Summary: character.Summary{GUID: 7, Level: 5}, Entered: true, Health: 10, Spells: []uint32{433}, Stats: &character.Stats{MaxHealth: 200}}
	s := &Session{player: p, log: slog.New(slog.NewTextHandler(io.Discard, nil)), send: func(ps ...world.Packet) error {
		for _, pk := range ps {
			ops = append(ops, pk.Opcode)
		}
		return nil
	}}
	return h, s, &ops
}

func TestCastFoodAppliesSeatedPeriodicHeal(t *testing.T) {
	h, s, ops := castFixture(t)
	now := time.Now()
	h.prepareSpell(s, now, h.Data.Spell(433), 1, codec.SpellTargets{}, nil, false)
	want := []uint32{op.SMSG_SPELL_START, op.SMSG_CLEAR_EXTRA_AURA_INFO, op.SMSG_SPELL_GO}
	for i, o := range want {
		if i >= len(*ops) || (*ops)[i] != o {
			t.Fatalf("packet %d: got %v want prefix %v", i, *ops, want)
		}
	}
	if !s.sitting || s.spells.auras.BySpell(433) == nil {
		t.Fatalf("seated %v aura %v", s.sitting, s.spells.auras.BySpell(433))
	}
	// first tick after one 3 s period heals 61
	h.spellTick(s, now.Add(250*time.Millisecond))
	h.spellTick(s, now.Add(3250*time.Millisecond))
	if s.player.Health != 71 {
		t.Fatalf("health %d", s.player.Health)
	}
	// standing up removes the aura
	h.setStandState(s, codec.StandStateStand)
	if s.spells.auras.BySpell(433) != nil {
		t.Fatal("aura survived standing up")
	}
}

func TestCooldownAndGlobalCooldown(t *testing.T) {
	h, s, ops := castFixture(t)
	now := time.Now()
	info := h.Data.Spell(433)
	h.prepareSpell(s, now, info, 1, codec.SpellTargets{}, nil, false)
	*ops = nil
	h.prepareSpell(s, now.Add(time.Second), info, 2, codec.SpellTargets{}, nil, false)
	if len(*ops) != 1 || (*ops)[0] != op.SMSG_CAST_FAILED {
		t.Fatalf("second cast must fail with NOT_READY, got %v", *ops)
	}
}
