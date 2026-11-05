package worldload

import (
	"context"
	"strings"

	"wowserver/internal/entity"
)

// creatureModelInfoLoader ports ObjectMgr::LoadCreatureModelInfo
// (creature_model_info, sCreatureModelStorage): modelid, bounding_radius,
// combat_reach, gender, modelid_other_gender, modelid_alternative.
func init() {
	Register(Loader{
		Name:       "creature_model_info",
		Order:      OrderCreatureModelInfo,
		CountQuery: "SELECT COUNT(*) FROM `creature_model_info`",
		Load:       loadCreatureModelInfos,
	})
}

func loadCreatureModelInfos(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `modelid`,`bounding_radius`,`combat_reach`,`gender`,`modelid_other_gender`,`modelid_alternative` FROM `creature_model_info` ORDER BY `modelid`",
		2000, func(r scanner) error {
			var v entity.CreatureModelInfo
			if err := r.Scan(u32(&v.ModelID), f32(&v.BoundingRadius), f32(&v.CombatReach), u8(&v.Gender), u32(&v.ModelIDOtherGender), u32(&v.ModelIDAlternative)); err != nil {
				return err
			}
			if v.ModelID == 0 {
				t.Skipf("creature_model_info entry 0 is not a valid model")
				return nil
			}
			if _, dup := dest.CreatureModelInfos[v.ModelID]; dup {
				t.Skipf("creature_model_info has duplicate modelid %d", v.ModelID)
				return nil
			}
			dest.CreatureModelInfos[v.ModelID] = v
			t.Stored(1)
			return nil
		})
}

// creatureEquipTemplateLoader ports ObjectMgr::LoadEquipmentTemplates for
// both layouts: the current creature_equip_template (item entries) and the
// deprecated creature_equip_template_raw (display/info/slot triples).
func init() {
	Register(Loader{
		Name:       "creature_equip_template",
		Order:      OrderCreatureEquipTemplate,
		CountQuery: "SELECT (SELECT COUNT(*) FROM `creature_equip_template`) + (SELECT COUNT(*) FROM `creature_equip_template_raw`)",
		Load:       loadEquipmentTemplates,
	})
}

func loadEquipmentTemplates(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	if err := scanTable(ctx, q, t,
		"SELECT `entry`,`equipentry1`,`equipentry2`,`equipentry3` FROM `creature_equip_template` ORDER BY `entry`",
		2000, func(r scanner) error {
			var v entity.EquipmentInfo
			if err := r.Scan(u32(&v.Entry), u32(&v.EquipEntry[0]), u32(&v.EquipEntry[1]), u32(&v.EquipEntry[2])); err != nil {
				return err
			}
			if v.Entry == 0 {
				t.Skipf("creature_equip_template entry 0 is not a valid equipment set")
				return nil
			}
			if _, dup := dest.EquipmentTemplates[v.Entry]; dup {
				t.Skipf("creature_equip_template has duplicate entry %d", v.Entry)
				return nil
			}
			dest.EquipmentTemplates[v.Entry] = v
			t.Stored(1)
			return nil
		}); err != nil {
		return err
	}
	return scanTable(ctx, q, t,
		"SELECT `entry`,`equipmodel1`,`equipmodel2`,`equipmodel3`,`equipinfo1`,`equipinfo2`,`equipinfo3`,`equipslot1`,`equipslot2`,`equipslot3` FROM `creature_equip_template_raw` ORDER BY `entry`",
		2000, func(r scanner) error {
			var v entity.EquipmentInfoRaw
			if err := r.Scan(u32(&v.Entry), u32(&v.EquipModel[0]), u32(&v.EquipModel[1]), u32(&v.EquipModel[2]),
				u32(&v.EquipInfo[0]), u32(&v.EquipInfo[1]), u32(&v.EquipInfo[2]),
				u32(&v.EquipSlot[0]), u32(&v.EquipSlot[1]), u32(&v.EquipSlot[2])); err != nil {
				return err
			}
			if v.Entry == 0 {
				t.Skipf("creature_equip_template_raw entry 0 is not a valid equipment set")
				return nil
			}
			if _, dup := dest.EquipmentTemplatesRaw[v.Entry]; dup {
				t.Skipf("creature_equip_template_raw has duplicate entry %d", v.Entry)
				return nil
			}
			dest.EquipmentTemplatesRaw[v.Entry] = v
			t.Stored(1)
			return nil
		})
}

