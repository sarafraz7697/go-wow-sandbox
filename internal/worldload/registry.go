package worldload

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Queryer is the subset of *sql.DB the loaders need. It exists so tests
// can drive loaders from a fake driver.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Dataset re-exports the entity dataset so loader files need one import.
type Dataset = datasetAlias

// LoadFunc streams one entity family into the dataset. It reports rows
// through the Table handle: t.Progress(n) drives the live percentage,
// t.Stored() counts kept rows, t.Skipf() counts-and-logs a dropped row
// exactly like a MaNGOS outErrorDb line followed by `continue`.
type LoadFunc func(ctx context.Context, q Queryer, dest *Dataset, t *Table) error

// Loader describes one entity family. Name is the entity (primary table)
// name used in logs; CountQuery must return exactly one row with one
// column holding the expected row count (used for the progress budget,
// like the SELECT COUNT(*) MaNGOS runs before drawing its BarGoLink).
type Loader struct {
	Name        string
	Order       int
	CountQuery  string
	Load        LoadFunc
	SkipMissing bool // log at info level when the table is absent instead of failing
}

// registry holds every registered loader. Registration happens in init()
// functions; the map is frozen once main starts.
var (
	registryMu sync.RWMutex
	registry   = make(map[string]Loader)
)

// Register adds a loader. Registering the same name twice panics — it is
// a programming error, not a runtime condition.
func Register(l Loader) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[l.Name]; dup {
		panic(fmt.Sprintf("worldload: duplicate loader %q", l.Name))
	}
	if l.Name == "" || l.Load == nil || l.CountQuery == "" {
		panic(fmt.Sprintf("worldload: loader %q has empty Name, CountQuery or Load", l.Name))
	}
	registry[l.Name] = l
}

// Loaders returns every registered loader in MaNGOS load order. The order
// is stable: equal Order values fall back to name order.
func Loaders() []Loader {
	registryMu.RLock()
	out := make([]Loader, 0, len(registry))
	for _, l := range registry {
		out = append(out, l)
	}
	registryMu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Table is the per-loader run handle: live progress, row/skip counters
// and capped warnings. All methods are safe for the single goroutine that
// runs the loader.
type Table struct {
	Name string

	log        Logger
	progress   func(rowsDone int)
	rowsTotal  int64
	warnBudget int

	rows    int
	skipped int
}

// Progress reports the running row count for the live percentage.
func (t *Table) Progress(rowsDone int) {
	if t.progress != nil {
		t.progress(rowsDone)
	}
}

// Stored records that n rows were kept in the dataset.
func (t *Table) Stored(n int) { t.rows += n }

// Skipf records a row dropped by validation and logs a warning like
// MaNGOS sLog.outErrorDb, capped at a few lines per table (the rest are
// counted silently). Returns nothing; use instead of logging directly.
func (t *Table) Skipf(format string, args ...any) {
	t.skipped++
	if t.log != nil && t.warnBudget > 0 {
		t.warnBudget--
		t.log.Warn(fmt.Sprintf(format, args...)+" [skipped]", "table", t.Name)
	}
}

// Rows returns the kept-row count (used by the run report).
func (t *Table) Rows() int { return t.rows }

// Skipped returns the dropped-row count.
func (t *Table) Skipped() int { return t.skipped }

// scanTable runs one streaming SELECT. scan consumes each row through
// the scanner (a *sql.Rows); an error returned by scan aborts the load
// like a MaNGOS table-structure error. Progress is reported with the
// running count every progressEvery rows. The query must be ordered so
// dataset iteration is deterministic.
func scanTable(ctx context.Context, q Queryer, t *Table, query string, progressEvery int, scan func(r scanner) error) error {
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	n := 0
	if progressEvery <= 0 {
		progressEvery = 1000
	}
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
		n++
		if n%progressEvery == 0 {
			t.Progress(n)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	t.Progress(n)
	return rows.Err()
}

// ErrTableMissing reports an absent (not merely empty) table. MaNGOS
// exits on missing tables; loaders that may legitimately be absent
// register with SkipMissing.
var ErrTableMissing = errors.New("table missing")
