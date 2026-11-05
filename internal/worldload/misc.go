package worldload

import (
	"context"
	"strings"

	"wowserver/internal/entity"
)

// This file holds the remaining world-DB entity loaders from the
// reference startup sequence, each a self-registering Loader like all
// the others: mangos_string, spell_template, instance_template,
// areatrigger_teleport, areatrigger_tavern, game_graveyard_zone,
// exploration_basexp, pet_name_generation, game_event, conditions,
// game_tele, points_of_interest, item_required_target, the reputation
// tables, mail_level_reward, skill_fishing_base_level,
// petcreateinfo_spell and pet_levelstats.

func init() {
	Register(Loader{
		Name:       "mangos_string",
		Order:      OrderMangosString,
		CountQuery: "SELECT COUNT(*) FROM `mangos_string`",
		Load:       loadMangosStrings,
	})
	Register(Loader{
		Name:       "spell_template",
		Order:      OrderSpellTemplate,
		CountQuery: "SELECT COUNT(*) FROM `spell_template`",
		Load:       loadSpellTemplates,
	})
	Register(Loader{
		Name:       "instance_template",
		Order:      OrderInstanceTemplate,
		CountQuery: "SELECT COUNT(*) FROM `instance_template`",
		Load:       loadInstanceTemplates,
	})
	Register(Loader{
		Name:       "areatrigger_teleport",
		Order:      OrderAreaTriggerTeleport,
		CountQuery: "SELECT COUNT(*) FROM `areatrigger_teleport`",
		Load:       loadAreaTriggerTeleports,
	})
	Register(Loader{
		Name:       "areatrigger_tavern",
		Order:      OrderAreaTriggerTavern,
		CountQuery: "SELECT COUNT(*) FROM `areatrigger_tavern`",
		Load:       loadAreaTriggerTaverns,
	})
	Register(Loader{
		Name:       "game_graveyard_zone",
		Order:      OrderGraveyardZones,
		CountQuery: "SELECT COUNT(*) FROM `game_graveyard_zone`",
		Load:       loadGraveyardZones,
	})
	Register(Loader{
		Name:       "exploration_basexp",
		Order:      OrderExplorationBaseXP,
		CountQuery: "SELECT COUNT(*) FROM `exploration_basexp`",
		Load:       loadExplorationBaseXP,
	})
	Register(Loader{
		Name:       "pet_name_generation",
		Order:      OrderPetNames,
		CountQuery: "SELECT COUNT(*) FROM `pet_name_generation`",
		Load:       loadPetNames,
	})
	Register(Loader{
		Name:       "game_event",
		Order:      OrderGameEvent,
		CountQuery: "SELECT COUNT(*) FROM `game_event`",
		Load:       loadGameEvents,
	})
	Register(Loader{
		Name:       "conditions",
		Order:      OrderConditions,
		CountQuery: "SELECT COUNT(*) FROM `conditions`",
		Load:       loadConditions,
	})
	Register(Loader{
		Name:       "game_tele",
		Order:      OrderGameTele,
		CountQuery: "SELECT COUNT(*) FROM `game_tele`",
		Load:       loadGameTeles,
	})
	Register(Loader{
		Name:       "points_of_interest",
		Order:      OrderPointsOfInterest,
		CountQuery: "SELECT COUNT(*) FROM `points_of_interest`",
		Load:       loadPointsOfInterest,
	})
	Register(Loader{
		Name:       "item_required_target",
		Order:      OrderItemRequiredTarget,
		CountQuery: "SELECT COUNT(*) FROM `item_required_target`",
		Load:       loadItemRequiredTargets,
	})
	Register(Loader{
		Name:       "reputation_reward_rate",
		Order:      OrderReputationRewardRate,
		CountQuery: "SELECT COUNT(*) FROM `reputation_reward_rate`",
		Load:       loadReputationRewardRates,
	})
	Register(Loader{
		Name:       "reputation_spillover_template",
		Order:      OrderReputationSpillover,
		CountQuery: "SELECT COUNT(*) FROM `reputation_spillover_template`",
		Load:       loadReputationSpilloverTemplates,
	})
	Register(Loader{
		Name:       "mail_level_reward",
		Order:      OrderMailLevelRewards,
		CountQuery: "SELECT COUNT(*) FROM `mail_level_reward`",
		Load:       loadMailLevelRewards,
	})
	Register(Loader{
		Name:       "skill_fishing_base_level",
		Order:      OrderFishingBaseSkill,
		CountQuery: "SELECT COUNT(*) FROM `skill_fishing_base_level`",
		Load:       loadFishingBaseSkillLevels,
	})
	Register(Loader{
		Name:       "petcreateinfo_spell",
		Order:      OrderPetCreateSpells,
		CountQuery: "SELECT COUNT(*) FROM `petcreateinfo_spell`",
		Load:       loadPetCreateSpells,
	})
	Register(Loader{
		Name:       "pet_levelstats",
		Order:      OrderPetLevelStats,
		CountQuery: "SELECT COUNT(*) FROM `pet_levelstats`",
		Load:       loadPetLevelStats,
	})
}

