package worldload

import (
	"context"

	"wowserver/internal/entity"
)

// questTemplateLoader ports ObjectMgr::LoadQuests (ObjectMgrQuests.cpp)
// and the Quest(Field*) constructor (QuestDef.cpp): the full 127-column
// quest_template row plus the four computed objective counts.
func init() {
	Register(Loader{
		Name:       "quest_template",
		Order:      OrderQuests,
		CountQuery: "SELECT COUNT(*) FROM `quest_template`",
		Load:       loadQuests,
	})
}

func questTemplateQuery() string {
	q := "SELECT `entry`,`Method`,`ZoneOrSort`,`MinLevel`,`QuestLevel`,`Type`,`RequiredClasses`,`RequiredRaces`," +
		"`RequiredSkill`,`RequiredSkillValue`,`RepObjectiveFaction`,`RepObjectiveValue`," +
		"`RequiredMinRepFaction`,`RequiredMinRepValue`,`RequiredMaxRepFaction`,`RequiredMaxRepValue`," +
		"`SuggestedPlayers`,`LimitTime`,`QuestFlags`,`SpecialFlags`,`CharTitleId`," +
		"`PrevQuestId`,`NextQuestId`,`ExclusiveGroup`,`NextQuestInChain`,`SrcItemId`,`SrcItemCount`,`SrcSpell`," +
		"`Title`,`Details`,`Objectives`,`OfferRewardText`,`RequestItemsText`,`EndText`"
	for i := 1; i <= entity.MaxQuestObjectives; i++ {
		q += ",`ObjectiveText" + itoa(i) + "`"
	}
	for i := 1; i <= entity.MaxQuestObjectives; i++ {
		q += ",`ReqItemId" + itoa(i) + "`"
	}
	for i := 1; i <= entity.MaxQuestObjectives; i++ {
		q += ",`ReqItemCount" + itoa(i) + "`"
	}
	for i := 1; i <= entity.MaxQuestSourceItemIDs; i++ {
		q += ",`ReqSourceId" + itoa(i) + "`"
	}
	for i := 1; i <= entity.MaxQuestSourceItemIDs; i++ {
		q += ",`ReqSourceCount" + itoa(i) + "`"
	}
	for i := 1; i <= entity.MaxQuestObjectives; i++ {
		q += ",`ReqCreatureOrGOId" + itoa(i) + "`"
	}
	for i := 1; i <= entity.MaxQuestObjectives; i++ {
		q += ",`ReqCreatureOrGOCount" + itoa(i) + "`"
	}
	for i := 1; i <= entity.MaxQuestObjectives; i++ {
		q += ",`ReqSpellCast" + itoa(i) + "`"
	}
	for i := 1; i <= entity.MaxQuestRewardChoices; i++ {
		q += ",`RewChoiceItemId" + itoa(i) + "`"
	}
	for i := 1; i <= entity.MaxQuestRewardChoices; i++ {
		q += ",`RewChoiceItemCount" + itoa(i) + "`"
	}
	for i := 1; i <= entity.MaxQuestRewards; i++ {
		q += ",`RewItemId" + itoa(i) + "`"
	}
	for i := 1; i <= entity.MaxQuestRewards; i++ {
		q += ",`RewItemCount" + itoa(i) + "`"
	}
	for i := 1; i <= entity.MaxQuestReputations; i++ {
		q += ",`RewRepFaction" + itoa(i) + "`"
	}
	for i := 1; i <= entity.MaxQuestReputations; i++ {
		q += ",`RewRepValue" + itoa(i) + "`"
	}
	q += ",`RewHonorableKills`,`RewOrReqMoney`,`RewMoneyMaxLevel`,`RewSpell`,`RewSpellCast`,`RewMailTemplateId`,`RewMailDelaySecs`," +
		"`PointMapId`,`PointX`,`PointY`,`PointOpt`"
	for i := 1; i <= entity.MaxQuestEmotes; i++ {
		q += ",`DetailsEmote" + itoa(i) + "`"
	}
	for i := 1; i <= entity.MaxQuestEmotes; i++ {
		q += ",`DetailsEmoteDelay" + itoa(i) + "`"
	}
	q += ",`IncompleteEmote`,`CompleteEmote`"
	for i := 1; i <= entity.MaxQuestEmotes; i++ {
		q += ",`OfferRewardEmote" + itoa(i) + "`"
	}
	for i := 1; i <= entity.MaxQuestEmotes; i++ {
		q += ",`OfferRewardEmoteDelay" + itoa(i) + "`"
	}
	q += ",`StartScript`,`CompleteScript` FROM `quest_template` ORDER BY `entry`"
	return q
}

