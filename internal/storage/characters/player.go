package characters

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"wowserver/internal/domain/character"
	"wowserver/internal/storage/characters/mangosone8606"
	"wowserver/internal/storage/sqlutil"
)

func (s *Store) CheckPlayer(ctx context.Context) error {
	var engine string
	if err := s.DB.QueryRowContext(ctx, "SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='character_tutorial'").Scan(&engine); err != nil {
		return err
	}
	if !strings.EqualFold(engine, "InnoDB") {
		return fmt.Errorf("character_tutorial must use InnoDB for atomic logout")
	}
	return sqlutil.Check(ctx, s.DB, map[string]string{
		"characters":           "SELECT guid,account,online,orientation,xp,money,health,power2,totaltime,leveltime,logout_time FROM characters",
		"character_homebind":   "SELECT guid,map,zone,position_x,position_y,position_z FROM character_homebind",
		"character_tutorial":   "SELECT account,tut0,tut1,tut2,tut3,tut4,tut5,tut6,tut7 FROM character_tutorial",
		"character_spell":      "SELECT guid,spell,active,disabled FROM character_spell",
		"character_action":     "SELECT guid,button,action,type FROM character_action",
		"character_skills":     "SELECT guid,skill,value,max FROM character_skills",
		"character_reputation": "SELECT guid,faction,standing,flags FROM character_reputation",
		"character_inventory":  "SELECT guid,bag,slot,item,item_template FROM character_inventory",
		"item_instance":        "SELECT guid,owner_guid,data FROM item_instance",
	})
}