// loadMangosStrings ports ObjectMgr::LoadMangosStrings: entry, the
// default text and the 8 locale columns.
func loadMangosStrings(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `entry`,`content_default`,`content_loc1`,`content_loc2`,`content_loc3`,`content_loc4`,`content_loc5`,`content_loc6`,`content_loc7`,`content_loc8` FROM `mangos_string` ORDER BY `entry`",
		2000, func(r scanner) error {
			var v entity.MangosString
			dest2 := []any{u32(&v.Entry), str(&v.ContentDefault)}
			for i := range v.ContentLoc {
				dest2 = append(dest2, str(&v.ContentLoc[i]))
			}
			if err := r.Scan(dest2...); err != nil {
				return err
			}
			if v.Entry == 0 {
				t.Skipf("mangos_string entry 0 is not a valid string")
				return nil
			}
			if _, dup := dest.MangosStrings[v.Entry]; dup {
				t.Skipf("mangos_string has duplicate entry %d", v.Entry)
				return nil
			}
			dest.MangosStrings[v.Entry] = v
			t.Stored(1)
			return nil
		})
}

func loadSpellTemplates(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `id`,`proc_flags`,`proc_chance`,`duration_index`,`effect0`,`effect0_implicit_target_a`,`effect0_radius_idx`,`effect0_apply_aura_name`,`effect0_misc_value`,`effect0_misc_value_b`,`effect0_trigger_spell` FROM `spell_template` ORDER BY `id`",
		2000, func(r scanner) error {
			var v entity.SpellTemplate
			if err := r.Scan(u32(&v.ID), u32(&v.ProcFlags), u32(&v.ProcChance), u32(&v.DurationIndex), u32(&v.Effect0),
				u32(&v.Effect0ImplicitTargetA), u32(&v.Effect0RadiusIdx), u32(&v.Effect0ApplyAuraName), u32(&v.Effect0MiscValue), u32(&v.Effect0MiscValueB), u32(&v.Effect0TriggerSpell)); err != nil {
				return err
			}
			if v.ID == 0 {
				t.Skipf("spell_template id 0 is not a valid spell")
				return nil
			}
			dest.SpellTemplates[v.ID] = v
			t.Stored(1)
			return nil
		})
}

func loadInstanceTemplates(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `map`,`parent`,`levelMin`,`levelMax`,`maxPlayers`,`reset_delay` FROM `instance_template` ORDER BY `map`",
		500, func(r scanner) error {
			var v entity.InstanceTemplate
			if err := r.Scan(u32(&v.Map), u32(&v.Parent), u32(&v.LevelMin), u32(&v.LevelMax), u32(&v.MaxPlayers), u32(&v.ResetDelay)); err != nil {
				return err
			}
			dest.InstanceTemplates[v.Map] = v
			t.Stored(1)
			return nil
		})
}

// loadAreaTriggerTeleports ports ObjectMgr::LoadAreaTriggerTeleports.
func loadAreaTriggerTeleports(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `id`,`name`,`required_level`,`required_item`,`required_item2`,`heroic_key`,`heroic_key2`,`required_quest_done`,`required_quest_done_heroic`,`target_map`,`target_position_x`,`target_position_y`,`target_position_z`,`target_orientation` FROM `areatrigger_teleport` ORDER BY `id`",
		1000, func(r scanner) error {
			var v entity.AreaTriggerTeleport
			if err := r.Scan(u32(&v.ID), str(&v.Name), u32(&v.RequiredLevel), u32(&v.RequiredItem), u32(&v.RequiredItem2),
				u32(&v.HeroicKey), u32(&v.HeroicKey2), u32(&v.RequiredQuestDone), u32(&v.RequiredQuestDoneHeroic),
				u32(&v.TargetMapID), f32(&v.TargetX), f32(&v.TargetY), f32(&v.TargetZ), f32(&v.TargetOrientation)); err != nil {
				return err
			}
			if v.ID == 0 {
				t.Skipf("areatrigger_teleport id 0 is not a valid trigger")
				return nil
			}
			dest.AreaTriggerTeleports[v.ID] = v
			t.Stored(1)
			return nil
		})
}

