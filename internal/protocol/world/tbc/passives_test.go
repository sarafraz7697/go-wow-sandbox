package tbc

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"wowserver/internal/application/character/stats"
	"wowserver/internal/domain/aura"
	"wowserver/internal/domain/character"
	"wowserver/internal/domain/spell"
	"wowserver/internal/entity"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world"
)

func passivesFixture(t *testing.T) (*Handler, *Session) {
	t.Helper()
	c := &gamedata.Catalog{}
	// 9000: passive +5 stamina (ModStat index 2). 9001: passive +20 attack
	// power. 9004: passive +10% stamina. 9200: passive melee crit rating +8.
	// 9003: a non-passive buff that must not auto-apply. 9100: enchant spell.
	spells := []gamedata.SpellInfo{
		{ID: 9000, Attributes: gamedata.SpellAttrPassive, Effect: [3]uint32{gamedata.EffectApplyAura}, Aura: [3]uint32{aura.ModStat}, MiscValue: [3]int32{2}, BasePoints: [3]int32{5}},
		{ID: 9001, Attributes: gamedata.SpellAttrPassive, Effect: [3]uint32{gamedata.EffectApplyAura}, Aura: [3]uint32{aura.ModAttackPower}, BasePoints: [3]int32{20}},
		{ID: 9004, Attributes: gamedata.SpellAttrPassive, Effect: [3]uint32{gamedata.EffectApplyAura}, Aura: [3]uint32{aura.ModTotalStatPercentage}, MiscValue: [3]int32{2}, BasePoints: [3]int32{10}},
		{ID: 9200, Attributes: gamedata.SpellAttrPassive, Effect: [3]uint32{gamedata.EffectApplyAura}, Aura: [3]uint32{aura.ModRating}, MiscValue: [3]int32{1 << gamedata.CRCritMelee}, BasePoints: [3]int32{8}},
		{ID: 9003, Effect: [3]uint32{gamedata.EffectApplyAura}, Aura: [3]uint32{aura.ModStat}, MiscValue: [3]int32{2}, BasePoints: [3]int32{99}},
		{ID: 9100, Effect: [3]uint32{gamedata.EffectApplyAura}, Aura: [3]uint32{aura.ModStat}, MiscValue: [3]int32{2}, BasePoints: [3]int32{7}},
	}
	c.SetSpellTables(spells, map[uint32]gamedata.CastTime{}, map[uint32]gamedata.Duration{})
	c.SetItemPrototypesForTest(map[uint32]entity.ItemPrototype{25: {ItemID: 25, Class: 4}})
	c.SetLevelForTest(1, 1, 10, 10, gamedata.Level{Stats: [5]uint32{20, 18, 19, 16, 21}})
	c.SetClassLevelForTest(1, 10, gamedata.ClassLevel{Health: 42})
	h := &Handler{Data: c}
	p := &character.Player{Summary: character.Summary{GUID: 5, Level: 10, Class: 1, Race: 1}, Entered: true, Health: 30,
		Spells: []uint32{9000, 9001, 9004, 9200, 9003}, Stats: &character.Stats{MaxHealth: 100},
		Items: []character.PlayerItem{{GUID: 1, Entry: 25, Slot: 5}}}
	s := &Session{player: p, log: slog.New(slog.NewTextHandler(io.Discard, nil)), send: func(ps ...world.Packet) error { return nil }}
	return h, s
}

func TestApplyPassiveAuras(t *testing.T) {
	h, s := passivesFixture(t)
	h.applyPassiveAuras(s)
	sp := s.sp()
	for _, id := range []uint32{9000, 9001, 9004, 9200} {
		hd := sp.auras.BySpell(id)
		if hd == nil || !hd.Passive || hd.Duration != -1 {
			t.Fatalf("passive %d must be an infinite holder: %+v", id, hd)
		}
	}
	if hd := sp.auras.BySpell(9003); hd != nil {
		t.Fatal("non-passive spell must not be applied")
	}
	m := sp.auras.StatMods()
	if m.Pos[2] != 5 {
		t.Fatalf("stamina mods %+v", m.Pos)
	}
	if m.AttackPower != 20 {
		t.Fatalf("attack power %d", m.AttackPower)
	}
	if m.StatPct[2] < 1.09 || m.StatPct[2] > 1.11 {
		t.Fatalf("stamina percent %v", m.StatPct[2])
	}
	if m.Ratings[gamedata.CRCritMelee] != 8 {
		t.Fatalf("crit rating %d", m.Ratings[gamedata.CRCritMelee])
	}
	// Passives are not persisted (snapshotSpellState skips Passive holders).
	h.snapshotSpellState(s, s.player, time.Now())
	for _, a := range s.player.Auras {
		if a.Spell == 9000 {
			t.Fatal("passive holder must not be persisted")
		}
	}
}

func TestApplyEquipAuras(t *testing.T) {
	h, s := passivesFixture(t)
	// Enchantment 77 (effect 3, spell 9100) on the chest item.
	s.player.Items[0].Fields[22] = 77
	h.Data.SetEnchantmentsForTest(map[uint32]gamedata.Enchantment{77: {Effects: [3]uint32{3}, Arguments: [3]uint32{9100}}})
	h.applyEquipAuras(s)
	if hd := s.spells.auras.BySpell(9100); hd == nil || !hd.Passive || !s.spells.equipAuras[9100] {
		t.Fatalf("equip spell aura: %+v equip=%v", hd, s.spells.equipAuras)
	}
	// Unequipping the item drops the holder.
	s.player.Items = nil
	h.applyEquipAuras(s)
	if hd := s.spells.auras.BySpell(9100); hd != nil {
		t.Fatal("unequipped item aura must be removed")
	}
	if s.spells.equipAuras[9100] {
		t.Fatal("equip bookkeeping must be cleared")
	}
}

func TestStatModsFoldIntoStats(t *testing.T) {
	h, s := passivesFixture(t)
	h.applyPassiveAuras(s)
	m := s.spells.auras.StatMods()
	base, err := stats.Calculate(s.player, h.Data, nil)
	if err != nil {
		t.Fatal(err)
	}
	mod, err := stats.Calculate(s.player, h.Data, &m)
	if err != nil {
		t.Fatal(err)
	}
	if mod.Attributes[2] <= base.Attributes[2] {
		t.Fatalf("stamina with passives %d <= base %d", mod.Attributes[2], base.Attributes[2])
	}
	if mod.AttackPower != base.AttackPower+20 {
		t.Fatalf("attack power %d vs %d", mod.AttackPower, base.AttackPower)
	}
	if mod.Ratings[gamedata.CRCritMelee] != 8 {
		t.Fatalf("crit rating %d", mod.Ratings[gamedata.CRCritMelee])
	}
	if mod.MaxHealth <= base.MaxHealth {
		t.Fatalf("max health %d <= %d", mod.MaxHealth, base.MaxHealth)
	}
	for _, l := range mod.Limitations {
		if l == "passive spells, talents, buffs and forms are not evaluated" || l == "item equip spell auras" {
			t.Fatalf("stale limitation %q", l)
		}
	}
}

// spell.EffectValue sanity for passive base-point scaling.
func TestPassiveEffectValueLevelScaled(t *testing.T) {
	info := gamedata.SpellInfo{ID: 9500, BasePoints: [3]int32{10}}
	if v := spell.EffectValue(&info, 0, 10, func(lo, hi int32) int32 { return lo }); v != 10 {
		t.Fatalf("base points %d", v)
	}
}