func loadQuests(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t, questTemplateQuery(), 2000, func(r scanner) error {
		v := &entity.Quest{}
		dest2 := []any{
			u32(&v.QuestID), u32(&v.QuestMethod), i32(&v.ZoneOrSort), u32(&v.MinLevel), u32(&v.QuestLevel), u32(&v.Type),
			u32(&v.RequiredClasses), u32(&v.RequiredRaces),
			u32(&v.RequiredSkill), u32(&v.RequiredSkillValue), u32(&v.RepObjectiveFaction), i32(&v.RepObjectiveValue),
			u32(&v.RequiredMinRepFaction), i32(&v.RequiredMinRepValue), u32(&v.RequiredMaxRepFaction), i32(&v.RequiredMaxRepValue),
			u32(&v.SuggestedPlayers), u32(&v.LimitTime), u32(&v.QuestFlags), u32(&v.SpecialFlags), u32(&v.CharTitleID),
			i32(&v.PrevQuestID), i32(&v.NextQuestID), i32(&v.ExclusiveGroup), u32(&v.NextQuestInChain), u32(&v.SrcItemID), u32(&v.SrcItemCount), u32(&v.SrcSpell),
			str(&v.Title), str(&v.Details), str(&v.Objectives), str(&v.OfferRewardText), str(&v.RequestItemsText), str(&v.EndText),
		}
		for i := range v.ObjectiveText {
			dest2 = append(dest2, str(&v.ObjectiveText[i]))
		}
		for i := range v.ReqItemID {
			dest2 = append(dest2, u32(&v.ReqItemID[i]))
		}
		for i := range v.ReqItemCount {
			dest2 = append(dest2, u32(&v.ReqItemCount[i]))
		}
		for i := range v.ReqSourceID {
			dest2 = append(dest2, u32(&v.ReqSourceID[i]))
		}
		for i := range v.ReqSourceCount {
			dest2 = append(dest2, u32(&v.ReqSourceCount[i]))
		}
		for i := range v.ReqCreatureOrGOID {
			dest2 = append(dest2, i32(&v.ReqCreatureOrGOID[i]))
		}
		for i := range v.ReqCreatureOrGOCount {
			dest2 = append(dest2, u32(&v.ReqCreatureOrGOCount[i]))
		}
		for i := range v.ReqSpell {
			dest2 = append(dest2, u32(&v.ReqSpell[i]))
		}
		for i := range v.RewChoiceItemID {
			dest2 = append(dest2, u32(&v.RewChoiceItemID[i]))
		}
		for i := range v.RewChoiceItemCount {
			dest2 = append(dest2, u32(&v.RewChoiceItemCount[i]))
		}
		for i := range v.RewItemID {
			dest2 = append(dest2, u32(&v.RewItemID[i]))
		}
		for i := range v.RewItemCount {
			dest2 = append(dest2, u32(&v.RewItemCount[i]))
		}
		for i := range v.RewRepFaction {
			dest2 = append(dest2, u32(&v.RewRepFaction[i]))
		}
		for i := range v.RewRepValue {
			dest2 = append(dest2, i32(&v.RewRepValue[i]))
		}
		dest2 = append(dest2,
			u32(&v.RewHonorableKills), i32(&v.RewOrReqMoney), u32(&v.RewMoneyMaxLevel), u32(&v.RewSpell), u32(&v.RewSpellCast),
			u32(&v.RewMailTemplateID), u32(&v.RewMailDelaySecs),
			u32(&v.PointMapID), f32(&v.PointX), f32(&v.PointY), u32(&v.PointOpt))
		for i := range v.DetailsEmote {
			dest2 = append(dest2, u32(&v.DetailsEmote[i]))
		}
		for i := range v.DetailsEmoteDelay {
			dest2 = append(dest2, u32(&v.DetailsEmoteDelay[i]))
		}
		dest2 = append(dest2, u32(&v.IncompleteEmote), u32(&v.CompleteEmote))
		for i := range v.OfferRewardEmote {
			dest2 = append(dest2, u32(&v.OfferRewardEmote[i]))
		}
		for i := range v.OfferRewardEmoteDelay {
			dest2 = append(dest2, u32(&v.OfferRewardEmoteDelay[i]))
		}
		dest2 = append(dest2, u32(&v.QuestStartScript), u32(&v.QuestCompleteScript))

		if err := r.Scan(dest2...); err != nil {
			return err
		}

		// Computed counts exactly like the Quest constructor: count the
		// non-zero objective slots.
		v.ReqItemsCount = 0
		for _, id := range v.ReqItemID {
			if id != 0 {
				v.ReqItemsCount++
			}
		}
		v.ReqCreatureOrGOCount2 = 0
		for _, id := range v.ReqCreatureOrGOID {
			if id != 0 {
				v.ReqCreatureOrGOCount2++
			}
		}
		v.RewChoiceItemsCount = 0
		for _, id := range v.RewChoiceItemID {
			if id != 0 {
				v.RewChoiceItemsCount++
			}
		}
		v.RewItemsCount = 0
		for _, id := range v.RewItemID {
			if id != 0 {
				v.RewItemsCount++
			}
		}

		if v.QuestID == 0 {
			t.Skipf("quest_template entry 0 is not a valid quest")
			return nil
		}
		if _, dup := dest.Quests[v.QuestID]; dup {
			t.Skipf("quest_template has duplicate entry %d", v.QuestID)
			return nil
		}
		dest.Quests[v.QuestID] = v
		dest.QuestOrder = append(dest.QuestOrder, v.QuestID)
		t.Stored(1)
		return nil
	})
}