// loadAreaTriggerTaverns ports ObjectMgr::LoadTavernAreaTriggers.
func loadAreaTriggerTaverns(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `id`,`name` FROM `areatrigger_tavern` ORDER BY `id`",
		500, func(r scanner) error {
			var v entity.AreaTriggerTavern
			if err := r.Scan(u32(&v.ID), str(&v.Name)); err != nil {
				return err
			}
			if v.ID == 0 {
				t.Skipf("areatrigger_tavern id 0 is not a valid trigger")
				return nil
			}
			if _, dup := dest.TavernTriggers[v.ID]; dup {
				t.Skipf("areatrigger_tavern has duplicate id %d", v.ID)
				return nil
			}
			dest.TavernTriggers[v.ID] = v
			t.Stored(1)
			return nil
		})
}

// loadGraveyardZones ports ObjectMgr::LoadGraveyardZones.
func loadGraveyardZones(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `id`,`ghost_zone`,`faction` FROM `game_graveyard_zone`",
		1000, func(r scanner) error {
			var v entity.GraveyardZoneLink
			if err := r.Scan(u32(&v.ID), u32(&v.GhostZone), u32(&v.Faction)); err != nil {
				return err
			}
			if v.ID == 0 {
				t.Skipf("game_graveyard_zone id 0 is not a valid graveyard link")
				return nil
			}
			// Reference validation: faction must be 0 (both), 1 (horde)
			// or 2 (alliance) — other values are skipped.
			if v.Faction > 2 {
				t.Skipf("Table `game_graveyard_zone` has record for non player faction (%d), skipped", v.Faction)
				return nil
			}
			dest.GraveyardLinks = append(dest.GraveyardLinks, v)
			t.Stored(1)
			return nil
		})
}

// loadExplorationBaseXP ports ObjectMgr::LoadExplorationBaseXP.
func loadExplorationBaseXP(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `level`,`basexp` FROM `exploration_basexp` ORDER BY `level`",
		200, func(r scanner) error {
			var level, basexp uint32
			if err := r.Scan(u32(&level), u32(&basexp)); err != nil {
				return err
			}
			if level == 0 {
				t.Skipf("exploration_basexp level 0 is not a valid row")
				return nil
			}
			dest.ExplorationBaseXP[level] = basexp
			t.Stored(1)
			return nil
		})
}

// loadPetNames ports ObjectMgr::LoadPetNames.
func loadPetNames(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `id`,`word`,`entry`,`half` FROM `pet_name_generation` ORDER BY `id`",
		500, func(r scanner) error {
			var v entity.PetNameGeneration
			if err := r.Scan(u32(&v.ID), str(&v.Word), u32(&v.Entry), u32(&v.Half)); err != nil {
				return err
			}
			if strings.TrimSpace(v.Word) == "" {
				t.Skipf("pet_name_generation word is empty")
				return nil
			}
			dest.PetNames = append(dest.PetNames, v)
			t.Stored(1)
			return nil
		})
}

func loadGameEvents(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `entry`,`start_time`,`end_time`,`occurence`,`length`,`holiday`,`description` FROM `game_event` ORDER BY `entry`",
		200, func(r scanner) error {
			var v entity.GameEvent
			if err := r.Scan(u32(&v.Entry), str(&v.StartTime), str(&v.EndTime), u32(&v.Occurence), u32(&v.Length), u32(&v.Holiday), str(&v.Description)); err != nil {
				return err
			}
			dest.GameEvents[v.Entry] = v
			t.Stored(1)
			return nil
		})
}

func loadConditions(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `condition_entry`,`type`,`value1`,`value2` FROM `conditions` ORDER BY `condition_entry`",
		1000, func(r scanner) error {
			var v entity.PlayerCondition
			if err := r.Scan(u32(&v.ConditionEntry), u32(&v.Type), u32(&v.Value1), u32(&v.Value2)); err != nil {
				return err
			}
			dest.Conditions[v.ConditionEntry] = v
			t.Stored(1)
			return nil
		})
}

func loadGameTeles(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `id`,`position_x`,`position_y`,`position_z`,`orientation`,`map`,`name` FROM `game_tele` ORDER BY `id`",
		500, func(r scanner) error {
			var v entity.GameTele
			if err := r.Scan(u32(&v.ID), f32(&v.PositionX), f32(&v.PositionY), f32(&v.PositionZ), f32(&v.Orientation), u32(&v.Map), str(&v.Name)); err != nil {
				return err
			}
			if v.ID == 0 || strings.TrimSpace(v.Name) == "" {
				t.Skipf("game_tele row %d is not valid (needs id and name)", v.ID)
				return nil
			}
			dest.GameTeles[v.ID] = v
			t.Stored(1)
			return nil
		})
}

