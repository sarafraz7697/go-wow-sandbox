package worldload

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// TableReport is the per-loader result collected by Run.
type TableReport struct {
	Name    string
	Rows    int64 // rows kept in the dataset
	Skipped int64 // rows dropped by validation (MaNGOS outErrorDb + continue)
	TookMS  int64 // wall time
	Absent  bool  // table missing and loader tolerates absence
	Err     error // fatal error, if any
}

// RunReport summarizes a full Run.
type RunReport struct {
	Tables []TableReport
	Start  time.Time
}

// TotalRows returns the number of rows kept across all tables.
func (r *RunReport) TotalRows() int64 {
	var n int64
	for _, t := range r.Tables {
		n += t.Rows
	}
	return n
}

// TotalSkipped returns the number of rows dropped by validation.
func (r *RunReport) TotalSkipped() int64 {
	var n int64
	for _, t := range r.Tables {
		n += t.Skipped
	}
	return n
}

// Logger is the minimal logging surface the runner needs.
type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
}

// TableProgress is one structured loading event for the console display.
type TableProgress struct {
	Name string
	// Overall is the row-weighted percentage (0..100) of the whole load at
	// the time of the event.
	Overall int
	// Done and Total are the rows streamed so far and the table's row count.
	Done, Total int64
	// Finished marks the last event of a table; Rows/Skipped are the final
	// kept/dropped counts then.
	Finished      bool
	Rows, Skipped int64
}

// RunOptions tunes Run.
type RunOptions struct {
	// Logger receives per-table completion lines and throttled live
	// progress lines. When nil, nothing is logged.
	Logger Logger
	// Progress maps the whole load onto a percent band and is called with
	// the current overall percentage (0..100 of the load itself) and a
	// human step like "creature_template 45% (23451/52134 rows)". The
	// caller can remap that 0..100 into any startup band it wants.
	// Called a few times per table per second at most, always from the
	// goroutine calling Run.
	Progress func(pct int, step string)
	// OnTable receives structured per-table events: one at table start
	// (Done 0), throttled ticks while rows stream, and one with Finished set.
	OnTable func(TableProgress)
	// MaxWarningsPerTable caps the per-row validation warnings logged per
	// table, like a human-readable outErrorDb tail. Default 5.
	MaxWarningsPerTable int
	// Parallelism bounds how many tables load at once (default 8).
	Parallelism int
	// PageWorkers is how many pages of one large table are fetched at the
	// same time (default 4; 1 disables paging).
	PageWorkers int
}

const defaultParallelism = 8

// dependencies lists loaders whose dataset output another loader validates
// against, so the second one waits for the first. Everything else is
// independent and runs concurrently.
var dependencies = map[string][]string{
	"creature":             {"creature_template"},
	"gameobject":           {"gameobject_template"},
	"quest_relations":      {"quest_template", "creature_template", "gameobject_template"},
	"npc_vendor":           {"item_template"},
	"npc_vendor_template":  {"item_template"},
}

func (o *RunOptions) maxWarnings() int {
	if o.MaxWarningsPerTable <= 0 {
		return 5
	}
	return o.MaxWarningsPerTable
}

