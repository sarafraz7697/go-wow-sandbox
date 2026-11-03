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
		{"creature_template", "SELECT Entry,GREATEST(Family,0) FROM creature_template ORDER BY Entry", func(r *sql.Rows) error {
			var v gamedata.Creature
			err := r.Scan(&v.ID, &v.Family)
			out.Creatures = append(out.Creatures, v)
			return err
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
