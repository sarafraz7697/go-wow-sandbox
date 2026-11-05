package worldload

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
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
	// MaxWarningsPerTable caps the per-row validation warnings logged per
	// table, like a human-readable outErrorDb tail. Default 5.
	MaxWarningsPerTable int
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

	// Phase 2: stream every table in order.
	var cumulative int64
	for i, l := range loaders {
		if totals[i] < 0 {
			report.Tables = append(report.Tables, TableReport{Name: l.Name, Absent: true})
			continue
		}
		started := time.Now()

		// Live-progress plumbing: at most one callback per
		// 2000 rows / 250 ms while a table streams.
		var lastTick time.Time
		var lastRows int
		rowsDone := func(n int) {
			if opts.Progress == nil || n == 0 {
				return
			}
			now := time.Now()
			if n-lastRows < 2000 && now.Sub(lastTick) < 250*time.Millisecond {
				return
			}
			lastTick, lastRows = now, n
			tablePct := 100
			if totals[i] > 0 {
				tablePct = int(float64(n) / float64(totals[i]) * 100)
				if tablePct > 100 {
					tablePct = 100
				}
			}
			overall := int(float64(cumulative+min64(int64(n), totals[i])) / float64(grandTotal) * 100)
			step := fmt.Sprintf("%s %d%% (%d/%d rows)", l.Name, tablePct, n, totals[i])
			opts.Progress(clamp(overall), step)
		}

		tbl := &Table{
			Name:       l.Name,
			log:        opts.Logger,
			progress:   rowsDone,
			rowsTotal:  totals[i],
			warnBudget: opts.maxWarnings(),
		}
		err := l.Load(ctx, q, dest, tbl)

		tr := TableReport{Name: l.Name, TookMS: time.Since(started).Milliseconds()}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				tr.Err = err
				report.Tables = append(report.Tables, tr)
				return report, err
			}
			tr.Err = err
			report.Tables = append(report.Tables, tr)
			return report, fmt.Errorf("worldload %s: %w", l.Name, err)
		}
		tr.Rows = int64(tbl.Rows())
		tr.Skipped = int64(tbl.Skipped())
		report.Tables = append(report.Tables, tr)
		cumulative += totals[i]
		if opts.Logger != nil {
			opts.Logger.Info("loaded entity", "table", l.Name, "rows", tr.Rows, "skipped", tr.Skipped, "took_ms", tr.TookMS)
		}
		if opts.Progress != nil {
			opts.Progress(clamp(int(float64(cumulative)/float64(grandTotal)*100)), fmt.Sprintf("%s complete (%d rows)", l.Name, tr.Rows))
		}
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
