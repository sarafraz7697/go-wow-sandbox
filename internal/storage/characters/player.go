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
		"characters":           "SELECT guid,account,online,orientation,xp,money,health,power2,totaltime,leveltime,logout_time,position_x,position_y,position_z,map,zone FROM characters",
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
		// A stale online flag (e.g. from a crashed process) should not permanently
		// lock the character. If logout_time is older than the idle timeout, no
		// live session in this single-process deployment could still own it.
		// Multi-process deployments need external coordination; this is a
		// best-effort recovery for the educational single-process setup.
		var age int64
		if e := tx.QueryRowContext(ctx, "SELECT COALESCE(UNIX_TIMESTAMP()-logout_time, 0) FROM characters WHERE guid=?", guid).Scan(&age); e != nil {
			return nil, e
		}
		if age < int64(s.idleTimeout.Seconds()) {
			return nil, fmt.Errorf("%w: character recently online (%ds ago)", character.ErrPlayerUnavailable, age)
		}
		// Stale: reclaim below. The UPDATE online=1 will overwrite the stale flag.
	}
	if p.Gender > 1 || p.Level == 0 || p.Level > 255 || p.AtLogin & ^uint32(32) != 0 {
		return nil, character.ErrPlayerUnsupported
	}
	if err = tx.QueryRowContext(ctx, "SELECT power1,power3,power4,power5,cinematic FROM characters WHERE guid=?", guid).Scan(&p.Mana, &p.Focus, &p.Energy, &p.Happiness, &p.Cinematic); err != nil {
		return nil, err
	}
	for _, v := range []float32{p.X, p.Y, p.Z, p.Orientation} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || math.Abs(float64(v)) > 100000 {
			return nil, fmt.Errorf("%w: invalid saved position or orientation", character.ErrPlayerUnavailable)
		}
	}
	if err = tx.QueryRowContext(ctx, "SELECT map,zone,position_x,position_y,position_z FROM character_homebind WHERE guid=?", guid).Scan(&p.HomeMap, &p.HomeZone, &p.HomeX, &p.HomeY, &p.HomeZ); errors.Is(err, sql.ErrNoRows) {
		// No bind row: the handler falls back to the race/class start (Player::_LoadHomeBind).
		p.HomeMissing = true
	} else if err != nil {
		return nil, err
	}
	err = tx.QueryRowContext(ctx, "SELECT tut0,tut1,tut2,tut3,tut4,tut5,tut6,tut7 FROM character_tutorial WHERE account=?", accountID).Scan(&p.Tutorials[0], &p.Tutorials[1], &p.Tutorials[2], &p.Tutorials[3], &p.Tutorials[4], &p.Tutorials[5], &p.Tutorials[6], &p.Tutorials[7])
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if e := tx.QueryRowContext(ctx, "SELECT COALESCE(GREATEST(UNIX_TIMESTAMP()-logout_time, 0), 0) FROM characters WHERE guid=?", guid).Scan(&p.OfflineSeconds); e != nil {
		p.OfflineSeconds = 0
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
	if err = read("SELECT caster_guid,item_guid,spell,stackcount,remaincharges,basepoints0,basepoints1,basepoints2,periodictime0,periodictime1,periodictime2,maxduration,remaintime,effIndexMask FROM character_aura WHERE guid=?", 256, func(r *sql.Rows) error {
		var a character.SavedAura
		e := r.Scan(&a.Caster, &a.Item, &a.Spell, &a.Stack, &a.Charges, &a.Amount[0], &a.Amount[1], &a.Amount[2], &a.Period[0], &a.Period[1], &a.Period[2], &a.MaxDuration, &a.Remain, &a.EffectMask)
		if e != nil {
			p.LoadWarnings = append(p.LoadWarnings, fmt.Sprintf("character_aura row ignored: %v", e))
			return nil
		}
		p.Auras = append(p.Auras, a)
		return nil
	}); err != nil {
		return nil, err
	}
	if err = read("SELECT spell,item,time FROM character_spell_cooldown WHERE guid=?", 20000, func(r *sql.Rows) error {
		var c character.SavedCooldown
		if e := r.Scan(&c.Spell, &c.Item, &c.End); e != nil {
			p.LoadWarnings = append(p.LoadWarnings, fmt.Sprintf("character_spell_cooldown row ignored: %v", e))
			return nil
		}
		p.Cooldowns = append(p.Cooldowns, c)
		return nil
	}); err != nil {
		return nil, err
	}
	if err = read("SELECT spell FROM character_spell WHERE guid=? AND active=1 AND disabled=0 ORDER BY spell", 16000, func(r *sql.Rows) error {
		var id uint32
		e := r.Scan(&id)
		if id > 65535 { // SMSG_INITIAL_SPELLS carries 16-bit ids
			p.LoadWarnings = append(p.LoadWarnings, fmt.Sprintf("spell %d ignored: id above 65535", id))
			return e
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
			p.LoadWarnings = append(p.LoadWarnings, fmt.Sprintf("skill %d ignored: invalid value %d/%d", v.ID, v.Value, v.Max))
			return e
		}
		p.Skills = append(p.Skills, v)
		return e
	}); err != nil {
		return nil, err
	}
	p.Skills = normalizeLanguageSkills(p.Race, p.Skills)
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
			p.LoadWarnings = append(p.LoadWarnings, fmt.Sprintf("item %d (entry %d) ignored: invalid slot, owner or duplicate position bag=%d slot=%d", v.GUID, v.Entry, bag, v.Slot))
			return nil
		}
		var err error
		v.Fields, err = mangosone8606.DecodeItem(data, v.GUID, v.Entry, guid)
		if err != nil {
			p.LoadWarnings = append(p.LoadWarnings, fmt.Sprintf("item %d (entry %d) ignored: %v", v.GUID, v.Entry, err))
			return nil
		}
		slots[key] = true
		v.Bag = bag
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

