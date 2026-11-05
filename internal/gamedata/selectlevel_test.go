package gamedata

import (
	"testing"

	"wowserver/internal/entity"
)

func spawnFixtureCatalog(t *testing.T) *Catalog {
	t.Helper()
	client, db := creationFixture()
	// A level 5..7 creature, class 1, expansion 0, with classlevelstats
	// for levels 5..7 and a model_info row for its display.
	db.CreatureTemplates[6] = entity.CreatureInfo{
		Entry: 6, Name: "Defias Thug", MinLevel: 5, MaxLevel: 7,
		ModelId:         [4]uint32{398, 0, 0, 0},
		FactionAlliance: 11, FactionHorde: 11, Scale: 1,
		UnitClass: 1, Expansion: 0,
		HealthMultiplier: 1, PowerMultiplier: 1,
		SpeedWalk: 1, SpeedRun: 1.14286,
		MinLevelHealth: 100, MaxLevelHealth: 140,
		MinLevelMana: 40, MaxLevelMana: 56,
	}
	for level := uint32(5); level <= 7; level++ {
		db.CreatureClassLvlStats[entity.CreatureClassLevelKey{Class: 1, Level: level, Expansion: 0}] =
			entity.CreatureClassLvlStats{Class: 1, Level: level, Expansion: 0, BaseHealth: 30 * level, BaseMana: 10 * level, BaseDamage: 1.5}
	}
	db.CreatureModelInfos[398] = entity.CreatureModelInfo{ModelID: 398, BoundingRadius: 0.35, CombatReach: 1.2}
	db.Creatures[101] = entity.CreatureData{ID: 6, MapID: 0, PosX: 1, PosY: 2, PosZ: 3, Orientation: 4, CurHealth: 0, CurMana: 0}
	db.CreatureOrder = []uint32{101}
	db.GameObjectTemplates[31] = entity.GameObjectInfo{ID: 31, Type: 3, DisplayID: 10, Name: "Barrel", Size: 2}
	db.GameObjects[55] = entity.GameObjectData{ID: 31, MapID: 0, PosX: 5, PosY: 6, PosZ: 7, Orientation: 0.5, Rotation3: 1, State: 1, AnimProgress: 42}
	db.GameObjectOrder = []uint32{55}
	c, err := New(client, db, 2)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestResolveCreatureSpawnUsesClassLevelStats(t *testing.T) {
	c := spawnFixtureCatalog(t)
	// GUID 101 -> deterministic level 5+(101%3)=5+2=7.
	res, ok := c.ResolveCreatureSpawn(CreatureSpawnRef{GUID: 101, Data: entity.CreatureData{ID: 6}})
	if !ok {
		t.Fatal("spawn not resolved")
	}
	if res.Level != 7 {
		t.Fatalf("level = %d, want 7 (guid 101 mod 3 range)", res.Level)
	}
	// classlevelstats BaseHealth for level 7 = 210, multiplier 1.
	if res.MaxHealth != 210 {
		t.Fatalf("max health = %d, want 210", res.MaxHealth)
	}
	if res.MaxMana != 70 {
		t.Fatalf("max mana = %d, want 70", res.MaxMana)
	}
	// curhealth 0 clamps up to full health like the spawn loader allows.
	if res.Health != 210 || res.Mana != 0 {
		t.Fatalf("current health/mana = %d/%d", res.Health, res.Mana)
	}
	if res.DisplayID != 398 || res.BoundingRadius != 0.35 || res.CombatReach != 1.2 {
		t.Fatalf("model fields wrong: %+v", res)
	}
	if res.Scale != 1 {
		t.Fatalf("scale = %f", res.Scale)
	}
	// Walk 2.5*1, run 7*1.14286 = 8.00002.
	if res.SpeedWalk != 2.5 || res.SpeedRun < 7.9999 || res.SpeedRun > 8.0001 {
		t.Fatalf("speeds wrong: %f/%f", res.SpeedWalk, res.SpeedRun)
	}
}

func TestResolveCreatureSpawnLegacyFallbackAndOverride(t *testing.T) {
	c := spawnFixtureCatalog(t)
	// Remove the classlevelstats row for the level the spawn resolves to
	// (guid 101 -> level 7): legacy interpolation between MinLevelHealth
	// 100..140 at rellevel (7-5)/(7-5)=1 -> 140.
	c.classLevelStats[entity.CreatureClassLevelKey{Class: 1, Level: 7, Expansion: 0}] = entity.CreatureClassLvlStats{}
	delete(c.classLevelStats, entity.CreatureClassLevelKey{Class: 1, Level: 7, Expansion: 0})
	res, ok := c.ResolveCreatureSpawn(CreatureSpawnRef{GUID: 101, Data: entity.CreatureData{ID: 6, ModelIDOverride: 999, CurHealth: 999, CurMana: 999}})
	if !ok {
		t.Fatal("spawn not resolved")
	}
	if res.MaxHealth != 140 || res.MaxMana != 56 {
		t.Fatalf("legacy health/mana = %d/%d, want 140/56", res.MaxHealth, res.MaxMana)
	}
	// modelid_override wins; no model_info for 999 -> default bounding/reach.
	if res.DisplayID != 999 {
		t.Fatalf("override display = %d, want 999", res.DisplayID)
	}
	if res.BoundingRadius != 0.5 || res.CombatReach != 1.5 {
		t.Fatalf("default model metrics wrong: %+v", res)
	}
	// curhealth above max clamps down; curmana clamps to max.
	if res.Health != 140 || res.Mana != 56 {
		t.Fatalf("clamped current values = %d/%d", res.Health, res.Mana)
	}
}

func TestSpawnQueriesAndTemplates(t *testing.T) {
	c := spawnFixtureCatalog(t)
	near := c.CreatureSpawnsNear(0, 1, 2, 3, 10)
	if len(near) != 1 || near[0].GUID != 101 || near[0].Data.ID != 6 {
		t.Fatalf("creature spawns near wrong: %+v", near)
	}
	if far := c.CreatureSpawnsNear(0, 100, 100, 100, 10); len(far) != 0 {
		t.Fatalf("creature spawns near should be empty, got %+v", far)
	}
	gos := c.GameObjectSpawnsNear(0, 5, 6, 7, 10)
	if len(gos) != 1 || gos[0].GUID != 55 {
		t.Fatalf("go spawns near wrong: %+v", gos)
	}
	info, ok := c.ResolveGameObjectSpawn(gos[0])
	if !ok || info.ID != 31 || info.Size != 2 {
		t.Fatalf("go resolve wrong: %+v", info)
	}
	if _, ok := c.ResolveCreatureSpawn(CreatureSpawnRef{GUID: 1, Data: entity.CreatureData{ID: 4242}}); ok {
		t.Fatal("unknown entry must not resolve")
	}
	ct, ok := c.CreatureTemplate(6)
	if !ok || ct.Name != "Defias Thug" || ct.ModelId[0] != 398 {
		t.Fatalf("creature template lookup wrong: %+v", ct)
	}
	gt, ok := c.GameObjectTemplate(31)
	if !ok || gt.Type != 3 {
		t.Fatalf("go template lookup wrong: %+v", gt)
	}
	if _, ok := c.Creature(6); !ok {
		t.Fatal("creature family view missing")
	}
}
