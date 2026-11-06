package app

import "time"

// Progress prints the startup sequence as one clean line per step:
//
//	  5% > Connecting to auth database...
//	 20% > Loading static data (DBC + world DB)...
//
// Table-level loading lines come from Console.TableStart/TableDone and use
// the same percentage scale, so the output stays monotonic.
type Progress struct {
	ui    *Console
	name  string
	start time.Time
}

// NewProgress creates a Progress tracker that prints through ui.
func NewProgress(ui *Console, name string) *Progress {
	return &Progress{ui: ui, name: name, start: time.Now()}
}

// Step prints a progress line; pct is clamped to [0, 100].
func (p *Progress) Step(pct int, step string) {
	p.ui.Step(clampPct(pct), step)
}

// Done prints the 100% line.
func (p *Progress) Done(msg string) {
	p.ui.Step(100, msg)
}

// Elapsed returns the time since the tracker was created.
func (p *Progress) Elapsed() time.Duration { return time.Since(p.start) }

func clampPct(pct int) int {
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}
