package worldload

import (
	"context"
	"database/sql/driver"
	"testing"

	"wowserver/internal/entity"
)

func TestSpawnLoaders(t *testing.T) {
	ResetForTest()
	db, srv := newFakeDB(t)

	// creature_template must exist first so spawn validation can look
	// entries up.
	srv.put("SELECT COUNT(*) FROM `creature_template`", rows1(1))
	ctQuery := "SELECT `Entry`,`Name`,`SubName`,`IconName`,`MinLevel`,`MaxLevel`,`HeroicEntry`,`ModelId1`,`ModelId2`,`ModelId3`,`ModelId4`,`FactionAlliance`,`FactionHorde`,`Scale`,`Family`,`CreatureType`,`InhabitType`,`RegenerateStats`,`RacialLeader`,`NpcFlags`,`UnitFlags`,`DynamicFlags`,`ExtraFlags`,`CreatureTypeFlags`,`SpeedWalk`,`SpeedRun`,`UnitClass`,`Rank`,`Expansion`,`HealthMultiplier`,`PowerMultiplier`,`DamageMultiplier`,`DamageVariance`,`ArmorMultiplier`,`ExperienceMultiplier`,`MinLevelHealth`,`MaxLevelHealth`,`MinLevelMana`,`MaxLevelMana`,`MinMeleeDmg`,`MaxMeleeDmg`,`MinRangedDmg`,`MaxRangedDmg`,`Armor`,`MeleeAttackPower`,`RangedAttackPower`,`MeleeBaseAttackTime`,`RangedBaseAttackTime`,`DamageSchool`,`MinLootGold`,`MaxLootGold`,`LootId`,`PickpocketLootId`,`SkinningLootId`,`KillCredit1`,`KillCredit2`,`MechanicImmuneMask`,`SchoolImmuneMask`,`ResistanceHoly`,`ResistanceFire`,`ResistanceNature`,`ResistanceFrost`,`ResistanceShadow`,`ResistanceArcane`,`PetSpellDataId`,`MovementType`,`TrainerType`,`TrainerSpell`,`TrainerClass`,`TrainerRace`,`TrainerTemplateId`,`VendorTemplateId`,`EquipmentTemplateId`,`GossipMenuId`,`AIName` FROM `creature_template` ORDER BY `Entry`"
	ctCols := []string{"Entry"}
	for i := 1; i < 75; i++ {
		ctCols = append(ctCols, "c")
	}
	ctRow := make([]driver.Value, 75)
	ctRow[0] = int64(6)
	ctRow[1] = "Defias Thug"
	ctRow[4], ctRow[5] = int64(5), int64(5)
	ctRow[35], ctRow[37] = int64(100), int64(50) // MinLevelHealth, MinLevelMana (indices 35/37 in schema order)
	srv.put(ctQuery, &fakeRows{cols: ctCols, data: [][]driver.Value{ctRow}})

	srv.put("SELECT COUNT(*) FROM `creature`", rows1(3))
	srv.put("SELECT `creature`.`guid`, `creature`.`id`, `map`, `modelid`,`equipment_id`, `position_x`, `position_y`, `position_z`, `orientation`, `spawntimesecs`, `spawndist`, `currentwaypoint`,`curhealth`, `curmana`, `DeathState`, `MovementType`, `spawnMask`, `event`,`pool_creature`.`pool_entry`, `pool_creature_template`.`pool_entry` FROM `creature` LEFT OUTER JOIN `game_event_creature` ON `creature`.`guid` = `game_event_creature`.`guid` LEFT OUTER JOIN `pool_creature` ON `creature`.`guid` = `pool_creature`.`guid` LEFT OUTER JOIN `pool_creature_template` ON `creature`.`id` = `pool_creature_template`.`id` ORDER BY `creature`.`guid`",
		&fakeRows{cols: []string{"guid", "id", "map", "modelid", "equipment_id", "position_x", "position_y", "position_z", "orientation", "spawntimesecs", "spawndist", "currentwaypoint", "curhealth", "curmana", "DeathState", "MovementType", "spawnMask", "event", "pool_entry", "pool_entry"},
			data: [][]driver.Value{
				// Valid spawn, member of game event 12 and pool 3.
				{int64(1), int64(6), int64(0), int64(0), int64(-1), -9445.0, 64.0, 55.0, 2.5, int64(300), 0.0, int64(0), int64(20), int64(0), int64(0), int64(0), int64(1), int64(12), int64(3), nil},
				// Missing template entry 999 -> skipped.
				{int64(2), int64(999), int64(0), int64(0), int64(0), 0.0, 0.0, 0.0, 0.0, int64(0), 0.0, int64(0), int64(0), int64(0), int64(0), int64(0), int64(1), nil, nil, nil},
				// Low mana gets clamped up to the template MinLevelMana=50.
				{int64(3), int64(6), int64(0), int64(0), int64(0), 1.0, 2.0, 3.0, 0.0, int64(0), 0.0, int64(0), int64(99), int64(10), int64(0), int64(0), int64(1), nil, nil, int64(4)},
			}})

	// gameobject_template must exist for gameobject spawn validation.
	srv.put("SELECT COUNT(*) FROM `gameobject_template`", rows1(1))
	goQuery := "SELECT `entry`,`type`,`displayId`,`name`,`IconName`,`castBarCaption`,`faction`,`flags`,`size`"
	for i := 0; i < entity.MaxGOData; i++ {
		goQuery += ",`data" + itoa(i) + "`"
	}
	goQuery += " FROM `gameobject_template` ORDER BY `entry`"
	goRow := []driver.Value{int64(31), int64(3), int64(10), "Broken Barrel", "", "", int64(0), int64(0), 1.0}
	for i := 0; i < entity.MaxGOData; i++ {
		goRow = append(goRow, int64(0))
	}
	goCols := []string{"entry"}
	for i := 1; i < 9+entity.MaxGOData; i++ {
		goCols = append(goCols, "c")
	}
	srv.put(goQuery, &fakeRows{cols: goCols, data: [][]driver.Value{goRow}})

	srv.put("SELECT COUNT(*) FROM `gameobject`", rows1(2))
	srv.put("SELECT `gameobject`.`guid`, `gameobject`.`id`, `gameobject`.`map`, `gameobject`.`position_x`, `gameobject`.`position_y`, `gameobject`.`position_z`, `gameobject`.`orientation`, `gameobject`.`rotation0`, `gameobject`.`rotation1`, `gameobject`.`rotation2`, `gameobject`.`rotation3`, `gameobject`.`spawntimesecs`, `gameobject`.`animprogress`, `gameobject`.`state`, `gameobject`.`spawnMask`,`game_event_gameobject`.`event`, `pool_gameobject`.`pool_entry`, `pool_gameobject_template`.`pool_entry` FROM `gameobject` LEFT OUTER JOIN `game_event_gameobject` ON `gameobject`.`guid` = `game_event_gameobject`.`guid` LEFT OUTER JOIN `pool_gameobject` ON `gameobject`.`guid` = `pool_gameobject`.`guid` LEFT OUTER JOIN `pool_gameobject_template` ON `gameobject`.`id` = `pool_gameobject_template`.`id` ORDER BY `gameobject`.`guid`",
		&fakeRows{cols: []string{"guid", "id", "map", "x", "y", "z", "o", "r0", "r1", "r2", "r3", "spawntimesecs", "animprogress", "state", "spawnMask", "event", "pool_entry", "pool_entry"},
			data: [][]driver.Value{
				{int64(10), int64(31), int64(0), -8833.0, 530.0, 95.0, 3.1, 0.0, 0.0, 0.0, 1.0, int64(180), int64(100), int64(1), int64(1), int64(5), nil, int64(9)},
				// Invalid state 7 -> skipped.
				{int64(11), int64(31), int64(0), 1.0, 1.0, 1.0, 0.0, 0.0, 0.0, 0.0, 1.0, int64(0), int64(100), int64(7), int64(1), nil, nil, nil},
			}})

	Register(Loader{Name: "creature_template", Order: OrderCreatureTemplate, CountQuery: "SELECT COUNT(*) FROM `creature_template`", Load: loadCreatureTemplates})
	Register(Loader{Name: "creature", Order: OrderCreatures, CountQuery: "SELECT COUNT(*) FROM `creature`", Load: loadCreatures})
	Register(Loader{Name: "gameobject_template", Order: OrderGameObjectTemplate, CountQuery: "SELECT COUNT(*) FROM `gameobject_template`", Load: loadGameObjectTemplates})
	Register(Loader{Name: "gameobject", Order: OrderGameObjects, CountQuery: "SELECT COUNT(*) FROM `gameobject`", Load: loadGameObjects})

	dest := newDatasetForTest()
	rep, err := Run(context.Background(), db, dest, RunOptions{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(dest.Creatures) != 2 {
		t.Fatalf("creatures = %d, want 2 (the entry-999 row is skipped)", len(dest.Creatures))
	}
	c1 := dest.Creatures[1]
	if c1.ID != 6 || c1.EquipmentID != -1 || c1.SpawnMask != 1 || c1.IsDead {
		t.Fatalf("creature 1 wrong: %+v", c1)
	}
	if c1.CurMana != 50 {
		t.Fatalf("creature 1 mana must clamp to MinLevelMana=50, got %d", c1.CurMana)
	}
	if dest.CreatureOrder[0] != 1 || dest.CreatureOrder[1] != 3 {
		t.Fatalf("creature order wrong: %v", dest.CreatureOrder)
	}
	if ev := dest.GameEventCreatureGUIDs[12]; len(ev) != 1 || ev[0] != 1 {
		t.Fatalf("game event guids wrong: %v", ev)
	}
	if dest.PoolCreature[1] != 3 {
		t.Fatalf("pool creature wrong: %v", dest.PoolCreature)
	}
	if dest.PoolCreatureTemplate[6] != 4 {
		t.Fatalf("pool creature template wrong: %v", dest.PoolCreatureTemplate)
	}
	c3 := dest.Creatures[3]
	if c3.CurMana != 50 {
		t.Fatalf("creature 3 mana must clamp to MinLevelMana=50, got %d", c3.CurMana)
	}

	if len(dest.GameObjects) != 1 {
		t.Fatalf("gameobjects = %d, want 1 (state 7 skipped)", len(dest.GameObjects))
	}
	g10 := dest.GameObjects[10]
	if g10.State != 1 || g10.Rotation3 != 1.0 || g10.SpawnTimeSecs != 180 {
		t.Fatalf("gameobject 10 wrong: %+v", g10)
	}
	if ev := dest.GameEventGameObjectGUIDs[5]; len(ev) != 1 || ev[0] != 10 {
		t.Fatalf("game event gameobject guids wrong: %v", ev)
	}
	if dest.PoolGameObjectTemplate[31] != 9 {
		t.Fatalf("pool gameobject template wrong: %v", dest.PoolGameObjectTemplate)
	}
	if rep.TotalSkipped() != 4 {
		t.Fatalf("total skipped = %d, want 4 (bad entry, 2 mana clamps, bad state)", rep.TotalSkipped())
	}
}
