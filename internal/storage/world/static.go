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
		// creature_template: full row for CREATE_OBJECT + CMSG_CREATURE_QUERY.
		{"creature_template", "SELECT entry,name,subname,minlevel,maxlevel,modelid_1,faction_A,faction_H,npcflag,unit_flags,type,family,rank,minhealth,maxhealth,minmana,maxmana FROM creature_template ORDER BY entry", func(r *sql.Rows) error {
			var v gamedata.CreatureTemplate
			err := r.Scan(&v.ID, &v.Name, &v.SubName, &v.MinLevel, &v.MaxLevel, &v.ModelID, &v.FactionAlliance, &v.FactionHorde, &v.NPCFlags, &v.UnitFlags, &v.CreatureType, &v.Family, &v.Rank, &v.MinHealth, &v.MaxHealth, &v.MinMana, &v.MaxMana)
			if err != nil {
				return err
			}
			out.CreatureTemplates = append(out.CreatureTemplates, v)
			// Keep the old (ID, Family) Creature for pet lookups.
			out.Creatures = append(out.Creatures, gamedata.Creature{ID: v.ID, Family: v.Family})
			return nil
		}},
		// gameobject_template: full row for CREATE_OBJECT + CMSG_GAMEOBJECT_QUERY.
		{"gameobject_template", "SELECT entry,type,displayId,name,faction,flags,size FROM gameobject_template ORDER BY entry", func(r *sql.Rows) error {
			var v gamedata.GameObjectTemplate
			err := r.Scan(&v.ID, &v.Type, &v.DisplayID, &v.Name, &v.Faction, &v.Flags, &v.Size)
			if err != nil {
				return err
			}
			out.GameObjectTemplates = append(out.GameObjectTemplates, v)
			return nil
		}},
		// creature spawns: the `creature` table. One row = one spawn point.
		{"creature", "SELECT guid,id,map,position_x,position_y,position_z,orientation,curhealth,curmana FROM creature ORDER BY guid", func(r *sql.Rows) error {
			var v gamedata.CreatureSpawn
			err := r.Scan(&v.GUID, &v.Entry, &v.Map, &v.X, &v.Y, &v.Z, &v.Orientation, &v.Health, &v.Mana)
			if err != nil {
				return err
			}
			out.CreatureSpawns = append(out.CreatureSpawns, v)
			return nil
		}},
		// gameobject spawns: the `gameobject` table. One row = one spawn point.
		{"gameobject", "SELECT guid,id,map,position_x,position_y,position_z,orientation,rotation0,rotation1,rotation2,rotation3,state,animprogress FROM gameobject ORDER BY guid", func(r *sql.Rows) error {
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