func loadPointsOfInterest(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `entry`,`x`,`y`,`icon`,`flags`,`data`,`icon_name` FROM `points_of_interest` ORDER BY `entry`",
		500, func(r scanner) error {
			var v entity.PointOfInterest
			if err := r.Scan(u32(&v.Entry), f32(&v.X), f32(&v.Y), u32(&v.Icon), u32(&v.Flags), u32(&v.Data), str(&v.IconName)); err != nil {
				return err
			}
			if v.Entry == 0 {
				t.Skipf("points_of_interest entry 0 is not a valid POI")
				return nil
			}
			dest.PointsOfInterest[v.Entry] = v
			t.Stored(1)
			return nil
		})
}

func loadItemRequiredTargets(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `entry`,`type`,`targetEntry` FROM `item_required_target`",
		1000, func(r scanner) error {
			var v entity.ItemRequiredTarget
			if err := r.Scan(u32(&v.Entry), u32(&v.Type), u32(&v.TargetEntry)); err != nil {
				return err
			}
			dest.ItemRequiredTargets = append(dest.ItemRequiredTargets, v)
			t.Stored(1)
			return nil
		})
}

func loadReputationRewardRates(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `faction`,`quest_rate`,`creature_rate`,`spell_rate` FROM `reputation_reward_rate`",
		1000, func(r scanner) error {
			var v entity.ReputationRewardRate
			if err := r.Scan(u32(&v.Faction), f32(&v.QuestRate), f32(&v.CreatureRate), f32(&v.SpellRate)); err != nil {
				return err
			}
			dest.ReputationRewardRates[v.Faction] = v
			t.Stored(1)
			return nil
		})
}

func loadReputationSpilloverTemplates(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `faction`,`faction1`,`rate_1`,`rank_1`,`faction2`,`rate_2`,`rank_2`,`faction3`,`rate_3`,`rank_3`,`faction4`,`rate_4`,`rank_4` FROM `reputation_spillover_template`",
		1000, func(r scanner) error {
			var v entity.ReputationSpilloverTemplate
			dest2 := []any{u32(&v.Faction)}
			for i := 0; i < 4; i++ {
				dest2 = append(dest2, u32(&v.SpilloverFaction[i]), f32(&v.Rate[i]), u32(&v.Rank[i]))
			}
			if err := r.Scan(dest2...); err != nil {
				return err
			}
			dest.ReputationSpilloverTemplates[v.Faction] = v
			t.Stored(1)
			return nil
		})
}

func loadMailLevelRewards(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `level`,`raceMask`,`mailTemplateId`,`senderEntry` FROM `mail_level_reward`",
		500, func(r scanner) error {
			var v entity.MailLevelReward
			if err := r.Scan(u32(&v.Level), u32(&v.RaceMask), u32(&v.MailTemplateID), u32(&v.SenderEntry)); err != nil {
				return err
			}
			dest.MailLevelRewards = append(dest.MailLevelRewards, v)
			t.Stored(1)
			return nil
		})
}

func loadFishingBaseSkillLevels(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `entry`,`skill` FROM `skill_fishing_base_level`",
		200, func(r scanner) error {
			var entry, skill uint32
			if err := r.Scan(u32(&entry), u32(&skill)); err != nil {
				return err
			}
			dest.FishingBaseSkillLevels[entry] = skill
			t.Stored(1)
			return nil
		})
}

func loadPetCreateSpells(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `entry`,`Spell1`,`Spell2`,`Spell3`,`Spell4` FROM `petcreateinfo_spell` ORDER BY `entry`",
		200, func(r scanner) error {
			var v entity.PetCreateInfoSpell
			if err := r.Scan(u32(&v.Entry), u32(&v.Spells[0]), u32(&v.Spells[1]), u32(&v.Spells[2]), u32(&v.Spells[3])); err != nil {
				return err
			}
			if v.Entry == 0 {
				t.Skipf("petcreateinfo_spell entry 0 is not a valid pet")
				return nil
			}
			dest.PetCreateSpells[v.Entry] = v
			t.Stored(1)
			return nil
		})
}

func loadPetLevelStats(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `creature_entry`,`level`,`hp`,`mana`,`armor`,`str`,`agi`,`sta`,`inte`,`spi` FROM `pet_levelstats` ORDER BY `creature_entry`,`level`",
		2000, func(r scanner) error {
			var v entity.PetLevelStats
			if err := r.Scan(u32(&v.CreatureEntry), u32(&v.Level), u32(&v.HP), u32(&v.Mana), u32(&v.Armor),
				u32(&v.Str), u32(&v.Agi), u32(&v.Sta), u32(&v.Inte), u32(&v.Spi)); err != nil {
				return err
			}
			if v.CreatureEntry == 0 || v.Level == 0 {
				t.Skipf("pet_levelstats row with zero entry/level")
				return nil
			}
			dest.PetLevelStats[entity.PetLevelStatsKey{Entry: v.CreatureEntry, Level: v.Level}] = v
			t.Stored(1)
			return nil
		})
}
