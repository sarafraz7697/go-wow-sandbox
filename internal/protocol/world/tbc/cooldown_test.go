package tbc

import (
	"testing"
	"time"

	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

func TestStartCooldownsMillisecondPrecision(t *testing.T) {
	h, s, _ := castFixture(t)
	info := h.Data.Spell(433)
	now := time.Now()
	// Recovery 5000 ms was truncated to whole seconds before; a 1500 ms spell
	// must stay blocked for the full 1.5 s.
	info.Recovery = 1500
	st := &castState{info: info, cat: info.Category}
	h.startCooldowns(s, now, st)
	end := s.sp().cooldowns[433]
	want := now.Add(1500 * time.Millisecond)
	if end.Sub(want) > 10*time.Millisecond || want.Sub(end) > 10*time.Millisecond {
		t.Fatalf("cooldown end %v want %v", end, want)
	}
	// Expired entries are pruned by the tick.
	h.spellTick(s, end.Add(time.Second))
	if _, ok := s.sp().cooldowns[433]; ok {
		t.Fatal("expired cooldown must be pruned")
	}
}

func TestProhibitSpellSchoolLocksMatchingSpells(t *testing.T) {
	h, s, ops := castFixture(t)
	s.player.Spells = append(s.player.Spells, 500)
	food := h.Data.Spell(433)
	// 433 has no school mask set (0): add a fire spell 500 to the tables.
	h.Data.SetSpellTables([]gamedata.SpellInfo{{ID: 500, SchoolMask: 4}, *food}, nil, nil)
	h.prohibitSpellSchool(s, 4, 6000)
	if _, ok := s.sp().cooldowns[500]; !ok {
		t.Fatal("fire spell must be locked")
	}
	if _, ok := s.sp().cooldowns[433]; ok {
		t.Fatal("unaffected spell must not be locked")
	}
	found := false
	for _, o := range *ops {
		if o == op.SMSG_SPELL_COOLDOWN {
			found = true
		}
	}
	if !found {
		t.Fatal("SMSG_SPELL_COOLDOWN must be sent")
	}
	if len(codec.SpellCooldownPacket(1, []codec.CooldownEntry{{Spell: 5, Remain: 100}}).Payload) != 17 {
		t.Fatal("packet layout")
	}
	if len(codec.ClearCooldownPacket(5, 1).Payload) != 12 {
		t.Fatal("clear layout")
	}
}