// creatureAddonLoader ports ObjectMgr::LoadCreatureAddons: both
// creature_template_addon (keyed by entry) and creature_addon (keyed by
// spawn guid). The auras column is a space-separated spell list in both
// tables and is parsed to the same uint32 slice the reference builds.
func init() {
	Register(Loader{
		Name:       "creature_addons",
		Order:      OrderCreatureAddons,
		CountQuery: "SELECT (SELECT COUNT(*) FROM `creature_template_addon`) + (SELECT COUNT(*) FROM `creature_addon`)",
		Load:       loadCreatureAddons,
	})
}

func scanAddon(r scanner) (guidOrEntry uint32, a entity.CreatureDataAddon, err error) {
	var auras string
	err = r.Scan(u32(&guidOrEntry), u32(&a.Mount), u32(&a.Bytes1), u8(&a.SheathState), u8(&a.Flags), u32(&a.Emote), u32(&a.MoveFlags), str(&auras))
	if err == nil {
		a.Auras = parseAuraList(auras)
	}
	return guidOrEntry, a, err
}

// parseAuraList splits "spell1 spell2 ..." into ids, ignoring blanks,
// exactly like the reference's strtok loop over the auras column.
func parseAuraList(s string) []uint32 {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Fields(s)
	out := make([]uint32, 0, len(parts))
	for _, p := range parts {
		var v uint32
		for i := 0; i < len(p); i++ {
			if p[i] < '0' || p[i] > '9' {
				v = 0
				break
			}
			v = v*10 + uint32(p[i]-'0')
		}
		if v != 0 {
			out = append(out, v)
		}
	}
	return out
}

func loadCreatureAddons(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	if err := scanTable(ctx, q, t,
		"SELECT `entry`,`mount`,`bytes1`,`b2_0_sheath`,`b2_1_flags`,`emote`,`moveflags`,`auras` FROM `creature_template_addon` ORDER BY `entry`",
		2000, func(r scanner) error {
			entry, a, err := scanAddon(r)
			if err != nil {
				return err
			}
			if _, dup := dest.CreatureTemplateAddons[entry]; dup {
				t.Skipf("creature_template_addon has duplicate entry %d", entry)
				return nil
			}
			dest.CreatureTemplateAddons[entry] = entity.CreatureInfoAddon(a)
			t.Stored(1)
			return nil
		}); err != nil {
		return err
	}
	return scanTable(ctx, q, t,
		"SELECT `guid`,`mount`,`bytes1`,`b2_0_sheath`,`b2_1_flags`,`emote`,`moveflags`,`auras` FROM `creature_addon` ORDER BY `guid`",
		2000, func(r scanner) error {
			guid, a, err := scanAddon(r)
			if err != nil {
				return err
			}
			if _, dup := dest.CreatureAddons[guid]; dup {
				t.Skipf("creature_addon has duplicate guid %d", guid)
				return nil
			}
			dest.CreatureAddons[guid] = a
			t.Stored(1)
			return nil
		})
}

// creatureClassLvlStatsLoader ports ObjectMgr::LoadCreatureClassLvlStats:
// creature_template_classlevelstats rows are split per expansion into the
// (class, level, expansion) keyed map, mirroring the reference's
// m_creatureClassLvlStats[level][classIndex][expansion] array.
func init() {
	Register(Loader{
		Name:       "creature_template_classlevelstats",
		Order:      OrderCreatureClassLvlStats,
		CountQuery: "SELECT COUNT(*) FROM `creature_template_classlevelstats`",
		Load:       loadCreatureClassLvlStats,
	})
}

