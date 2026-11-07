package worldload

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"wowserver/internal/entity"
)

// fakeDriver serves canned results keyed by a prefix of the query. It is
// the minimum database/sql plumbing needed to exercise the runners and
// loaders without a MySQL server.
type fakeDriver struct{ s *fakeServer }

func (d *fakeDriver) Open(name string) (driver.Conn, error) { return &fakeConn{s: d.s}, nil }

type fakeServer struct {
	mu      sync.Mutex
	rows    map[string]driver.Rows
	queried map[string]int
}

// put registers rows under an exact query string.
func (s *fakeServer) put(query string, rows driver.Rows) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rows == nil {
		s.rows = map[string]driver.Rows{}
	}
	s.rows[strings.ToLower(strings.TrimSpace(query))] = rows
}

func (s *fakeServer) lookup(query string) (driver.Rows, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.queried == nil {
		s.queried = map[string]int{}
	}
	key := strings.ToLower(strings.TrimSpace(query))
	s.queried[key]++
	if r, ok := s.rows[key]; ok {
		// Return a fresh cursor each time: one fakeRows cannot serve two
		// queries because its position state is consumed.
		if fr, ok := r.(*fakeRows); ok {
			clone := *fr
			clone.pos = 0
			return &clone, nil
		}
		return r, nil
	}
	return nil, fmt.Errorf("Error 1146: Table 'world.nope' doesn't exist")
}

type fakeConn struct{ s *fakeServer }

func (c *fakeConn) Prepare(query string) (driver.Stmt, error) {
	return &fakeStmt{s: c.s, query: query}, nil
}
func (c *fakeConn) Close() error              { return nil }
func (c *fakeConn) Begin() (driver.Tx, error) { return nil, errors.New("tx unsupported") }

type fakeStmt struct {
	s     *fakeServer
	query string
}

func (st *fakeStmt) Close() error  { return nil }
func (st *fakeStmt) NumInput() int { return -1 }
func (st *fakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	return st.s.lookup(st.query)
}
func (st *fakeStmt) Exec(args []driver.Value) (driver.Result, error) {
	return nil, errors.New("exec unsupported")
}

// fakeRows is a static driver.Rows.
type fakeRows struct {
	cols []string
	data [][]driver.Value
	pos  int
}

func (r *fakeRows) Columns() []string { return r.cols }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.pos])
	r.pos++
	return nil
}

// fakeDriver implements driver.Rows but not driver.RowsNext, which is
// fine for database/sql's fallback path.
var (
	_ driver.Driver = (*fakeDriver)(nil)
	_ driver.Conn   = (*fakeConn)(nil)
	_ driver.Stmt   = (*fakeStmt)(nil)
	_ driver.Rows   = (*fakeRows)(nil)
)

func newFakeDB(t *testing.T) (*sql.DB, *fakeServer) {
	t.Helper()
	srv := &fakeServer{}
	name := fmt.Sprintf("fake-%p", srv)
	sql.Register(name, &fakeDriver{s: srv})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatalf("open fake db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, srv
}

func rows1(n int64) *fakeRows {
	return &fakeRows{cols: []string{"c"}, data: [][]driver.Value{{n}}}
}

func TestRegistryOrdersAndDedups(t *testing.T) {
	ResetForTest()
	Register(Loader{Name: "zzz", Order: 200, CountQuery: "SELECT 1", Load: func(context.Context, Queryer, *Dataset, *Table) error { return nil }})
	Register(Loader{Name: "aaa", Order: 100, CountQuery: "SELECT 1", Load: func(context.Context, Queryer, *Dataset, *Table) error { return nil }})
	got := Loaders()
	if len(got) != 2 || got[0].Name != "aaa" || got[1].Name != "zzz" {
		t.Fatalf("loaders not ordered: %+v", got)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate registration must panic")
		}
	}()
	Register(Loader{Name: "aaa", Order: 100, CountQuery: "SELECT 1", Load: func(context.Context, Queryer, *Dataset, *Table) error { return nil }})
}

