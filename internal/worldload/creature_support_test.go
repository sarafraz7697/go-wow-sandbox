package worldload

import (
	"context"
	"database/sql/driver"
	"testing"

	"wowserver/internal/entity"
)

func TestCreatureSupportLoaders(t *testing.T) {
	ResetForTest()
	db, srv := newFakeDB(t)

	srv.put("SELECT COUNT(*) FROM `creature_model_info`", rows1(1))
	srv.put("SELECT `modelid`,`bounding_radius`,`combat_reach`,`gender`,`modelid_other_gender`,`modelid_alternative` FROM `creature_model_info` ORDER BY `modelid`",
		&fakeRows{cols: []string{"modelid", "bounding_radius", "combat_reach", "gender", "modelid_other_gender", "modelid_alternative"},
			data: [][]driver.Value{{int64(398), 0.35, 1.2, int64(0), int64(0), int64(0)}}})

	srv.put("SELECT (SELECT COUNT(*) FROM `creature_equip_template`) + (SELECT COUNT(*) FROM `creature_equip_template_raw`)", rows1(2))
	srv.put("SELECT `entry`,`equipentry1`,`equipentry2`,`equipentry3` FROM `creature_equip_template` ORDER BY `entry`",
		&fakeRows{cols: []string{"entry", "equipentry1", "equipentry2", "equipentry3"},
			data: [][]driver.Value{{int64(3), int64(1906), int64(0), int64(1907)}}})
	srv.put("SELECT `entry`,`equipmodel1`,`equipmodel2`,`equipmodel3`,`equipinfo1`,`equipinfo2`,`equipinfo3`,`equipslot1`,`equipslot2`,`equipslot3` FROM `creature_equip_template_raw` ORDER BY `entry`",
		&fakeRows{cols: []string{"entry", "equipmodel1", "equipmodel2", "equipmodel3", "equipinfo1", "equipinfo2", "equipinfo3", "equipslot1", "equipslot2", "equipslot3"},
			data: [][]driver.Value{{int64(7), int64(11), int64(12), int64(13), int64(21), int64(22), int64(23), int64(31), int64(32), int64(33)}}})

	srv.put("SELECT (SELECT COUNT(*) FROM `creature_template_addon`) + (SELECT COUNT(*) FROM `creature_addon`)", rows1(2))
	srv.put("SELECT `entry`,`mount`,`bytes1`,`b2_0_sheath`,`b2_1_flags`,`emote`,`moveflags`,`auras` FROM `creature_template_addon` ORDER BY `entry`",
		&fakeRows{cols: []string{"entry", "mount", "bytes1", "b2_0_sheath", "b2_1_flags", "emote", "moveflags", "auras"},
			data: [][]driver.Value{{int64(6), int64(0), int64(0), int64(1), int64(0), int64(0), int64(0), "2334 3412"}}})
	srv.put("SELECT `guid`,`mount`,`bytes1`,`b2_0_sheath`,`b2_1_flags`,`emote`,`moveflags`,`auras` FROM `creature_addon` ORDER BY `guid`",
		&fakeRows{cols: []string{"guid", "mount", "bytes1", "b2_0_sheath", "b2_1_flags", "emote", "moveflags", "auras"},
			data: [][]driver.Value{{int64(54321), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), nil}}})

	srv.put("SELECT COUNT(*) FROM `creature_template_classlevelstats`", rows1(1))
	srv.put("SELECT `Class`,`Level`,`BaseMana`,`BaseMeleeAttackPower`,`BaseRangedAttackPower`,`BaseArmor`,`BaseHealthExp0`,`BaseDamageExp0`,`BaseHealthExp1`,`BaseDamageExp1` FROM `creature_template_classlevelstats` ORDER BY `Class`,`Level`",
		&fakeRows{cols: []string{"Class", "Level", "BaseMana", "BaseMeleeAttackPower", "BaseRangedAttackPower", "BaseArmor", "BaseHealthExp0", "BaseDamageExp0", "BaseHealthExp1", "BaseDamageExp1"},
			data: [][]driver.Value{{int64(1), int64(5), int64(100), 4.5, 1.5, int64(40), int64(100), 2.0, int64(150), 3.0}}})

	srv.put("SELECT COUNT(*) FROM `creature_template_spells`", rows1(1))
	srv.put("SELECT `entry`,`spell1`,`spell2`,`spell3`,`spell4` FROM `creature_template_spells` ORDER BY `entry`",
		&fakeRows{cols: []string{"entry", "spell1", "spell2", "spell3", "spell4"},
			data: [][]driver.Value{{int64(6), int64(1), int64(2), int64(3), int64(4)}}})

	srv.put("SELECT COUNT(*) FROM `creature_model_race`", rows1(1))
	srv.put("SELECT `modelid`,`racemask`,`creature_entry`,`modelid_racial` FROM `creature_model_race`",
		&fakeRows{cols: []string{"modelid", "racemask", "creature_entry", "modelid_racial"},
			data: [][]driver.Value{{int64(398), int64(690), int64(1), int64(399)}}})

	// Register only the support loaders under test.
	Register(Loader{Name: "creature_model_info", Order: OrderCreatureModelInfo, CountQuery: "SELECT COUNT(*) FROM `creature_model_info`", Load: loadCreatureModelInfos})
	Register(Loader{Name: "creature_equip_template", Order: OrderCreatureEquipTemplate, CountQuery: "SELECT (SELECT COUNT(*) FROM `creature_equip_template`) + (SELECT COUNT(*) FROM `creature_equip_template_raw`)", Load: loadEquipmentTemplates})
	Register(Loader{Name: "creature_addons", Order: OrderCreatureAddons, CountQuery: "SELECT (SELECT COUNT(*) FROM `creature_template_addon`) + (SELECT COUNT(*) FROM `creature_addon`)", Load: loadCreatureAddons})
	Register(Loader{Name: "creature_template_classlevelstats", Order: OrderCreatureClassLvlStats, CountQuery: "SELECT COUNT(*) FROM `creature_template_classlevelstats`", Load: loadCreatureClassLvlStats})
	Register(Loader{Name: "creature_template_spells", Order: OrderCreatureTemplateSpells, CountQuery: "SELECT COUNT(*) FROM `creature_template_spells`", Load: loadCreatureTemplateSpells})
	Register(Loader{Name: "creature_model_race", Order: OrderCreatureModelRace, CountQuery: "SELECT COUNT(*) FROM `creature_model_race`", Load: loadCreatureModelRace})

	dest := newDatasetForTest()
	if _, err := Run(context.Background(), db, dest, RunOptions{}); err != nil {
		t.Fatalf("run: %v", err)
	}

	if mi := dest.CreatureModelInfos[398]; mi.BoundingRadius != 0.35 || mi.CombatReach != 1.2 {
		t.Fatalf("model info wrong: %+v", mi)
	}
	if eq := dest.EquipmentTemplates[3]; eq.EquipEntry[0] != 1906 || eq.EquipEntry[2] != 1907 {
		t.Fatalf("equipment wrong: %+v", eq)
	}
	if eq := dest.EquipmentTemplatesRaw[7]; eq.EquipModel[2] != 13 || eq.EquipSlot[1] != 32 {
		t.Fatalf("equipment raw wrong: %+v", eq)
	}
	addon := dest.CreatureTemplateAddons[6]
	if len(addon.Auras) != 2 || addon.Auras[0] != 2334 || addon.Auras[1] != 3412 {
		t.Fatalf("template addon auras wrong: %+v", addon.Auras)
	}
	if guidAddon := dest.CreatureAddons[54321]; guidAddon.Auras != nil {
		t.Fatalf("guid addon NULL auras must be empty, got %+v", guidAddon.Auras)
	}
	cls := dest.CreatureClassLvlStats[entity.CreatureClassLevelKey{Class: 1, Level: 5, Expansion: 0}]
	if cls.BaseHealth != 100 || cls.BaseDamage != 2.0 {
		t.Fatalf("classlevelstats exp0 wrong: %+v", cls)
	}
	cls1 := dest.CreatureClassLvlStats[entity.CreatureClassLevelKey{Class: 1, Level: 5, Expansion: 1}]
	if cls1.BaseHealth != 150 || cls1.BaseDamage != 3.0 || cls1.BaseMana != 100 {
		t.Fatalf("classlevelstats exp1 wrong: %+v", cls1)
	}
	if ts := dest.CreatureTemplateSpells[6]; ts.Spells[3] != 4 {
		t.Fatalf("template spells wrong: %+v", ts)
	}
	if mr := dest.CreatureModelRaces[0]; mr.RaceMask != 690 || mr.ModelIDRacial != 399 {
		t.Fatalf("model race wrong: %+v", mr)
	}
}