// questRelationsLoader ports ObjectMgr::LoadQuestRelations
// (ObjectMgrQuestRelations.cpp): the unified `quest_relations` table is
// split into the four maps the reference keeps — creature/GO ×
// starter/ender.
func init() {
	Register(Loader{
		Name:       "quest_relations",
		Order:      OrderQuestRelations,
		CountQuery: "SELECT COUNT(*) FROM `quest_relations`",
		Load:       loadQuestRelations,
	})
}

func loadQuestRelations(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `actor`,`entry`,`quest`,`role` FROM `quest_relations` ORDER BY `entry`,`quest`",
		2000, func(r scanner) error {
			var rel entity.QuestRelation
			var actor, role nullI32Raw
			if err := r.Scan(&actor, u32(&rel.Entry), u32(&rel.Quest), &role); err != nil {
				return err
			}
			rel.Actor = uint8(actor.v)
			rel.Role = uint8(role.v)

			// Reference validation: the quest must exist; entries are
			// validated against creature/gameobject templates with the
			// same outErrorDb + continue behavior.
			if _, ok := dest.Quests[rel.Quest]; !ok {
				t.Skipf("Table `quest_relations`: quest %d does not exist in quest_template", rel.Quest)
				return nil
			}
			switch rel.Actor {
			case entity.QuestActorCreature:
				if _, ok := dest.CreatureTemplates[rel.Entry]; !ok {
					t.Skipf("Table `quest_relations` has entry %d (creature) that does not exist in creature_template", rel.Entry)
					return nil
				}
			case entity.QuestActorGameObject:
				if _, ok := dest.GameObjectTemplates[rel.Entry]; !ok {
					t.Skipf("Table `quest_relations` has entry %d (gameobject) that does not exist in gameobject_template", rel.Entry)
					return nil
				}
			default:
				t.Skipf("Table `quest_relations` has unknown actor %d", rel.Actor)
				return nil
			}

			switch {
			case rel.Actor == entity.QuestActorCreature && rel.Role == entity.QuestRoleStart:
				if !containsQuest(dest.CreatureQuestStarters[rel.Entry], rel.Quest) {
					dest.CreatureQuestStarters[rel.Entry] = append(dest.CreatureQuestStarters[rel.Entry], rel.Quest)
				}
			case rel.Actor == entity.QuestActorCreature && rel.Role == entity.QuestRoleEnd:
				if !containsQuest(dest.CreatureQuestEnders[rel.Entry], rel.Quest) {
					dest.CreatureQuestEnders[rel.Entry] = append(dest.CreatureQuestEnders[rel.Entry], rel.Quest)
				}
			case rel.Actor == entity.QuestActorGameObject && rel.Role == entity.QuestRoleStart:
				if !containsQuest(dest.GameObjectQuestStarters[rel.Entry], rel.Quest) {
					dest.GameObjectQuestStarters[rel.Entry] = append(dest.GameObjectQuestStarters[rel.Entry], rel.Quest)
				}
			case rel.Actor == entity.QuestActorGameObject && rel.Role == entity.QuestRoleEnd:
				if !containsQuest(dest.GameObjectQuestEnders[rel.Entry], rel.Quest) {
					dest.GameObjectQuestEnders[rel.Entry] = append(dest.GameObjectQuestEnders[rel.Entry], rel.Quest)
				}
			default:
				t.Skipf("Table `quest_relations` has unknown role %d", rel.Role)
				return nil
			}
			t.Stored(1)
			return nil
		})
}

func containsQuest(list []uint32, quest uint32) bool {
	for _, q := range list {
		if q == quest {
			return true
		}
	}
	return false
}
