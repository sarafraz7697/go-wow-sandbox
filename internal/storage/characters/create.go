package characters

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"wowserver/internal/domain/account"
	"wowserver/internal/domain/character"
	"wowserver/internal/storage/characters/mangosone8606"

	mysql "github.com/go-sql-driver/mysql"
)

type Creator struct {
	Logger      *slog.Logger // Set once before serving; obeys the configured log level.
	db          *sql.DB
	login, lock string
	realm       uint32
	policy      character.CreationPolicy
}

// NewCreator requires a single MySQL server so the realm counter participates
// in the same transaction. No schema or data is changed during this check.
func (s *Store) NewCreator(ctx context.Context, characterDSN, loginDSN string, realm uint32, policy character.CreationPolicy) (*Creator, error) {
	if policy.RealmLimit == 0 || policy.AccountLimit == 0 {
		return nil, fmt.Errorf("creation policy requires positive limits")
	}
	c, err := mysql.ParseDSN(characterDSN)
	if err != nil {
		return nil, fmt.Errorf("invalid character DSN")
	}
	a, err := mysql.ParseDSN(loginDSN)
	if err != nil {
		return nil, fmt.Errorf("invalid login DSN")
	}
	if c.Net != a.Net || c.Addr != a.Addr || c.DBName == "" || a.DBName == "" {
		return nil, fmt.Errorf("character creation requires login and character databases on the same MySQL endpoint")
	}
	tables := map[string][]string{c.DBName: {"characters", "item_instance", "character_inventory", "character_homebind", "character_spell", "character_action", "character_skills", "character_reputation"}}
	tables[a.DBName] = append(tables[a.DBName], "account", "realmcharacters")
	for schema, names := range tables {
		for _, name := range names {
			var engine string
			if err := s.DB.QueryRowContext(ctx, "SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=? AND TABLE_NAME=?", schema, name).Scan(&engine); err != nil {
				return nil, fmt.Errorf("creation table %s.%s: %w", schema, name, err)
			}
			if !strings.EqualFold(engine, "InnoDB") {
				return nil, fmt.Errorf("creation requires InnoDB: %s.%s uses %s", schema, name, engine)
			}
		}
	}
	login := "`" + strings.ReplaceAll(a.DBName, "`", "``") + "`."
	// Check cross-schema access before accepting clients.
	rows, err := s.DB.QueryContext(ctx, "SELECT acctid,numchars FROM "+login+"realmcharacters LIMIT 0")
	if err != nil {
		return nil, err
	}
	rows.Close()
	sum := sha256.Sum256([]byte(c.DBName))
	// Stash the validated login schema prefix and realm on Store so the
	// delete path can decrement realmcharacters.numchars in its own
	// transaction. NewCreator runs once at startup before the listener
	// accepts traffic; subsequent reads by Delete are safe under the same
	// "set once before serving" contract as SetIdleTimeout.
	s.loginSchema = login
	s.realmID = realm
	return &Creator{db: s.DB, login: login, realm: realm, policy: policy, lock: fmt.Sprintf("wow-create-%x", sum[:20])}, nil
}