// AcquirePlayer reserves online atomically. No blanket startup reset: an online
// row left by a crashed process needs explicit operator recovery, not stealing.
func (s *Store) AcquirePlayer(ctx context.Context, accountID uint64, guid uint32) (*character.Player, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	p := &character.Player{}
	var owner uint64
	var online uint8
	err = tx.QueryRowContext(ctx, `SELECT guid,account,name,race,class,gender,level,playerBytes,playerBytes2,playerFlags,at_login,map,zone,position_x,position_y,position_z,orientation,xp,money,health,power2,totaltime,leveltime,online FROM characters WHERE guid=? AND account=? AND (deleteDate IS NULL OR deleteDate=0) FOR UPDATE`, guid, accountID).Scan(&p.GUID, &owner, &p.Name, &p.Race, &p.Class, &p.Gender, &p.Level, &p.PlayerBytes, &p.PlayerBytes2, &p.PlayerFlags, &p.AtLogin, &p.Map, &p.Zone, &p.X, &p.Y, &p.Z, &p.Orientation, &p.XP, &p.Money, &p.Health, &p.Rage, &p.Played, &p.LevelPlayed, &online)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: character missing, deleted or not owned by account", character.ErrPlayerUnavailable)
	}
	if err != nil {
		return nil, err
	}
	if online != 0 {
		return nil, fmt.Errorf("%w: character online flag is set", character.ErrPlayerUnavailable)
	}
	if p.Gender > 1 || p.Level == 0 || p.Level > 70 || p.AtLogin & ^uint32(32) != 0 {
		return nil, character.ErrPlayerUnsupported
	}
	if err = tx.QueryRowContext(ctx, "SELECT power1,power3,power4,power5 FROM characters WHERE guid=?", guid).Scan(&p.Mana, &p.Focus, &p.Energy, &p.Happiness); err != nil {
		return nil, err
	}
	for _, v := range []float32{p.X, p.Y, p.Z, p.Orientation} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || math.Abs(float64(v)) > 100000 {
			return nil, fmt.Errorf("%w: invalid saved position or orientation", character.ErrPlayerUnavailable)
		}
	}
	if err = tx.QueryRowContext(ctx, "SELECT map,zone,position_x,position_y,position_z FROM character_homebind WHERE guid=?", guid).Scan(&p.HomeMap, &p.HomeZone, &p.HomeX, &p.HomeY, &p.HomeZ); err != nil {
		return nil, err
	}
	err = tx.QueryRowContext(ctx, "SELECT tut0,tut1,tut2,tut3,tut4,tut5,tut6,tut7 FROM character_tutorial WHERE account=?", accountID).Scan(&p.Tutorials[0], &p.Tutorials[1], &p.Tutorials[2], &p.Tutorials[3], &p.Tutorials[4], &p.Tutorials[5], &p.Tutorials[6], &p.Tutorials[7])
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	read := func(query string, limit int, scan func(*sql.Rows) error) error {
		rows, e := tx.QueryContext(ctx, query, guid)
		if e != nil {
			return e
		}
		defer rows.Close()
		for n := 0; rows.Next(); n++ {
			if n >= limit {
				return fmt.Errorf("player child row limit exceeded")
			}
			if e = scan(rows); e != nil {
				return e
			}
		}
		return rows.Err()
	}
	if err = read("SELECT spell FROM character_spell WHERE guid=? AND active=1 AND disabled=0 ORDER BY spell", 2048, func(r *sql.Rows) error {
		var id uint32
		e := r.Scan(&id)
		if id > 65535 {
			return character.ErrPlayerUnsupported
		}
		p.Spells = append(p.Spells, id)
		return e
	}); err != nil {
		return nil, err
	}
	if err = read("SELECT button,action,type FROM character_action WHERE guid=? ORDER BY button", 132, func(r *sql.Rows) error {
		var b, a, k uint32
		e := r.Scan(&b, &a, &k)
		if b >= 132 || a > 0xffffff || k > 255 {
			return fmt.Errorf("%w: invalid action button record", character.ErrPlayerUnavailable)
		}
		p.Actions[b] = a | k<<24
		return e
	}); err != nil {
		return nil, err
	}
	if err = read("SELECT skill,value,max FROM character_skills WHERE guid=? ORDER BY skill", 128, func(r *sql.Rows) error {
		var v character.InitialSkill
		e := r.Scan(&v.ID, &v.Value, &v.Max)
		if v.ID > 65535 || v.Value > 65535 || v.Max > 65535 {
			return fmt.Errorf("%w: invalid skill record", character.ErrPlayerUnavailable)
		}
		p.Skills = append(p.Skills, v)
		return e
	}); err != nil {
		return nil, err
	}
	if err = read("SELECT faction,standing,flags FROM character_reputation WHERE guid=? ORDER BY faction", 128, func(r *sql.Rows) error {
		var v character.Reputation
		e := r.Scan(&v.Faction, &v.Standing, &v.Flags)
		p.Reputations = append(p.Reputations, v)
		return e
	}); err != nil {
		return nil, err
	}
	slots := map[uint64]bool{}
	if err = read(`SELECT i.item,i.item_template,i.bag,i.slot,COALESCE(o.owner_guid,0),COALESCE(o.data,'') FROM character_inventory i LEFT JOIN item_instance o ON o.guid=i.item WHERE i.guid=? ORDER BY i.bag,i.slot`, 514, func(r *sql.Rows) error {
		var v character.PlayerItem
		var bag, owner uint32
		var data string
		if e := r.Scan(&v.GUID, &v.Entry, &bag, &v.Slot, &owner, &data); e != nil {
			return e
		}
		key := uint64(bag)<<8 | uint64(v.Slot)
		if (bag == 0 && (v.Slot >= 118 || (v.Slot >= 74 && v.Slot < 86))) || (bag != 0 && v.Slot >= 36) || owner != guid || slots[key] || len(data) > 4096 {
			return character.ErrPlayerUnsupported
		}
		slots[key] = true
		v.Bag = bag
		var err error
		v.Fields, err = mangosone8606.DecodeItem(data, v.GUID, v.Entry, guid)
		if err != nil {
			return err
		}
		p.Items = append(p.Items, v)
		return nil
	}); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE characters SET online=1 WHERE guid=? AND account=?", guid, accountID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return p, nil
}

// ReleasePlayer saves only phase-9 owned values; inventory, spells, money and
// positions are not rewritten from a partial in-memory representation.
func (s *Store) ReleasePlayer(ctx context.Context, accountID uint64, p *character.Player, seconds uint32) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	clear := uint32(0)
	if p.Entered {
		clear = 32
	}
	result, err := tx.ExecContext(ctx, `UPDATE characters SET online=0,logout_time=UNIX_TIMESTAMP(),totaltime=LEAST(totaltime+?,4294967295),leveltime=LEAST(leveltime+?,4294967295),at_login=at_login & ~? WHERE guid=? AND account=? AND online=1`, seconds, seconds, clear, p.GUID, accountID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return character.ErrPlayerUnavailable
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO character_tutorial (account,tut0,tut1,tut2,tut3,tut4,tut5,tut6,tut7) VALUES (?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE tut0=VALUES(tut0),tut1=VALUES(tut1),tut2=VALUES(tut2),tut3=VALUES(tut3),tut4=VALUES(tut4),tut5=VALUES(tut5),tut6=VALUES(tut6),tut7=VALUES(tut7)`, accountID, p.Tutorials[0], p.Tutorials[1], p.Tutorials[2], p.Tutorials[3], p.Tutorials[4], p.Tutorials[5], p.Tutorials[6], p.Tutorials[7])
	if err != nil {
		return err
	}
	return tx.Commit()
}