func loadCreatureClassLvlStats(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	query := "SELECT `Class`,`Level`,`BaseMana`,`BaseMeleeAttackPower`,`BaseRangedAttackPower`,`BaseArmor`"
	for e := 0; e <= entity.MaxExpansion; e++ {
		query += ",`BaseHealthExp" + itoa(e) + "`,`BaseDamageExp" + itoa(e) + "`"
	}
	query += " FROM `creature_template_classlevelstats` ORDER BY `Class`,`Level`"

	return scanTable(ctx, q, t, query, 500, func(r scanner) error {
		var v entity.CreatureClassLvlStats
		dest2 := []any{
			u32(&v.Class), u32(&v.Level), u32(&v.BaseMana), f32(&v.BaseMeleeAttackPower), f32(&v.BaseRangedAttackPower), u32(&v.BaseArmor),
		}
		// One (health, damage) pair per expansion, scanned in column order
		// so a single row fans out to MaxExpansion+1 dataset entries.
		healths := make([]uint32, entity.MaxExpansion+1)
		damages := make([]float32, entity.MaxExpansion+1)
		for e := 0; e <= entity.MaxExpansion; e++ {
			dest2 = append(dest2, u32(&healths[e]), f32(&damages[e]))
		}
		if err := r.Scan(dest2...); err != nil {
			return err
		}
		// Reference validation: level 0 or above DEFAULT_MAX_CREATURE_LEVEL
		// (100) is skipped; classes outside CLASSMASK_ALL_CREATURES (the
		// creature classes 1..4 in the 2.4.3 core) are skipped.
		if v.Level == 0 || v.Level > 100 || v.Class < 1 || v.Class > 4 {
			t.Skipf("Found stats for creature class/level [%d/%d], incorrect for this core", v.Class, v.Level)
			return nil
		}
		for e := 0; e <= entity.MaxExpansion; e++ {
			row := v
			row.Expansion = int32(e)
			row.BaseHealth = healths[e]
			row.BaseDamage = damages[e]
			key := entity.CreatureClassLevelKey{Class: row.Class, Level: row.Level, Expansion: row.Expansion}
			if _, dup := dest.CreatureClassLvlStats[key]; dup {
				t.Skipf("creature_template_classlevelstats has duplicate row for class=%d level=%d exp=%d", row.Class, row.Level, row.Expansion)
				continue
			}
			dest.CreatureClassLvlStats[key] = row
			t.Stored(1)
		}
		return nil
	})
}

// creatureTemplateSpellsLoader ports ObjectMgr::LoadCreatureTemplateSpells
// (sCreatureTemplateSpellsStorage).
func init() {
	Register(Loader{
		Name:       "creature_template_spells",
		Order:      OrderCreatureTemplateSpells,
		CountQuery: "SELECT COUNT(*) FROM `creature_template_spells`",
		Load:       loadCreatureTemplateSpells,
	})
}

func loadCreatureTemplateSpells(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `entry`,`spell1`,`spell2`,`spell3`,`spell4` FROM `creature_template_spells` ORDER BY `entry`",
		2000, func(r scanner) error {
			var v entity.CreatureTemplateSpells
			if err := r.Scan(u32(&v.Entry), u32(&v.Spells[0]), u32(&v.Spells[1]), u32(&v.Spells[2]), u32(&v.Spells[3])); err != nil {
				return err
			}
			if v.Entry == 0 {
				t.Skipf("creature_template_spells entry 0 is not a valid row")
				return nil
			}
			if _, dup := dest.CreatureTemplateSpells[v.Entry]; dup {
				t.Skipf("creature_template_spells has duplicate entry %d", v.Entry)
				return nil
			}
			dest.CreatureTemplateSpells[v.Entry] = v
			t.Stored(1)
			return nil
		})
}

// creatureModelRaceLoader ports ObjectMgr::LoadCreatureModelRace
// (creature_model_race): racemask -> model overrides for shapeshift
// display selection.
func init() {
	Register(Loader{
		Name:       "creature_model_race",
		Order:      OrderCreatureModelRace,
		CountQuery: "SELECT COUNT(*) FROM `creature_model_race`",
		Load:       loadCreatureModelRace,
	})
}

func loadCreatureModelRace(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `modelid`,`racemask`,`creature_entry`,`modelid_racial` FROM `creature_model_race`",
		2000, func(r scanner) error {
			var v entity.CreatureModelRace
			if err := r.Scan(u32(&v.ModelID), u32(&v.RaceMask), u32(&v.CreatureEntry), u32(&v.ModelIDRacial)); err != nil {
				return err
			}
			if v.ModelID == 0 {
				t.Skipf("creature_model_race modelid 0 is not a valid row")
				return nil
			}
			dest.CreatureModelRaces = append(dest.CreatureModelRaces, v)
			t.Stored(1)
			return nil
		})
}
