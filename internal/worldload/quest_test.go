package worldload

import (
	"context"
	"database/sql/driver"
	"testing"
)

func TestQuestLoaders(t *testing.T) {
	ResetForTest()
	db, srv := newFakeDB(t)

	// quest_template: columns = 28 header + 6 texts + 4 objText + 4+4 items
	// + 4+4 source + 4+4 creatureGO + 4 spellcast + 6+6 choice + 4+4 items
	// + 5+5 rep + 7 money + 4 point + 4+4 emotes + 2 + 4+4 offerEmotes +
	// 2 scripts = 127.
	qCols := 127
	cols := make([]string, qCols)
	row := make([]driver.Value, qCols)
	for i := range cols {
		cols[i] = "c"
	}
	row[0] = int64(46) // entry
	row[2] = int64(12) // ZoneOrSort
	row[3] = int64(1)  // MinLevel
	row[18] = int64(8) // Flags = QUEST_FLAGS_SHARABLE
	row[19] = int64(1) // SpecialFlags = repeatable
	row[28] = "Paladin quest"
	row[33] = "Bring the letter."
	// ReqItemId1 at 38 (after 4 objective texts at 34..37).
	row[38] = int64(6948)
	row[42] = int64(1) // ReqItemCount1
	// ReqCreatureOrGOId1 at 54.
	row[54] = int64(6)
	row[58] = int64(3) // ReqCreatureOrGOCount1
	// RewChoiceItemId1 at 66.
	row[66] = int64(1906)
	row[72] = int64(1) // RewChoiceItemCount1
	// RewItemId1 at 78.
	row[78] = int64(6948)
	row[82] = int64(1) // RewItemCount1
	// RewRepFaction1 at 86.
	row[86] = int64(1)
	row[91] = int64(250)   // RewRepValue1
	row[97] = int64(-1000) // RewOrReqMoney (signed; 96=RewHonorableKills)
	row[104] = 1.5         // PointX
	row[107] = int64(1)    // DetailsEmote1

	// quest_relations: valid creature starter, unknown quest (skip), and
	// duplicate (counted once).
	srv.put("SELECT COUNT(*) FROM `quest_relations`", rows1(3))
	srv.put("SELECT `actor`,`entry`,`quest`,`role` FROM `quest_relations` ORDER BY `entry`,`quest`",
		&fakeRows{cols: []string{"actor", "entry", "quest", "role"},
			data: [][]driver.Value{
				{int64(0), int64(6), int64(46), int64(0)},
				{int64(0), int64(6), int64(46), int64(0)},   // duplicate, ignored
				{int64(0), int64(6), int64(9999), int64(0)}, // quest missing, skipped
			}})

	// creature_template row for entry 6 (needed by relation validation).
	srv.put("SELECT COUNT(*) FROM `creature_template`", rows1(1))
	ctQuery := "SELECT `Entry`,`Name`,`SubName`,`IconName`,`MinLevel`,`MaxLevel`,`HeroicEntry`,`ModelId1`,`ModelId2`,`ModelId3`,`ModelId4`,`FactionAlliance`,`FactionHorde`,`Scale`,`Family`,`CreatureType`,`InhabitType`,`RegenerateStats`,`RacialLeader`,`NpcFlags`,`UnitFlags`,`DynamicFlags`,`ExtraFlags`,`CreatureTypeFlags`,`SpeedWalk`,`SpeedRun`,`UnitClass`,`Rank`,`Expansion`,`HealthMultiplier`,`PowerMultiplier`,`DamageMultiplier`,`DamageVariance`,`ArmorMultiplier`,`ExperienceMultiplier`,`MinLevelHealth`,`MaxLevelHealth`,`MinLevelMana`,`MaxLevelMana`,`MinMeleeDmg`,`MaxMeleeDmg`,`MinRangedDmg`,`MaxRangedDmg`,`Armor`,`MeleeAttackPower`,`RangedAttackPower`,`MeleeBaseAttackTime`,`RangedBaseAttackTime`,`DamageSchool`,`MinLootGold`,`MaxLootGold`,`LootId`,`PickpocketLootId`,`SkinningLootId`,`KillCredit1`,`KillCredit2`,`MechanicImmuneMask`,`SchoolImmuneMask`,`ResistanceHoly`,`ResistanceFire`,`ResistanceNature`,`ResistanceFrost`,`ResistanceShadow`,`ResistanceArcane`,`PetSpellDataId`,`MovementType`,`TrainerType`,`TrainerSpell`,`TrainerClass`,`TrainerRace`,`TrainerTemplateId`,`VendorTemplateId`,`EquipmentTemplateId`,`GossipMenuId`,`AIName` FROM `creature_template` ORDER BY `Entry`"
	ctCols := make([]string, 75)
	for i := range ctCols {
		ctCols[i] = "c"
	}
	ctRow := make([]driver.Value, 75)
	ctRow[0] = int64(6)
	ctRow[1] = "Marshall Dughan"
	ctRow[4], ctRow[5] = int64(10), int64(10) // MinLevel/MaxLevel

	srv.put("SELECT COUNT(*) FROM `quest_template`", rows1(1))
	srv.put(questTemplateQuery(), &fakeRows{cols: cols, data: [][]driver.Value{row}})
	srv.put(ctQuery, &fakeRows{cols: ctCols, data: [][]driver.Value{ctRow}})

	Register(Loader{Name: "creature_template", Order: OrderCreatureTemplate, CountQuery: "SELECT COUNT(*) FROM `creature_template`", Load: loadCreatureTemplates})
	Register(Loader{Name: "quest_template", Order: OrderQuests, CountQuery: "SELECT COUNT(*) FROM `quest_template`", Load: loadQuests})
	Register(Loader{Name: "quest_relations", Order: OrderQuestRelations, CountQuery: "SELECT COUNT(*) FROM `quest_relations`", Load: loadQuestRelations})

	dest := newDatasetForTest()
	rep, err := Run(context.Background(), db, dest, RunOptions{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	quest, ok := dest.Quests[46]
	if !ok {
		t.Fatalf("quest 46 missing")
	}
	if quest.Title != "Paladin quest" || quest.ZoneOrSort != 12 || !quest.IsRepeatable() || quest.QuestFlags != 8 {
		t.Fatalf("quest fields wrong: %+v", quest)
	}
	if quest.ReqItemID[0] != 6948 || quest.ReqItemCount[0] != 1 || quest.ReqItemsCount != 1 {
		t.Fatalf("quest item objective wrong: %+v", quest.ReqItemID)
	}
	if quest.ReqCreatureOrGOID[0] != 6 || quest.ReqCreatureOrGOCount[0] != 3 || quest.ReqCreatureOrGOCount2 != 1 {
		t.Fatalf("quest creature objective wrong: %+v", quest.ReqCreatureOrGOID)
	}
	if quest.RewChoiceItemsCount != 1 || quest.RewItemsCount != 1 {
		t.Fatalf("quest reward counts wrong: %d/%d", quest.RewChoiceItemsCount, quest.RewItemsCount)
	}
	if quest.RewRepValue[0] != 250 || quest.RewOrReqMoney != -1000 || quest.PointX != 1.5 || quest.DetailsEmote[0] != 1 {
		t.Fatalf("quest tail wrong: %+v", quest)
	}
	if starters := dest.CreatureQuestStarters[6]; len(starters) != 1 || starters[0] != 46 {
		t.Fatalf("creature quest starters wrong: %v", starters)
	}
	if len(dest.CreatureQuestEnders[6]) != 0 {
		t.Fatalf("enders should be empty")
	}
	if rep.TotalSkipped() != 1 {
		t.Fatalf("skipped = %d, want 1 (missing quest)", rep.TotalSkipped())
	}
}