// Run loads every registered entity family into dest, in MaNGOS order,
// reporting live progress. The count queries run up front so the overall
// percentage is row-weighted across all tables: a big creature_template
// moves the bar more than a tiny page_text, and the console never stalls
// on one silent blocking step. This is the Go shape of MaNGOS's
// BarGoLink(result->GetRowCount()) loop.
func Run(ctx context.Context, q Queryer, dest *Dataset, opts RunOptions) (*RunReport, error) {
	loaders := Loaders()
	if len(loaders) == 0 {
		return nil, fmt.Errorf("worldload: no loaders registered")
	}
	report := &RunReport{Start: time.Now(), Tables: make([]TableReport, 0, len(loaders))}

	// Phase 1: row budget (SELECT COUNT(*) per table).
	totals := make([]int64, len(loaders))
	var grandTotal int64
	for i, l := range loaders {
		n, err := countRows(ctx, q, l.CountQuery)
		if err != nil {
			if l.SkipMissing && isMissingTable(err) {
				if opts.Logger != nil {
					opts.Logger.Info("entity table absent, skipped", "table", l.Name)
				}
				totals[i] = -1
				continue
			}
			return nil, fmt.Errorf("worldload %s: %w", l.Name, err)
		}
		totals[i] = n
		grandTotal += n
	}
	if grandTotal <= 0 {
		grandTotal = 1 // avoid division by zero on an empty database
	}

	// Phase 2: stream the tables concurrently. A loader starts as soon as
	// the loaders it validates against (see dependencies) are done, so
	// independent tables overlap; each loader still fills its own dataset
	// fields from a single goroutine.
	var (
		mu         sync.Mutex // serializes callbacks and cumulative counters
		cumulative int64
		lastPct    int
	)
	results := make([]TableReport, len(loaders))
	doneCh := make(map[string]chan struct{}, len(loaders))
	for _, l := range loaders {
		doneCh[l.Name] = make(chan struct{})
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	workers := opts.Parallelism
	if workers <= 0 {
		workers = defaultParallelism
	}
	sem := make(chan struct{}, workers)
	var firstErr error
	var errOnce sync.Once
	fail := func(err error) {
		errOnce.Do(func() { firstErr = err; cancel() })
	}
	// monotonic keeps the shared percentage from going backwards when
	// several tables report at once; call with mu held.
	monotonic := func(p int) int {
		p = max(p, lastPct)
		lastPct = p
		return p
	}
	overallPct := func() int { return monotonic(clamp(int(float64(cumulative) / float64(grandTotal) * 100))) }
	var wg sync.WaitGroup
	for i, l := range loaders {
		if totals[i] < 0 {
			results[i] = TableReport{Name: l.Name, Absent: true}
			close(doneCh[l.Name])
			continue
		}
		wg.Add(1)
		go func(i int, l Loader) {
			defer wg.Done()
			defer close(doneCh[l.Name])
			for _, dep := range dependencies[l.Name] {
				if ch, ok := doneCh[dep]; ok {
					select {
					case <-ch:
					case <-runCtx.Done():
						return
					}
				}
			}
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-runCtx.Done():
				return
			}
			if runCtx.Err() != nil {
				return
			}
			started := time.Now()
			var lastTick time.Time
			var lastRows int
			rowsDone := func(n int) {
				if (opts.Progress == nil && opts.OnTable == nil) || n == 0 {
					return
				}
				now := time.Now()
				if n-lastRows < 2000 && now.Sub(lastTick) < 250*time.Millisecond {
					return
				}
				lastTick, lastRows = now, n
				tablePct := 100
				if totals[i] > 0 {
					tablePct = min(100, int(float64(n)/float64(totals[i])*100))
				}
				mu.Lock()
				defer mu.Unlock()
				overall := monotonic(clamp(int(float64(cumulative+min64(int64(n), totals[i])) / float64(grandTotal) * 100)))
				if opts.OnTable != nil {
					opts.OnTable(TableProgress{Name: l.Name, Overall: overall, Done: int64(n), Total: totals[i]})
				}
				if opts.Progress != nil {
					opts.Progress(overall, fmt.Sprintf("%s %d%% (%d/%d rows)", l.Name, tablePct, n, totals[i]))
				}
			}
			if opts.OnTable != nil {
				mu.Lock()
				opts.OnTable(TableProgress{Name: l.Name, Overall: overallPct(), Total: totals[i]})
				mu.Unlock()
			}
			tbl := &Table{
				Name:        l.Name,
				log:         opts.Logger,
				progress:    rowsDone,
				rowsTotal:   totals[i],
				warnBudget:  opts.maxWarnings(),
				pageWorkers: opts.PageWorkers,
			}
			err := l.Load(runCtx, q, dest, tbl)
			tr := TableReport{Name: l.Name, TookMS: time.Since(started).Milliseconds()}
			if err != nil {
				tr.Err = err
				results[i] = tr
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					fail(err)
				} else {
					fail(fmt.Errorf("worldload %s: %w", l.Name, err))
				}
				return
			}
			tr.Rows = int64(tbl.Rows())
			tr.Skipped = int64(tbl.Skipped())
			results[i] = tr
			mu.Lock()
			defer mu.Unlock()
			cumulative += totals[i]
			if opts.OnTable != nil {
				opts.OnTable(TableProgress{Name: l.Name, Overall: overallPct(), Done: totals[i], Total: totals[i], Finished: true, Rows: tr.Rows, Skipped: tr.Skipped})
			}
			if opts.Logger != nil {
				opts.Logger.Info("loaded entity", "table", l.Name, "rows", tr.Rows, "skipped", tr.Skipped, "took_ms", tr.TookMS)
			}
			if opts.Progress != nil {
				opts.Progress(overallPct(), fmt.Sprintf("%s complete (%d rows)", l.Name, tr.Rows))
			}
		}(i, l)
	}
	wg.Wait()
	report.Tables = results
	if firstErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return report, ctxErr
		}
		return report, firstErr
	}
	return report, nil
}

func countRows(ctx context.Context, q Queryer, query string) (int64, error) {
	var n sql.NullInt64
	if err := q.QueryRowContext(ctx, query).Scan(&n); err != nil {
		return 0, err
	}
	if !n.Valid {
		return 0, nil
	}
	return n.Int64, nil
}

// isMissingTable matches the go-sql-driver error for a nonexistent table
// (MySQL error 1146) so SkipMissing loaders can degrade gracefully.
func isMissingTable(err error) bool {
	if err == nil {
		return false
	}
	var codeErr interface{ ErrorCode() uint16 }
	if errors.As(err, &codeErr) {
		return codeErr.ErrorCode() == 1146
	}
	msg := err.Error()
	return strings.Contains(msg, "Error 1146") ||
		strings.Contains(msg, "doesn't exist") ||
		strings.Contains(msg, "Unknown table")
}

func clamp(p int) int {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
