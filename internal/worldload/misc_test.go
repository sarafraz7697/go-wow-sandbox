package worldload

import (
	"context"
	"database/sql/driver"
	"testing"

	"wowserver/internal/entity"
)

func TestMiscAndPlayerInfoLoaders(t *testing.T) {
	ResetForTest()
	db, srv := newFakeDB(t)

	// mangos_string
	srv.put("SELECT COUNT(*) FROM `mangos_string`", rows1(1))
	srv.put("SELECT `entry`,`content_default`,`content_loc1`,`content_loc2`,`content_loc3`,`content_loc4`,`content_loc5`,`content_loc6`,`content_loc7`,`content_loc8` FROM `mangos_string` ORDER BY `entry`",
		&fakeRows{cols: []string{"entry", "d", "l1", "l2", "l3", "l4", "l5", "l6", "l7", "l8"},
			data: [][]driver.Value{{int64(1), "You are not mounted.", nil, nil, nil, nil, nil, nil, nil, nil}}})

	// spell_template
	srv.put("SELECT COUNT(*) FROM `spell_template`", rows1(1))
	srv.put("SELECT `id`,`proc_flags`,`proc_chance`,`duration_index`,`effect0`,`effect0_implicit_target_a`,`effect0_radius_idx`,`effect0_apply_aura_name`,`effect0_misc_value`,`effect0_misc_value_b`,`effect0_trigger_spell` FROM `spell_template` ORDER BY `id`",
		&fakeRows{cols: []string{"id", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
			data: [][]driver.Value{{int64(1), int64(0), int64(101), int64(21), int64(2), int64(6), int64(0), int64(0), int64(0), int64(0), int64(0)}}})

	// instance_template
	srv.put("SELECT COUNT(*) FROM `instance_template`", rows1(1))
	srv.put("SELECT `map`,`parent`,`levelMin`,`levelMax`,`maxPlayers`,`reset_delay` FROM `instance_template` ORDER BY `map`",
		&fakeRows{cols: []string{"map", "parent", "min", "max", "players", "reset"},
			data: [][]driver.Value{{int64(33), int64(0), int64(18), int64(26), int64(5), int64(7200)}}})

	// areatrigger_teleport
	srv.put("SELECT COUNT(*) FROM `areatrigger_teleport`", rows1(1))
	srv.put("SELECT `id`,`name`,`required_level`,`required_item`,`required_item2`,`heroic_key`,`heroic_key2`,`required_quest_done`,`required_quest_done_heroic`,`target_map`,`target_position_x`,`target_position_y`,`target_position_z`,`target_orientation` FROM `areatrigger_teleport` ORDER BY `id`",
		&fakeRows{cols: []string{"id", "name", "lvl", "i1", "i2", "k1", "k2", "q1", "q2", "map", "x", "y", "z", "o"},
			data: [][]driver.Value{{int64(45), "Deadmines Entrance", int64(10), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(36), -11347.0, -298.0, 10.0, 3.14}}})

	// areatrigger_tavern
	srv.put("SELECT COUNT(*) FROM `areatrigger_tavern`", rows1(1))
	srv.put("SELECT `id`,`name` FROM `areatrigger_tavern` ORDER BY `id`",
		&fakeRows{cols: []string{"id", "name"}, data: [][]driver.Value{{int64(98), "Lion's Pride Inn"}}})

	// game_graveyard_zone
	srv.put("SELECT COUNT(*) FROM `game_graveyard_zone`", rows1(2))
	srv.put("SELECT `id`,`ghost_zone`,`faction` FROM `game_graveyard_zone`",
		&fakeRows{cols: []string{"id", "zone", "faction"},
			data: [][]driver.Value{
				{int64(3), int64(12), int64(0)},
				{int64(4), int64(14), int64(9)}, // invalid faction -> skipped
			}})

	// exploration_basexp
	srv.put("SELECT COUNT(*) FROM `exploration_basexp`", rows1(1))
	srv.put("SELECT `level`,`basexp` FROM `exploration_basexp` ORDER BY `level`",
		&fakeRows{cols: []string{"level", "basexp"}, data: [][]driver.Value{{int64(1), int64(5)}}})

	// pet_name_generation
	srv.put("SELECT COUNT(*) FROM `pet_name_generation`", rows1(1))
	srv.put("SELECT `id`,`word`,`entry`,`half` FROM `pet_name_generation` ORDER BY `id`",
		&fakeRows{cols: []string{"id", "word", "entry", "half"}, data: [][]driver.Value{{int64(1), "fluffy", int64(0), int64(0)}}})

	// game_event
	srv.put("SELECT COUNT(*) FROM `game_event`", rows1(1))
	srv.put("SELECT `entry`,`start_time`,`end_time`,`occurence`,`length`,`holiday`,`description` FROM `game_event` ORDER BY `entry`",
		&fakeRows{cols: []string{"entry", "start", "end", "occ", "len", "holiday", "desc"},
			data: [][]driver.Value{{int64(1), "2008-01-01 00:00:00", "2020-12-31 23:59:59", int64(525600), int64(720), int64(0), "Darkmoon Faire"}}})

	// conditions
	srv.put("SELECT COUNT(*) FROM `conditions`", rows1(1))
	srv.put("SELECT `condition_entry`,`type`,`value1`,`value2` FROM `conditions` ORDER BY `condition_entry`",
		&fakeRows{cols: []string{"c", "t", "v1", "v2"}, data: [][]driver.Value{{int64(1), int64(8), int64(5), int64(0)}}})

	// game_tele
	srv.put("SELECT COUNT(*) FROM `game_tele`", rows1(1))
	srv.put("SELECT `id`,`position_x`,`position_y`,`position_z`,`orientation`,`map`,`name` FROM `game_tele` ORDER BY `id`",
		&fakeRows{cols: []string{"id", "x", "y", "z", "o", "map", "name"},
			data: [][]driver.Value{{int64(1), -8913.0, -133.0, 80.0, 0.0, int64(0), "stormwind"}}})

	// points_of_interest
	srv.put("SELECT COUNT(*) FROM `points_of_interest`", rows1(1))
	srv.put("SELECT `entry`,`x`,`y`,`icon`,`flags`,`data`,`icon_name` FROM `points_of_interest` ORDER BY `entry`",
		&fakeRows{cols: []string{"entry", "x", "y", "icon", "flags", "data", "icon_name"},
			data: [][]driver.Value{{int64(1), -8833.0, 485.0, int64(6), int64(99), int64(0), "Gryphon Master"}}})

	// item_required_target
	srv.put("SELECT COUNT(*) FROM `item_required_target`", rows1(1))
	srv.put("SELECT `entry`,`type`,`targetEntry` FROM `item_required_target`",
		&fakeRows{cols: []string{"entry", "type", "target"}, data: [][]driver.Value{{int64(6948), int64(1), int64(6)}}})

	// reputation_reward_rate
	srv.put("SELECT COUNT(*) FROM `reputation_reward_rate`", rows1(1))
	srv.put("SELECT `faction`,`quest_rate`,`creature_rate`,`spell_rate` FROM `reputation_reward_rate`",
		&fakeRows{cols: []string{"faction", "q", "c", "s"}, data: [][]driver.Value{{int64(1), 1.0, 1.0, 1.0}}})

	// reputation_spillover_template
	srv.put("SELECT COUNT(*) FROM `reputation_spillover_template`", rows1(1))
	srv.put("SELECT `faction`,`faction1`,`rate_1`,`rank_1`,`faction2`,`rate_2`,`rank_2`,`faction3`,`rate_3`,`rank_3`,`faction4`,`rate_4`,`rank_4` FROM `reputation_spillover_template`",
		&fakeRows{cols: []string{"f", "f1", "r1", "k1", "f2", "r2", "k2", "f3", "r3", "k3", "f4", "r4", "k4"},
			data: [][]driver.Value{{int64(7), int64(7), 0.5, int64(4), int64(0), 0.0, int64(0), int64(0), 0.0, int64(0), int64(0), 0.0, int64(0)}}})

	// mail_level_reward
	srv.put("SELECT COUNT(*) FROM `mail_level_reward`", rows1(1))
	srv.put("SELECT `level`,`raceMask`,`mailTemplateId`,`senderEntry` FROM `mail_level_reward`",
		&fakeRows{cols: []string{"level", "race", "mail", "sender"}, data: [][]driver.Value{{int64(20), int64(1), int64(1), int64(358)}}})

	// skill_fishing_base_level
	srv.put("SELECT COUNT(*) FROM `skill_fishing_base_level`", rows1(1))
	srv.put("SELECT `entry`,`skill` FROM `skill_fishing_base_level`",
		&fakeRows{cols: []string{"entry", "skill"}, data: [][]driver.Value{{int64(12), int64(55)}}})

	// petcreateinfo_spell
	srv.put("SELECT COUNT(*) FROM `petcreateinfo_spell`", rows1(1))
	srv.put("SELECT `entry`,`Spell1`,`Spell2`,`Spell3`,`Spell4` FROM `petcreateinfo_spell` ORDER BY `entry`",
		&fakeRows{cols: []string{"entry", "s1", "s2", "s3", "s4"}, data: [][]driver.Value{{int64(416), int64(16827), int64(16828), int64(16829), int64(16830)}}})

	// pet_levelstats
	srv.put("SELECT COUNT(*) FROM `pet_levelstats`", rows1(1))
	srv.put("SELECT `creature_entry`,`level`,`hp`,`mana`,`armor`,`str`,`agi`,`sta`,`inte`,`spi` FROM `pet_levelstats` ORDER BY `creature_entry`,`level`",
		&fakeRows{cols: []string{"e", "l", "hp", "mana", "armor", "str", "agi", "sta", "inte", "spi"},
			data: [][]driver.Value{{int64(416), int64(1), int64(42), int64(0), int64(15), int64(4), int64(6), int64(5), int64(2), int64(2)}}})

	// playercreateinfo family
	srv.put("SELECT COUNT(*) FROM `playercreateinfo`", rows1(1))
	srv.put("SELECT `race`,`class`,`map`,`zone`,`position_x`,`position_y`,`position_z`,`orientation` FROM `playercreateinfo` ORDER BY `race`,`class`",
		&fakeRows{cols: []string{"race", "class", "map", "zone", "x", "y", "z", "o"},
			data: [][]driver.Value{{int64(1), int64(1), int64(0), int64(12), -8949.0, -132.0, 84.0, 0.0}}})
	srv.put("SELECT COUNT(*) FROM `playercreateinfo_item`", rows1(1))
	srv.put("SELECT `race`,`class`,`itemid`,`amount` FROM `playercreateinfo_item` ORDER BY `race`,`class`,`itemid`",
		&fakeRows{cols: []string{"race", "class", "itemid", "amount"}, data: [][]driver.Value{{int64(1), int64(1), int64(25), int64(1)}}})
	srv.put("SELECT COUNT(*) FROM `playercreateinfo_spell`", rows1(1))
	srv.put("SELECT `race`,`class`,`Spell` FROM `playercreateinfo_spell` ORDER BY `race`,`class`,`Spell`",
		&fakeRows{cols: []string{"race", "class", "Spell"}, data: [][]driver.Value{{int64(1), int64(1), int64(78)}}})
	srv.put("SELECT COUNT(*) FROM `playercreateinfo_action`", rows1(1))
	srv.put("SELECT `race`,`class`,`button`,`action`,`type` FROM `playercreateinfo_action` ORDER BY `race`,`class`,`button`",
		&fakeRows{cols: []string{"race", "class", "button", "action", "type"}, data: [][]driver.Value{{int64(1), int64(1), int64(0), int64(6603), int64(0)}}})
	srv.put("SELECT COUNT(*) FROM `player_levelstats`", rows1(1))
	srv.put("SELECT `race`,`class`,`level`,`str`,`agi`,`sta`,`inte`,`spi` FROM `player_levelstats` ORDER BY `race`,`class`,`level`",
		&fakeRows{cols: []string{"race", "class", "level", "str", "agi", "sta", "inte", "spi"},
			data: [][]driver.Value{{int64(1), int64(1), int64(1), int64(20), int64(23), int64(22), int64(20), int64(20)}}})
	srv.put("SELECT COUNT(*) FROM `player_classlevelstats`", rows1(1))
	srv.put("SELECT `class`,`level`,`basehp`,`basemana` FROM `player_classlevelstats` ORDER BY `class`,`level`",
		&fakeRows{cols: []string{"class", "level", "hp", "mana"}, data: [][]driver.Value{{int64(1), int64(1), int64(60), int64(0)}}})
	srv.put("SELECT COUNT(*) FROM `player_xp_for_level`", rows1(1))
	srv.put("SELECT `lvl`,`xp_for_next_level` FROM `player_xp_for_level` ORDER BY `lvl`",
		&fakeRows{cols: []string{"lvl", "xp"}, data: [][]driver.Value{{int64(1), int64(400)}}})
	srv.put("SELECT COUNT(*) FROM `reserved_name`", rows1(1))
	srv.put("SELECT `name` FROM `reserved_name` ORDER BY `name`",
		&fakeRows{cols: []string{"name"}, data: [][]driver.Value{{"Khadgar"}}})

	registerSubset(
		"mangos_string", "spell_template", "instance_template", "areatrigger_teleport", "areatrigger_tavern",
		"game_graveyard_zone", "exploration_basexp", "pet_name_generation", "game_event", "conditions",
		"game_tele", "points_of_interest", "item_required_target", "reputation_reward_rate",
		"reputation_spillover_template", "mail_level_reward", "skill_fishing_base_level",
		"petcreateinfo_spell", "pet_levelstats",
		"playercreateinfo", "playercreateinfo_item", "playercreateinfo_spell", "playercreateinfo_action",
		"player_levelstats", "player_classlevelstats", "player_xp_for_level", "reserved_name",
	)

	dest := newDatasetForTest()
	if _, err := Run(context.Background(), db, dest, RunOptions{}); err != nil {
		t.Fatalf("run: %v", err)
	}

	if s := dest.MangosStrings[1]; s.ContentDefault != "You are not mounted." || s.ContentLoc[0] != "" {
		t.Fatalf("mangos_string wrong: %+v", s)
	}
	if st := dest.SpellTemplates[1]; st.ProcChance != 101 || st.Effect0 != 2 {
		t.Fatalf("spell_template wrong: %+v", st)
	}
	if it := dest.InstanceTemplates[33]; it.MaxPlayers != 5 || it.ResetDelay != 7200 {
		t.Fatalf("instance_template wrong: %+v", it)
	}
	if at := dest.AreaTriggerTeleports[45]; at.TargetMapID != 36 || at.TargetX != -11347.0 || at.Name != "Deadmines Entrance" {
		t.Fatalf("areatrigger_teleport wrong: %+v", at)
	}
	if tt := dest.TavernTriggers[98]; tt.Name != "Lion's Pride Inn" {
		t.Fatalf("areatrigger_tavern wrong: %+v", tt)
	}
	if len(dest.GraveyardLinks) != 1 || dest.GraveyardLinks[0].ID != 3 {
		t.Fatalf("graveyard links wrong: %+v", dest.GraveyardLinks)
	}
	if dest.ExplorationBaseXP[1] != 5 {
		t.Fatal("exploration_basexp wrong")
	}
	if pn := dest.PetNames[0]; pn.Word != "fluffy" {
		t.Fatalf("pet names wrong: %+v", pn)
	}
	if ge := dest.GameEvents[1]; ge.Occurence != 525600 || ge.Description != "Darkmoon Faire" {
		t.Fatalf("game_event wrong: %+v", ge)
	}
	if c := dest.Conditions[1]; c.Type != 8 || c.Value1 != 5 {
		t.Fatalf("conditions wrong: %+v", c)
	}
	if gt := dest.GameTeles[1]; gt.Name != "stormwind" || gt.PositionX != -8913.0 {
		t.Fatalf("game_tele wrong: %+v", gt)
	}
	if poi := dest.PointsOfInterest[1]; poi.IconName != "Gryphon Master" || poi.X != -8833.0 {
		t.Fatalf("points_of_interest wrong: %+v", poi)
	}
	if irt := dest.ItemRequiredTargets[0]; irt.Entry != 6948 || irt.TargetEntry != 6 {
		t.Fatalf("item_required_target wrong: %+v", irt)
	}
	if rr := dest.ReputationRewardRates[1]; rr.QuestRate != 1.0 {
		t.Fatalf("reputation_reward_rate wrong: %+v", rr)
	}
	if rs := dest.ReputationSpilloverTemplates[7]; rs.Rate[0] != 0.5 || rs.Rank[0] != 4 {
		t.Fatalf("reputation_spillover_template wrong: %+v", rs)
	}
	if ml := dest.MailLevelRewards[0]; ml.SenderEntry != 358 {
		t.Fatalf("mail_level_reward wrong: %+v", ml)
	}
	if dest.FishingBaseSkillLevels[12] != 55 {
		t.Fatal("skill_fishing_base_level wrong")
	}
	if pc := dest.PetCreateSpells[416]; pc.Spells[0] != 16827 {
		t.Fatalf("petcreateinfo_spell wrong: %+v", pc)
	}
	if pls := dest.PetLevelStats[petKey(416, 1)]; pls.HP != 42 || pls.Agi != 6 {
		t.Fatalf("pet_levelstats wrong: %+v", pls)
	}
	if pci := dest.PlayerCreateInfos[playerKey(1, 1)]; pci.Map != 0 || pci.PositionX != -8949.0 {
		t.Fatalf("playercreateinfo wrong: %+v", pci)
	}
	if len(dest.PlayerCreateInfoItems) != 1 || dest.PlayerCreateInfoItems[0].ItemID != 25 {
		t.Fatalf("playercreateinfo_item wrong: %+v", dest.PlayerCreateInfoItems)
	}
	if len(dest.PlayerCreateInfoSpells) != 1 || dest.PlayerCreateInfoSpells[0].Spell != 78 {
		t.Fatalf("playercreateinfo_spell wrong: %+v", dest.PlayerCreateInfoSpells)
	}
	if len(dest.PlayerCreateInfoActions) != 1 || dest.PlayerCreateInfoActions[0].Action != 6603 {
		t.Fatalf("playercreateinfo_action wrong: %+v", dest.PlayerCreateInfoActions)
	}
	if ls := dest.PlayerLevelStats[levelKey(1, 1, 1)]; ls.Str != 20 || ls.Spi != 20 {
		t.Fatalf("player_levelstats wrong: %+v", ls)
	}
	if cls := dest.PlayerClassLevelStats[classKey(1, 1)]; cls.BaseHP != 60 {
		t.Fatalf("player_classlevelstats wrong: %+v", cls)
	}
	if dest.PlayerXPForLevel[1] != 400 {
		t.Fatal("player_xp_for_level wrong")
	}
	if _, ok := dest.ReservedNames["khadgar"]; !ok {
		t.Fatal("reserved_name wrong (must be lowercased)")
	}
}

func petKey(entry, level uint32) entity.PetLevelStatsKey {
	return entity.PetLevelStatsKey{Entry: entry, Level: level}
}
func playerKey(race, class uint32) entity.RaceClassKey {
	return entity.RaceClassKey{Race: race, Class: class}
}
func levelKey(race, class, level uint32) entity.RaceClassLevelKey {
	return entity.RaceClassLevelKey{Race: race, Class: class, Level: level}
}
func classKey(class, level uint32) entity.ClassLevelKey {
	return entity.ClassLevelKey{Class: class, Level: level}
}
