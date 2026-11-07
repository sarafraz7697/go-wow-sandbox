package worldload

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
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
	// pageWorkers > 1 lets scanTable fetch big tables in parallel pages.
	pageWorkers int

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
	if t.pageWorkers > 1 && t.rowsTotal >= pagedThreshold && pageable(query) {
		return scanPaged(ctx, q, t, query, progressEvery, scan)
	}
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

const (
	pagedThreshold = 20000 // tables smaller than this are streamed in one query
	pageRows       = 10000
)

// pageable reports whether query is safe to split with LIMIT/OFFSET: it
// must have a deterministic ORDER BY and no LIMIT of its own.
func pageable(query string) bool {
	u := strings.ToUpper(query)
	return strings.Contains(u, " ORDER BY ") && !strings.Contains(u, " LIMIT ")
}

type pageResult struct {
	rows [][]any
	err  error
}

// replayScanner hands one buffered row to a loader callback exactly as
// *sql.Rows would; every loader scans through the nullable wrappers, which
// implement sql.Scanner.
type replayScanner struct{ vals []any }

func (r replayScanner) Scan(dest ...any) error {
	if len(dest) != len(r.vals) {
		return fmt.Errorf("scan: have %d columns, want %d", len(r.vals), len(dest))
	}
	for i, d := range dest {
		s, ok := d.(sql.Scanner)
		if !ok {
			return fmt.Errorf("scan: destination %d (%T) is not a sql.Scanner", i, d)
		}
		if err := s.Scan(r.vals[i]); err != nil {
			return err
		}
	}
	return nil
}

// scanPaged fetches the table in LIMIT/OFFSET pages concurrently and feeds
// the rows to scan in their original order from this goroutine, so loaders
// (which mutate plain maps) stay single-threaded.
func scanPaged(ctx context.Context, q Queryer, t *Table, query string, progressEvery int, scan func(r scanner) error) error {
	pages := int((t.rowsTotal + pageRows - 1) / pageRows)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := make([]chan pageResult, pages)
	for i := range out {
		out[i] = make(chan pageResult, 1)
	}
	sem := make(chan struct{}, t.pageWorkers)
	go func() {
		for i := 0; i < pages; i++ {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			go func(i int) {
				defer func() { <-sem }()
				limit := fmt.Sprintf("%d", pageRows)
				if i == pages-1 {
					limit = "18446744073709551615" // last page is open-ended if the count was stale
				}
				out[i] <- fetchPage(ctx, q, fmt.Sprintf("%s LIMIT %s OFFSET %d", query, limit, i*pageRows))
			}(i)
		}
	}()
	if progressEvery <= 0 {
		progressEvery = 1000
	}
	n := 0
	for i := 0; i < pages; i++ {
		var res pageResult
		select {
		case res = <-out[i]:
		case <-ctx.Done():
			return ctx.Err()
		}
		if res.err != nil {
			return res.err
		}
		for _, vals := range res.rows {
			if err := scan(replayScanner{vals}); err != nil {
				return err
			}
			n++
			if n%progressEvery == 0 {
				t.Progress(n)
			}
		}
		out[i] = nil
	}
	t.Progress(n)
	return ctx.Err()
}

func fetchPage(ctx context.Context, q Queryer, query string) pageResult {
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return pageResult{err: err}
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return pageResult{err: err}
	}
	var res pageResult
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return pageResult{err: err}
		}
		res.rows = append(res.rows, vals)
	}
	res.err = rows.Err()
	return res
}