// ReleasePlayer saves logout metadata, tutorial flags and, when the player
// entered the world, the current position/map/zone. Inventory, spells, money
// and health are not rewritten from the partial in-memory representation.
// Position validation lives in application/movement.Apply; storage trusts
// the snapshot it receives and only writes coordinates for an entered player,
// so a login that failed before entry does not overwrite stored coordinates.
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
	var result sql.Result
	if p.Entered {
		result, err = tx.ExecContext(ctx, `UPDATE characters SET online=0,logout_time=UNIX_TIMESTAMP(),totaltime=LEAST(totaltime+?,4294967295),leveltime=LEAST(leveltime+?,4294967295),at_login=at_login & ~?,position_x=?,position_y=?,position_z=?,orientation=?,map=?,zone=?,health=?,power1=?,power2=?,power3=?,power4=?,power5=?,money=?,cinematic=?,playerFlags=? WHERE guid=? AND account=? AND online=1`, seconds, seconds, clear, p.X, p.Y, p.Z, p.Orientation, p.Map, p.Zone, p.Health, p.Mana, p.Rage, p.Focus, p.Energy, p.Happiness, p.Money, p.Cinematic, p.PlayerFlags, p.GUID, accountID)
	} else {
		result, err = tx.ExecContext(ctx, `UPDATE characters SET online=0,logout_time=UNIX_TIMESTAMP(),totaltime=LEAST(totaltime+?,4294967295),leveltime=LEAST(leveltime+?,4294967295),at_login=at_login & ~? WHERE guid=? AND account=? AND online=1`, seconds, seconds, clear, p.GUID, accountID)
	}
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
	if p.Entered && p.InventoryDirty {
		if err = syncInventory(ctx, tx, p); err != nil {
			return err
		}
	}
	if p.Entered {
		if err = saveSpellState(ctx, tx, p); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO character_tutorial (account,tut0,tut1,tut2,tut3,tut4,tut5,tut6,tut7) VALUES (?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE tut0=VALUES(tut0),tut1=VALUES(tut1),tut2=VALUES(tut2),tut3=VALUES(tut3),tut4=VALUES(tut4),tut5=VALUES(tut5),tut6=VALUES(tut6),tut7=VALUES(tut7)`, accountID, p.Tutorials[0], p.Tutorials[1], p.Tutorials[2], p.Tutorials[3], p.Tutorials[4], p.Tutorials[5], p.Tutorials[6], p.Tutorials[7])
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ResetOnline clears the online flag for one character (by guid) or all
// characters (guid == 0). It is an admin recovery tool for stale flags left
// by a crashed process, not part of the normal login/logout path.
func (s *Store) ResetOnline(ctx context.Context, guid uint32) (int64, error) {
	if guid == 0 {
		res, err := s.DB.ExecContext(ctx, "UPDATE characters SET online=0 WHERE online<>0")
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		return n, err
	}
	res, err := s.DB.ExecContext(ctx, "UPDATE characters SET online=0 WHERE guid=? AND online<>0", guid)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return n, err
}

// saveSpellState rewrites character_aura and character_spell_cooldown
// (Player::_SaveAuras, SpellCooldownMgr::SaveToDB).
func saveSpellState(ctx context.Context, tx *sql.Tx, p *character.Player) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_aura WHERE guid=?", p.GUID); err != nil {
		return err
	}
	for _, a := range p.Auras {
		if _, err := tx.ExecContext(ctx, "INSERT INTO character_aura (guid,caster_guid,item_guid,spell,stackcount,remaincharges,basepoints0,basepoints1,basepoints2,periodictime0,periodictime1,periodictime2,maxduration,remaintime,effIndexMask) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
			p.GUID, a.Caster, a.Item, a.Spell, a.Stack, a.Charges, a.Amount[0], a.Amount[1], a.Amount[2], a.Period[0], a.Period[1], a.Period[2], a.MaxDuration, a.Remain, a.EffectMask); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_spell_cooldown WHERE guid=?", p.GUID); err != nil {
		return err
	}
	for _, c := range p.Cooldowns {
		if _, err := tx.ExecContext(ctx, "INSERT INTO character_spell_cooldown (guid,spell,item,time) VALUES (?,?,?,?)", p.GUID, c.Spell, c.Item, c.End); err != nil {
			return err
		}
	}
	return nil
}
