package app

import (
	"fmt"
	"time"

	"log/slog"
)

// Progress tracks startup loading stages and logs a percentage with the
// step name. It is used by worldd to make the loading sequence transparent
// — as more subsystems are added, the startup log shows exactly which step
// is running and how far along the load is.
//
// Usage:
//
//	p := NewProgress(log, "worldd startup")
//	p.Step(10, "connecting to auth database")
//	... do work ...
//	p.Step(25, "connecting to world database")
//	... do work ...
//	p.Done("startup complete")
type Progress struct {
	log   *slog.Logger
	name  string
	total int
	start time.Time
}

// NewProgress creates a Progress tracker. The name is logged at the start.
func NewProgress(log *slog.Logger, name string) *Progress {
	p := &Progress{log: log, name: name, start: time.Now()}
	log.Info("loading started", "stage", name)
	return p
}

// Step logs a progress update with the given percentage (0-100) and step
// name. The percentage is clamped to [0, 100].
func (p *Progress) Step(pct int, step string) {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	p.total = pct
	p.log.Info("loading progress", "stage", p.name, "step", step, "pct", pct, "elapsed_ms", time.Since(p.start).Milliseconds())
}

// Stepf is like Step but takes a format string.
func (p *Progress) Stepf(pct int, format string, args ...any) {
	p.Step(pct, fmt.Sprintf(format, args...))
}

// Band returns a callback that maps a 0..100 sub-progress onto the
// [from, to] percent band of the overall startup and reports it as live
// steps. The worldd startup uses it so the world-database load visibly
// advances through its band instead of hanging on one line:
//
//	p.Band(20, 60)(pct, "creature_template 45% (23451/52134 rows)")
//
// The step text is passed through unchanged; only the overall percent is
// remapped. Repeat percentages are swallowed so the log stays readable.
func (p *Progress) Band(from, to int) func(pct int, step string) {
	if from < 0 {
		from = 0
	}
	if to > 100 {
		to = 100
	}
	if to <= from {
		to = from + 1
	}
	last := -1
	return func(pct int, step string) {
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		overall := from + (to-from)*pct/100
		if overall == last {
			return
		}
		last = overall
		p.log.Info("loading progress", "stage", p.name, "step", step, "pct", overall, "elapsed_ms", time.Since(p.start).Milliseconds())
	}
}

// Done logs 100% completion with the elapsed time.
func (p *Progress) Done(msg string) {
	p.log.Info("loading complete", "stage", p.name, "step", msg, "pct", 100, "elapsed_ms", time.Since(p.start).Milliseconds())
}
