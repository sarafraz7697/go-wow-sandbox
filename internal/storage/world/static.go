package world

import (
	"context"
	"database/sql"
	"fmt"
	"wowserver/internal/gamedata"
)

// These MaNGOS tables may be MyISAM; a transaction cannot promise a consistent
// snapshot across tables. Administrators must not edit static data during load.
// No queries from this loader run after startup; only SELECT is used.
func (s *Store) LoadStatic(ctx context.Context) (gamedata.Rows, error) {
	var out gamedata.Rows
	queries := []struct {
		table, sql string
		scan       func(*sql.Rows) error
	}{
		// creature_template: full row mirroring the mangos-one 2.4.3 CreatureInfo
		// struct (Object/Creature.h) and the project's world DB schema
		// (docker/init/00-schema.sql). Column order matches the reference source
		// format string CreatureInfosrcfmt in SQLStorages.cpp, restricted to the
		// fields the project consumes: identifier, names, levels, heroic entry,
		// the four model slots, faction/scale/family/type, inhabit + regen +
		// racial-leader, npc/unit/dynamic/extra/type flags, walk/run speeds,
		// unit class, rank, expansion, health/power multipliers, min/max level
		// health and mana, pet spell data id, movement type.
		//
		// Nullable text columns (SubName, IconName) are COALESCEd to ''. `Rank`
		// is backticked because it is a reserved word on MySQL 8.0+.
		{"creature_template", "SELECT Entry,Name,COALESCE(SubName,''),COALESCE(IconName,''),MinLevel,MaxLevel,HeroicEntry,ModelId1,ModelId2,ModelId3,ModelId4,FactionAlliance,FactionHorde,Scale,Family,CreatureType,InhabitType,RegenerateStats,RacialLeader,NpcFlags,UnitFlags,DynamicFlags,ExtraFlags,CreatureTypeFlags,SpeedWalk,SpeedRun,UnitClass,`Rank`,Expansion,HealthMultiplier,PowerMultiplier,MinLevelHealth,MaxLevelHealth,MinLevelMana,MaxLevelMana,PetSpellDataId,MovementType FROM creature_template ORDER BY Entry", func(r *sql.Rows) error {
			var v gamedata.CreatureTemplate
			err := r.Scan(&v.Entry, &v.Name, &v.SubName, &v.IconName, &v.MinLevel, &v.MaxLevel, &v.HeroicEntry, &v.ModelId[0], &v.ModelId[1], &v.ModelId[2], &v.ModelId[3], &v.FactionAlliance, &v.FactionHorde, &v.Scale, &v.Family, &v.CreatureType, &v.InhabitType, &v.RegenerateStats, &v.RacialLeader, &v.NpcFlags, &v.UnitFlags, &v.DynamicFlags, &v.ExtraFlags, &v.CreatureTypeFlags, &v.SpeedWalk, &v.SpeedRun, &v.UnitClass, &v.Rank, &v.Expansion, &v.HealthMultiplier, &v.PowerMultiplier, &v.MinLevelHealth, &v.MaxLevelHealth, &v.MinLevelMana, &v.MaxLevelMana, &v.PetSpellDataId, &v.MovementType)
			if err != nil {
				return err
			}
			out.CreatureTemplates = append(out.CreatureTemplates, v)
			// Keep the legacy (ID, Family) Creature for pet lookups. The catalog
			// key is the entry id, matching the reference's sCreatureStorage.
			out.Creatures = append(out.Creatures, gamedata.Creature{ID: v.Entry, Family: v.Family})
			return nil
		}},
		// gameobject_template: full row mirroring the mangos-one 2.4.3
		// GameObjectInfo struct (Object/GameObject.h). Column order matches the
		// reference source format string GameObjectInfosrcfmt in
		// SQLStorages.cpp. IconName/castBarCaption are nullable in the schema
		// and are COALESCEd to ''. data0..data23 are scanned into the Data
		// array; SMSG_GAMEOBJECT_QUERY_RESPONSE ships the first 6 (24 bytes)
		// raw as info->raw.data.
		{"gameobject_template", "SELECT entry,type,displayId,name,COALESCE(IconName,''),COALESCE(castBarCaption,''),faction,flags,size,data0,data1,data2,data3,data4,data5,data6,data7,data8,data9,data10,data11,data12,data13,data14,data15,data16,data17,data18,data19,data20,data21,data22,data23 FROM gameobject_template ORDER BY entry", func(r *sql.Rows) error {
			var v gamedata.GameObjectTemplate
			err := r.Scan(&v.ID, &v.Type, &v.DisplayID, &v.Name, &v.IconName, &v.CastBarCaption, &v.Faction, &v.Flags, &v.Size, &v.Data[0], &v.Data[1], &v.Data[2], &v.Data[3], &v.Data[4], &v.Data[5], &v.Data[6], &v.Data[7], &v.Data[8], &v.Data[9], &v.Data[10], &v.Data[11], &v.Data[12], &v.Data[13], &v.Data[14], &v.Data[15], &v.Data[16], &v.Data[17], &v.Data[18], &v.Data[19], &v.Data[20], &v.Data[21], &v.Data[22], &v.Data[23])
			if err != nil {
				return err
			}
			out.GameObjectTemplates = append(out.GameObjectTemplates, v)
			return nil
		}},
		// creature spawns: the `creature` table, filtered the same way the
		// mangos-one 2.4.3 reference filters when adding to the grid
		// (ObjectMgrCreatures.cpp: "if (gameEvent == 0 && GuidPoolId == 0 &&
		// EntryPoolId == 0)"). Spawns owned by a game_event or pool are not
		// loaded as static world content; they are managed by those subsystems
		// and would otherwise appear in-world when they should not. LEFT JOIN +
		// IS NULL keeps the query compatible with databases where the event/pool
		// tables are empty (every row passes the filter).
		{"creature", "SELECT c.guid,c.id,c.map,c.position_x,c.position_y,c.position_z,c.orientation,c.curhealth,c.curmana FROM creature c LEFT OUTER JOIN game_event_creature ge ON ge.guid = c.guid LEFT OUTER JOIN pool_creature pc ON pc.guid = c.guid LEFT OUTER JOIN pool_creature_template pct ON pct.id = c.id WHERE ge.guid IS NULL AND pc.guid IS NULL AND pct.id IS NULL ORDER BY c.guid", func(r *sql.Rows) error {
			var v gamedata.CreatureSpawn
			err := r.Scan(&v.GUID, &v.Entry, &v.Map, &v.X, &v.Y, &v.Z, &v.Orientation, &v.Health, &v.Mana)
			if err != nil {
				return err
			}
			out.CreatureSpawns = append(out.CreatureSpawns, v)
			return nil
		}},
		// gameobject spawns: the `gameobject` table, filtered the same way the
		// reference filters when adding to the grid (ObjectMgrGameObjects.cpp:
		// "if (gameEvent == 0 && GuidPoolId == 0 && EntryPoolId == 0)"). Spawns
		// owned by a game_event or pool are not loaded as static world content.
		// LEFT JOIN + IS NULL keeps the query compatible with databases where
		// the event/pool tables are empty.
		{"gameobject", "SELECT g.guid,g.id,g.map,g.position_x,g.position_y,g.position_z,g.orientation,g.rotation0,g.rotation1,g.rotation2,g.rotation3,g.state,g.animprogress FROM gameobject g LEFT OUTER JOIN game_event_gameobject geg ON geg.guid = g.guid LEFT OUTER JOIN pool_gameobject pg ON pg.guid = g.guid LEFT OUTER JOIN pool_gameobject_template pgt ON pgt.id = g.id WHERE geg.guid IS NULL AND pg.guid IS NULL AND pgt.id IS NULL ORDER BY g.guid", func(r *sql.Rows) error {
			var v gamedata.GameObjectSpawn
			err := r.Scan(&v.GUID, &v.Entry, &v.Map, &v.X, &v.Y, &v.Z, &v.Orientation, &v.Rotation0, &v.Rotation1, &v.Rotation2, &v.Rotation3, &v.State, &v.AnimProgress)
			if err != nil {
				return err
			}
			out.GameObjectSpawns = append(out.GameObjectSpawns, v)
			return nil
		}},
		{"playercreateinfo", "SELECT race,class,map,zone,position_x,position_y,position_z,orientation FROM playercreateinfo ORDER BY race,class", func(r *sql.Rows) error {
			var v gamedata.Start
			err := r.Scan(&v.Race, &v.Class, &v.Map, &v.Zone, &v.X, &v.Y, &v.Z, &v.Orientation)
			out.Starts = append(out.Starts, v)
			return err
		}},
		{"item_template", "SELECT entry,displayid,InventoryType,class,subclass,BuyCount,stackable,MaxDurability,duration,bonding,spellcategory_1,spellcharges_1,spellcharges_2,spellcharges_3,spellcharges_4,spellcharges_5 FROM item_template ORDER BY entry", func(r *sql.Rows) error {
			var v gamedata.Item
			err := r.Scan(&v.ID, &v.Display, &v.InventoryType, &v.Class, &v.Subclass, &v.BuyCount, &v.Stackable, &v.MaxDurability, &v.Duration, &v.Bonding, &v.FoodCategory, &v.Charges[0], &v.Charges[1], &v.Charges[2], &v.Charges[3], &v.Charges[4])
			out.Items = append(out.Items, v)
			return err
		}},
		{"playercreateinfo_item", "SELECT race,class,itemid,amount FROM playercreateinfo_item ORDER BY race,class,itemid", func(r *sql.Rows) error {
			var v gamedata.StartItem
			err := r.Scan(&v.Race, &v.Class, &v.Item, &v.Amount)
			out.StartItems = append(out.StartItems, v)
			return err
		}},
		{"playercreateinfo_spell", "SELECT race,class,Spell FROM playercreateinfo_spell ORDER BY race,class,Spell", func(r *sql.Rows) error {
			var v gamedata.StartSpell
			err := r.Scan(&v.Race, &v.Class, &v.Spell)
			out.Spells = append(out.Spells, v)
			return err
		}},
		{"playercreateinfo_action", "SELECT race,class,button,action,type FROM playercreateinfo_action ORDER BY race,class,button", func(r *sql.Rows) error {
			var v gamedata.Action
			err := r.Scan(&v.Race, &v.Class, &v.Button, &v.Action, &v.Type)
			out.Actions = append(out.Actions, v)
			return err
		}},
		{"player_levelstats", "SELECT race,class,level,str,agi,sta,inte,spi FROM player_levelstats ORDER BY race,class,level", func(r *sql.Rows) error {
			var v gamedata.Level
			err := r.Scan(&v.Race, &v.Class, &v.Level, &v.Stats[0], &v.Stats[1], &v.Stats[2], &v.Stats[3], &v.Stats[4])
			out.Levels = append(out.Levels, v)
			return err
		}},
		{"player_classlevelstats", "SELECT class,level,basehp,basemana FROM player_classlevelstats ORDER BY class,level", func(r *sql.Rows) error {
			var v gamedata.ClassLevel
			err := r.Scan(&v.Class, &v.Level, &v.Health, &v.Mana)
			out.ClassLevels = append(out.ClassLevels, v)
			return err
		}},
		{"player_xp_for_level", "SELECT lvl,xp_for_next_level FROM player_xp_for_level ORDER BY lvl", func(r *sql.Rows) error {
			var v gamedata.XP
			err := r.Scan(&v.Level, &v.Next)
			out.XP = append(out.XP, v)
			return err
		}},
		{"reserved_name", "SELECT name FROM reserved_name ORDER BY name", func(r *sql.Rows) error {
			var v string
			err := r.Scan(&v)
			out.Reserved = append(out.Reserved, v)
			return err
		}},
		{"areatrigger_tavern", "SELECT id FROM areatrigger_tavern ORDER BY id", func(r *sql.Rows) error {
			var v uint32
			err := r.Scan(&v)
			out.TavernTriggerIDs = append(out.TavernTriggerIDs, v)
			return err
		}},
	}
	for _, q := range queries {
		if err := s.readStatic(ctx, q.sql, q.scan); err != nil {
			return gamedata.Rows{}, fmt.Errorf("load %s: %w", q.table, err)
		}
	}
	var err error
	out.ItemDetails, err = s.loadItemDetails(ctx)
	if err != nil {
		return out, fmt.Errorf("load item details: %w", err)
	}
	return out, nil
}
func (s *Store) readStatic(ctx context.Context, query string, scan func(*sql.Rows) error) error {
	rows, err := s.DB.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for count := 0; rows.Next(); count++ {
		if count >= 500000 {
			return fmt.Errorf("static table exceeds row limit")
		}
		if err = scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