func (s *Creator) Create(ctx context.Context, a account.WorldSession, ip string, p character.Creation) (uint32, error) {
	started, stageStarted := time.Now(), time.Now()
	stage := "connection"
	writes := 0
	log := s.Logger
	if log == nil {
		log = slog.Default()
	}
	log = log.With("account_id", a.ID, "realm_id", s.realm)
	mark := func(next string) {
		log.Debug("character creation stage", "stage", stage, "elapsed_ms", float64(time.Since(stageStarted).Microseconds())/1000)
		stage, stageStarted = next, time.Now()
	}
	defer func() {
		mark("done")
		log.Debug("character creation storage finished", "elapsed_ms", float64(time.Since(started).Microseconds())/1000, "write_statements", writes)
	}()
	if a.ID == 0 || a.ID > math.MaxUint32 {
		return 0, character.ErrDisabled
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	mark("advisory_lock")
	// A pinned connection owns the advisory lock; all Go writers for this
	// character schema use it. C++ world writers must not run concurrently.
	var locked sql.NullInt64
	if err = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?,5)", s.lock).Scan(&locked); err != nil {
		return 0, err
	}
	if !locked.Valid || locked.Int64 != 1 {
		return 0, fmt.Errorf("character creation busy")
	}
	defer func() {
		mark("release_lock")
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var released sql.NullInt64
		if e := conn.QueryRowContext(releaseCtx, "SELECT RELEASE_LOCK(?)", s.lock).Scan(&released); e != nil || !released.Valid || released.Int64 != 1 {
			// Never return a session holding a named lock to the pool.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	mark("begin_transaction")
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	mark("account_lock_and_session")
	var key, builds string
	var icon uint32
	query := `SELECT COALESCE(a.sessionkey,''),r.realmbuilds,r.icon FROM ` + s.login + `account a JOIN ` + s.login + `realmlist r ON r.id=?
 WHERE a.id=? AND (a.locked=0 OR a.last_ip=?) AND LEAST(a.gmlevel,3)>=LEAST(r.allowedSecurityLevel,3)
 AND NOT EXISTS(SELECT 1 FROM ` + s.login + `account_banned b WHERE b.id=a.id AND b.active=1 AND (b.unbandate=b.bandate OR b.unbandate>UNIX_TIMESTAMP()))
 AND NOT EXISTS(SELECT 1 FROM ` + s.login + `ip_banned b WHERE b.ip=? AND (b.unbandate=b.bandate OR b.unbandate>UNIX_TIMESTAMP())) FOR UPDATE`
	if err = tx.QueryRowContext(ctx, query, s.realm, a.ID, ip, ip).Scan(&key, &builds, &icon); err != nil {
		return 0, err
	}
	var expected [40]byte
	for i := range expected {
		expected[i] = a.Key[39-i]
	}
	compatible := false
	for _, b := range strings.Fields(builds) {
		if b == "8606" {
			compatible = true
		}
	}
	if !compatible || !strings.EqualFold(strings.TrimLeft(key, "0"), strings.TrimLeft(hex.EncodeToString(expected[:]), "0")) {
		return 0, account.ErrChanged
	}
	var count, other uint32
	mark("limits_and_name")
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters WHERE account=? AND (deleteDate IS NULL OR deleteDate=0)", a.ID).Scan(&count); err != nil {
		return 0, err
	}

	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(SUM(numchars),0) FROM "+s.login+"realmcharacters WHERE acctid=? AND realmid<>?", a.ID, s.realm).Scan(&other); err != nil {
		return 0, err
	}

	var exists bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM characters WHERE name=?)", p.Request.Name).Scan(&exists); err != nil {
		return 0, err
	}
	facts := character.CreationFacts{RealmCount: count, OtherRealmsCount: other, NameTaken: exists, RealmIcon: icon}
	{
		// The reference rejects a PvP-realm creation when the account already
		// has a character of the OPPOSING team (CharacterHandler.cpp compares
		// TeamForRace of the new race against the existing characters), not
		// "any Horde character exists". Compare teams instead of hardcoding
		// one side.
		opposing := "1,3,7,11" // Alliance races
		if p.Team == 1 {
			opposing = "2,5,6,8,10" // Horde races
		}
		if p.Team == 1 || p.Team == 7 {
			if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM characters WHERE account=? AND race IN ("+opposing+") AND (deleteDate IS NULL OR deleteDate=0))", a.ID).Scan(&exists); err != nil {
				return 0, err
			}
			facts.HasOpposingFaction = exists
		}
	}
	if err = s.policy.Check(facts); err != nil {
		return 0, err
	}
	var next, itemNext uint64
	mark("allocate_guids")
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid),0)+1 FROM characters").Scan(&next); err != nil {
		return 0, err
	}
	if err = tx.QueryRowContext(ctx, "SELECT GREATEST((SELECT COALESCE(MAX(guid),0) FROM item_instance),(SELECT COALESCE(MAX(item),0) FROM character_inventory),(SELECT COALESCE(MAX(item_guid),0) FROM mail_items))+1").Scan(&itemNext); err != nil {
		return 0, err
	}
	if next > math.MaxUint32 || itemNext+uint64(len(p.Items)) > math.MaxUint32 {
		return 0, fmt.Errorf("GUID space exhausted")
	}
	guid := uint32(next)
	mark("write_character")
	r := p.Request
	bytes1 := uint32(r.Skin) | uint32(r.Face)<<8 | uint32(r.HairStyle)<<16 | uint32(r.HairColor)<<24
	bytes2 := uint32(r.FacialHair) | 2<<24
	cache := make([]uint32, 38)
	for _, item := range p.Items {
		if item.Slot < 19 {
			cache[int(item.Slot)*2] = item.Template.ItemID
		}
	}
	// Preserve the first SQL error and stop executing subsequent writes.
	exec := func(q string, args ...any) {
		if err == nil {
			writes++
			_, err = tx.ExecContext(ctx, q, args...)
			if err != nil {
				err = fmt.Errorf("character creation %s: %w", stage, err)
			}
		}
	}
	// Multi-row VALUES reduces round trips without enabling multiStatements or
	// changing transaction boundaries. All values remain bound parameters.
	batch := func(prefix, tuple string, rows [][]any) {
		for start := 0; start < len(rows) && err == nil; start += 128 {
			end := min(start+128, len(rows))
			var args []any
			for _, row := range rows[start:end] {
				args = append(args, row...)
			}
			exec(prefix+strings.TrimSuffix(strings.Repeat(tuple+",", end-start), ","), args...)
		}
	}
	now := time.Now().Unix()
	// power1/power4 are persisted like the reference SaveToDB at creation
	// (Player::Create -> InitStatsForLevel fills mana/energy before the first
	// save). power2 (rage) and power3 (focus) stay 0 via explicit defaults.
	exec(`INSERT INTO characters (guid,account,name,race,class,gender,level,playerBytes,playerBytes2,position_x,position_y,position_z,map,orientation,zone,at_login,health,power1,power2,power3,power4,watchedFaction,taximask,exploredZones,equipmentCache,knownTitles,createdDate,logout_time)
 VALUES (?,?,?,?,?,?,1,?,?,?,?,?,?,?,?,32,?,?,0,0,?,4294967295,?,?,?,?,?,?)`, guid, a.ID, r.Name, r.Race, r.Class, r.Gender, bytes1, bytes2, p.Start.X, p.Start.Y, p.Start.Z, p.Start.Map, p.Start.Orientation, p.Start.Zone, p.Health, p.Mana, p.Energy, uints(p.TaxiMask[:]), strings.Repeat("0 ", 128), uints(cache), "0 0 ", now, now)
	exec("INSERT INTO character_homebind (guid,map,zone,position_x,position_y,position_z) VALUES (?,?,?,?,?,?)", guid, p.Start.Map, p.Start.Zone, p.Start.X, p.Start.Y, p.Start.Z)
	mark("write_spells")
	rows := make([][]any, 0, len(p.Spells))
	for _, spell := range p.Spells {
		rows = append(rows, []any{guid, spell})
	}
	batch("INSERT INTO character_spell (guid,spell,active,disabled) VALUES ", "(?,?,1,0)", rows)
	mark("write_actions")
	rows = nil
	for _, action := range p.Actions {
		rows = append(rows, []any{guid, action.Button, action.Action, action.Type})
	}
	batch("INSERT INTO character_action (guid,button,action,type) VALUES ", "(?,?,?,?)", rows)
	mark("write_skills")
	rows = nil
	for _, skill := range p.Skills {
		rows = append(rows, []any{guid, skill.ID, skill.Value, skill.Max})
	}
	batch("INSERT INTO character_skills (guid,skill,value,max) VALUES ", "(?,?,?,?)", rows)
	mark("write_reputations")
	rows = nil
	for _, rep := range p.Reputations {
		rows = append(rows, []any{guid, rep.Faction, rep.Flags})
	}
	batch("INSERT INTO character_reputation (guid,faction,standing,flags) VALUES ", "(?,?,0,?)", rows)
	mark("write_items")
	rows = nil
	var inventory [][]any
	for i, item := range p.Items {
		id := uint32(itemNext + uint64(i))
		rows = append(rows, []any{id, guid, mangosone8606.EncodeItem(id, guid, item)})
		inventory = append(inventory, []any{guid, item.Slot, id, item.Template.ItemID})
	}
	batch("INSERT INTO item_instance (guid,owner_guid,data,text) VALUES ", "(?,?,?,'')", rows)
	batch("INSERT INTO character_inventory (guid,bag,slot,item,item_template) VALUES ", "(?,0,?,?,?)", inventory)
	mark("write_realm_count")
	exec("INSERT INTO "+s.login+"realmcharacters (realmid,acctid,numchars) VALUES (?,?,?) ON DUPLICATE KEY UPDATE numchars=VALUES(numchars)", s.realm, a.ID, count+1)
	if err != nil {
		return 0, err
	}
	mark("commit")
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return guid, nil
}

func uints(values []uint32) string {
	var b strings.Builder
	for _, v := range values {
		b.WriteString(strconv.FormatUint(uint64(v), 10))
		b.WriteByte(' ')
	}
	return b.String()
}
