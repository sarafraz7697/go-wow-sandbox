package worldload

import (
	"context"
	"strings"

	"wowserver/internal/entity"
)

// playerInfoLoader ports ObjectMgr::LoadPlayerInfo
// (ObjectMgrPlayerInfo.cpp): playercreateinfo plus its item/spell/action
// extensions and the level/class progression tables. The old static
// loader read the same tables into gamedata.Rows; this loader fills the
// entity dataset the same way, one family per registered loader so the
// progress bar reflects each table separately.
func init() {
	Register(Loader{
		Name:       "playercreateinfo",
		Order:      OrderPlayerInfo,
		CountQuery: "SELECT COUNT(*) FROM `playercreateinfo`",
		Load:       loadPlayerCreateInfos,
	})
	Register(Loader{
		Name:       "playercreateinfo_item",
		Order:      OrderPlayerInfo + 1,
		CountQuery: "SELECT COUNT(*) FROM `playercreateinfo_item`",
		Load:       loadPlayerCreateInfoItems,
	})
	Register(Loader{
		Name:       "playercreateinfo_spell",
		Order:      OrderPlayerInfo + 2,
		CountQuery: "SELECT COUNT(*) FROM `playercreateinfo_spell`",
		Load:       loadPlayerCreateInfoSpells,
	})
	Register(Loader{
		Name:       "playercreateinfo_action",
		Order:      OrderPlayerInfo + 3,
		CountQuery: "SELECT COUNT(*) FROM `playercreateinfo_action`",
		Load:       loadPlayerCreateInfoActions,
	})
	Register(Loader{
		Name:       "player_levelstats",
		Order:      OrderPlayerInfo + 4,
		CountQuery: "SELECT COUNT(*) FROM `player_levelstats`",
		Load:       loadPlayerLevelStats,
	})
	Register(Loader{
		Name:       "player_classlevelstats",
		Order:      OrderPlayerInfo + 5,
		CountQuery: "SELECT COUNT(*) FROM `player_classlevelstats`",
		Load:       loadPlayerClassLevelStats,
	})
	Register(Loader{
		Name:       "player_xp_for_level",
		Order:      OrderPlayerInfo + 6,
		CountQuery: "SELECT COUNT(*) FROM `player_xp_for_level`",
		Load:       loadPlayerXPForLevel,
	})
	Register(Loader{
		Name:       "reserved_name",
		Order:      OrderReservedNames,
		CountQuery: "SELECT COUNT(*) FROM `reserved_name`",
		Load:       loadReservedNames,
	})
}

func loadPlayerCreateInfos(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `race`,`class`,`map`,`zone`,`position_x`,`position_y`,`position_z`,`orientation` FROM `playercreateinfo` ORDER BY `race`,`class`",
		100, func(r scanner) error {
			key := entity.RaceClassKey{}
			var v entity.PlayerCreateInfo
			if err := r.Scan(u32(&key.Race), u32(&key.Class), u32(&v.Map), u32(&v.Zone), f32(&v.PositionX), f32(&v.PositionY), f32(&v.PositionZ), f32(&v.Orientation)); err != nil {
				return err
			}
			if _, dup := dest.PlayerCreateInfos[key]; dup {
				t.Skipf("playercreateinfo has duplicate race/class %v", key)
				return nil
			}
			dest.PlayerCreateInfos[key] = v
			t.Stored(1)
			return nil
		})
}

func loadPlayerCreateInfoItems(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `race`,`class`,`itemid`,`amount` FROM `playercreateinfo_item` ORDER BY `race`,`class`,`itemid`",
		500, func(r scanner) error {
			var v entity.PlayerCreateInfoItem
			if err := r.Scan(u32(&v.Race), u32(&v.Class), u32(&v.ItemID), u32(&v.Amount)); err != nil {
				return err
			}
			dest.PlayerCreateInfoItems = append(dest.PlayerCreateInfoItems, v)
			t.Stored(1)
			return nil
		})
}

func loadPlayerCreateInfoSpells(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `race`,`class`,`Spell` FROM `playercreateinfo_spell` ORDER BY `race`,`class`,`Spell`",
		500, func(r scanner) error {
			var v entity.PlayerCreateInfoSpell
			if err := r.Scan(u32(&v.Race), u32(&v.Class), u32(&v.Spell)); err != nil {
				return err
			}
			dest.PlayerCreateInfoSpells = append(dest.PlayerCreateInfoSpells, v)
			t.Stored(1)
			return nil
		})
}

func loadPlayerCreateInfoActions(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `race`,`class`,`button`,`action`,`type` FROM `playercreateinfo_action` ORDER BY `race`,`class`,`button`",
		500, func(r scanner) error {
			var v entity.PlayerCreateInfoAction
			if err := r.Scan(u32(&v.Race), u32(&v.Class), u32(&v.Button), u32(&v.Action), u32(&v.Type)); err != nil {
				return err
			}
			dest.PlayerCreateInfoActions = append(dest.PlayerCreateInfoActions, v)
			t.Stored(1)
			return nil
		})
}

func loadPlayerLevelStats(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `race`,`class`,`level`,`str`,`agi`,`sta`,`inte`,`spi` FROM `player_levelstats` ORDER BY `race`,`class`,`level`",
		2000, func(r scanner) error {
			var v entity.PlayerLevelStats
			if err := r.Scan(u32(&v.Race), u32(&v.Class), u32(&v.Level), u32(&v.Str), u32(&v.Agi), u32(&v.Sta), u32(&v.Inte), u32(&v.Spi)); err != nil {
				return err
			}
			key := entity.RaceClassLevelKey{Race: v.Race, Class: v.Class, Level: v.Level}
			if _, dup := dest.PlayerLevelStats[key]; dup {
				t.Skipf("player_levelstats has duplicate row %v", key)
				return nil
			}
			dest.PlayerLevelStats[key] = v
			t.Stored(1)
			return nil
		})
}

func loadPlayerClassLevelStats(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `class`,`level`,`basehp`,`basemana` FROM `player_classlevelstats` ORDER BY `class`,`level`",
		1000, func(r scanner) error {
			var v entity.PlayerClassLevelStats
			if err := r.Scan(u32(&v.Class), u32(&v.Level), u32(&v.BaseHP), u32(&v.BaseMana)); err != nil {
				return err
			}
			key := entity.ClassLevelKey{Class: v.Class, Level: v.Level}
			if _, dup := dest.PlayerClassLevelStats[key]; dup {
				t.Skipf("player_classlevelstats has duplicate row %v", key)
				return nil
			}
			dest.PlayerClassLevelStats[key] = v
			t.Stored(1)
			return nil
		})
}

func loadPlayerXPForLevel(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `lvl`,`xp_for_next_level` FROM `player_xp_for_level` ORDER BY `lvl`",
		200, func(r scanner) error {
			var v entity.PlayerXPForLevel
			if err := r.Scan(u32(&v.Level), u32(&v.XPForNextLevel)); err != nil {
				return err
			}
			if v.Level == 0 {
				t.Skipf("player_xp_for_level lvl 0 is not a valid row")
				return nil
			}
			dest.PlayerXPForLevel[v.Level] = v.XPForNextLevel
			t.Stored(1)
			return nil
		})
}

func loadReservedNames(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `name` FROM `reserved_name` ORDER BY `name`",
		500, func(r scanner) error {
			var name string
			if err := r.Scan(str(&name)); err != nil {
				return err
			}
			if name == "" {
				t.Skipf("reserved_name has an empty entry")
				return nil
			}
			dest.ReservedNames[strings.ToLower(name)] = struct{}{}
			t.Stored(1)
			return nil
		})
}