func TestRunStreamsProgressAndStores(t *testing.T) {
	ResetForTest()
	db, srv := newFakeDB(t)

	// creature_template: two valid rows, one entry-0 skip.
	ctCols := []string{"Entry", "Name", "SubName", "IconName", "MinLevel", "MaxLevel", "HeroicEntry",
		"ModelId1", "ModelId2", "ModelId3", "ModelId4", "FactionAlliance", "FactionHorde", "Scale",
		"Family", "CreatureType", "InhabitType", "RegenerateStats", "RacialLeader",
		"NpcFlags", "UnitFlags", "DynamicFlags", "ExtraFlags", "CreatureTypeFlags",
		"SpeedWalk", "SpeedRun", "UnitClass", "Rank", "Expansion",
		"HealthMultiplier", "PowerMultiplier", "DamageMultiplier", "DamageVariance", "ArmorMultiplier", "ExperienceMultiplier",
		"MinLevelHealth", "MaxLevelHealth", "MinLevelMana", "MaxLevelMana",
		"MinMeleeDmg", "MaxMeleeDmg", "MinRangedDmg", "MaxRangedDmg",
		"Armor", "MeleeAttackPower", "RangedAttackPower", "MeleeBaseAttackTime", "RangedBaseAttackTime", "DamageSchool",
		"MinLootGold", "MaxLootGold", "LootId", "PickpocketLootId", "SkinningLootId",
		"KillCredit1", "KillCredit2", "MechanicImmuneMask", "SchoolImmuneMask",
		"ResistanceHoly", "ResistanceFire", "ResistanceNature", "ResistanceFrost", "ResistanceShadow", "ResistanceArcane",
		"PetSpellDataId", "MovementType",
		"TrainerType", "TrainerSpell", "TrainerClass", "TrainerRace", "TrainerTemplateId", "VendorTemplateId", "EquipmentTemplateId", "GossipMenuId",
		"AIName"}
	base := []driver.Value{
		int64(1), "Defias Thug", nil, nil, int64(5), int64(5), int64(0),
		int64(398), int64(0), int64(0), int64(0), int64(11), int64(11), 1.0,
		int64(0), int64(7), int64(3), int64(3), int64(0),
		int64(1), int64(0), int64(0), int64(0), int64(0),
		1.0, 1.14286, int64(1), int64(0), int64(-1),
		1.0, 1.0, 1.0, 1.0, 1.0, 1.0,
		int64(100), int64(120), int64(0), int64(0),
		2.5, 3.5, 100.0, 120.0,
		int64(20), int64(22), int64(3), int64(2000), int64(2000), int64(0),
		int64(1), int64(15), int64(0), int64(0), int64(0),
		int64(0), int64(0), int64(0), int64(0),
		int64(0), int64(0), int64(0), int64(0), int64(0), int64(0),
		int64(0), int64(0),
		int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0),
		"",
	}
	entry2 := append([]driver.Value{}, base...)
	entry2[0] = int64(2)
	entry2[1] = "Stormwind Guard"
	entry2[4], entry2[5] = int64(0), int64(3) // invalid min/max pair, gets fixed
	zeroEntry := append([]driver.Value{}, base...)
	zeroEntry[0] = int64(0)
	srv.put("SELECT COUNT(*) FROM `creature_template`", rows1(3))

	goCols := []string{"entry", "type", "displayId", "name", "IconName", "castBarCaption", "faction", "flags", "size"}
	for i := 0; i < entity.MaxGOData; i++ {
		goCols = append(goCols, fmt.Sprintf("data%d", i))
	}
	goRow := []driver.Value{int64(31), int64(2), int64(10), "Gold Mine", "", "", int64(0), int64(0), 1.0}
	for i := 0; i < entity.MaxGOData; i++ {
		goRow = append(goRow, int64(0))
	}
	srv.put("SELECT COUNT(*) FROM `gameobject_template`", rows1(1))

	// Register the real loaders (as their own files do) and the run.
	Register(Loader{Name: "creature_template", Order: OrderCreatureTemplate, CountQuery: "SELECT COUNT(*) FROM `creature_template`", Load: loadCreatureTemplates})
	Register(Loader{Name: "gameobject_template", Order: OrderGameObjectTemplate, CountQuery: "SELECT COUNT(*) FROM `gameobject_template`", Load: loadGameObjectTemplates})

	const ctQuery0 = "SELECT `Entry`,`Name`,`SubName`,`IconName`,`MinLevel`,`MaxLevel`,`HeroicEntry`,`ModelId1`,`ModelId2`,`ModelId3`,`ModelId4`,`FactionAlliance`,`FactionHorde`,`Scale`,`Family`,`CreatureType`,`InhabitType`,`RegenerateStats`,`RacialLeader`,`NpcFlags`,`UnitFlags`,`DynamicFlags`,`ExtraFlags`,`CreatureTypeFlags`,`SpeedWalk`,`SpeedRun`,`UnitClass`,`Rank`,`Expansion`,`HealthMultiplier`,`PowerMultiplier`,`DamageMultiplier`,`DamageVariance`,`ArmorMultiplier`,`ExperienceMultiplier`,`MinLevelHealth`,`MaxLevelHealth`,`MinLevelMana`,`MaxLevelMana`,`MinMeleeDmg`,`MaxMeleeDmg`,`MinRangedDmg`,`MaxRangedDmg`,`Armor`,`MeleeAttackPower`,`RangedAttackPower`,`MeleeBaseAttackTime`,`RangedBaseAttackTime`,`DamageSchool`,`MinLootGold`,`MaxLootGold`,`LootId`,`PickpocketLootId`,`SkinningLootId`,`KillCredit1`,`KillCredit2`,`MechanicImmuneMask`,`SchoolImmuneMask`,`ResistanceHoly`,`ResistanceFire`,`ResistanceNature`,`ResistanceFrost`,`ResistanceShadow`,`ResistanceArcane`,`PetSpellDataId`,`MovementType`,`TrainerType`,`TrainerSpell`,`TrainerClass`,`TrainerRace`,`TrainerTemplateId`,`VendorTemplateId`,`EquipmentTemplateId`,`GossipMenuId`,`AIName` FROM `creature_template` ORDER BY `Entry`"
	srv.put(ctQuery0, &fakeRows{cols: ctCols, data: [][]driver.Value{base, entry2, zeroEntry}})

	goQuery := "SELECT `entry`,`type`,`displayId`,`name`,`IconName`,`castBarCaption`,`faction`,`flags`,`size`"
	for i := 0; i < entity.MaxGOData; i++ {
		goQuery += ",`data" + fmt.Sprint(i) + "`"
	}
	goQuery += " FROM `gameobject_template` ORDER BY `entry`"
	srv.put(goQuery, &fakeRows{cols: goCols, data: [][]driver.Value{goRow}})

	dest := newDatasetForTest()
	var pcts []int
	var steps []string
	rep, err := Run(context.Background(), db, dest, RunOptions{
		Progress: func(pct int, step string) {
			pcts = append(pcts, pct)
			steps = append(steps, step)
		},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := len(dest.CreatureTemplates); got != 2 {
		t.Fatalf("creature templates = %d, want 2", got)
	}
	ct := dest.CreatureTemplates[1]
	if ct.Name != "Defias Thug" || ct.ModelId[0] != 398 || ct.Expansion != -1 || ct.Scale != 1.0 {
		t.Fatalf("creature template 1 fields wrong: %+v", ct)
	}
	if ct.SubName != "" {
		t.Fatalf("NULL SubName must scan to empty string, got %q", ct.SubName)
	}
	ct2 := dest.CreatureTemplates[2]
	if ct2.MinLevel != 1 || ct2.MaxLevel != 3 {
		t.Fatalf("minlevel fixup failed: %+v", ct2)
	}
	if got := len(dest.GameObjectTemplates); got != 1 {
		t.Fatalf("gameobject templates = %d, want 1", got)
	}
	if rep.TotalRows() != 3 || rep.TotalSkipped() != 2 {
		t.Fatalf("report rows=%d skipped=%d, want 3 rows 2 skips", rep.TotalRows(), rep.TotalSkipped())
	}
	// Progress must be monotonic and reach 100.
	for i := 1; i < len(pcts); i++ {
		if pcts[i] < pcts[i-1] {
			t.Fatalf("progress not monotonic: %v", pcts)
		}
	}
	if len(pcts) == 0 || pcts[len(pcts)-1] != 100 {
		t.Fatalf("progress must end at 100, got %v", pcts)
	}
	// Tables load concurrently, so only require that both reported.
	joined := strings.Join(steps, "|")
	for _, name := range []string{"creature_template", "gameobject_template"} {
		if !strings.Contains(joined, name) {
			t.Fatalf("progress steps should mention %s: %v", name, steps)
		}
	}
}

func TestMissingTableSkippedWhenAllowed(t *testing.T) {
	ResetForTest()
	db, srv := newFakeDB(t)
	Register(Loader{
		Name:        "optional_thing",
		Order:       100,
		CountQuery:  "SELECT COUNT(*) FROM `optional_thing`",
		Load:        func(context.Context, Queryer, *Dataset, *Table) error { return nil },
		SkipMissing: true,
	})
	_ = srv
	dest := newDatasetForTest()
	rep, err := Run(context.Background(), db, dest, RunOptions{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !rep.Tables[0].Absent {
		t.Fatalf("table should be reported absent")
	}
}

func TestMissingTableFatalWithoutFlag(t *testing.T) {
	ResetForTest()
	db, _ := newFakeDB(t)
	Register(Loader{
		Name:       "required_thing",
		Order:      100,
		CountQuery: "SELECT COUNT(*) FROM `required_thing`",
		Load:       func(context.Context, Queryer, *Dataset, *Table) error { return nil },
	})
	_, err := Run(context.Background(), db, newDatasetForTest(), RunOptions{})
	if err == nil {
		t.Fatal("missing table without SkipMissing must fail the run")
	}
}
