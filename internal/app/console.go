package app

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// Console is the single writer behind both the compact log handler and the
// startup loading display. Sharing one writer (and one mutex) guarantees a
// log record never lands in the middle of an in-place "done/total" counter.
type Console struct {
	mu    sync.Mutex
	out   io.Writer
	tty   bool // in-place counters (carriage return) are only used on a terminal
	open  bool // an in-place counter line is currently on screen
	width int  // visible width of that line, to blank leftovers
}

func newConsole(out io.Writer, tty bool) *Console { return &Console{out: out, tty: tty} }

// Write implements io.Writer: it first terminates an open counter line.
func (c *Console) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLine()
	return c.out.Write(p)
}

func (c *Console) closeLine() {
	if c.open {
		_, _ = io.WriteString(c.out, "\n")
		c.open, c.width = false, 0
	}
}

// Step prints "  5% > Connecting to auth database...".
func (c *Console) Step(pct int, step string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLine()
	fmt.Fprintf(c.out, "%3d%% > %s...\n", pct, step)
}

// TableStart prints the header of one table load:
//
//	21% > Loading creature_template...
func (c *Console) TableStart(pct int, table string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLine()
	fmt.Fprintf(c.out, "%3d%% > Loading %s...\n", pct, table)
}

// TableTick refreshes the " done/total" line below the header in place. It
// does nothing when the output is not a terminal (the final line from
// TableDone is still printed).
func (c *Console) TableTick(done, total int64) {
	if !c.tty {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rewrite(fmt.Sprintf(" %d/%d", done, total))
}

// TableDone prints the final counter line and moves to the next line.
func (c *Console) TableDone(done, total, skipped int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	line := fmt.Sprintf(" %d/%d", done, total)
	if skipped > 0 {
		line += fmt.Sprintf(" (%d skipped)", skipped)
	}
	if c.tty {
		c.rewrite(line)
		c.closeLine()
		return
	}
	c.closeLine()
	fmt.Fprintln(c.out, line)
}

func (c *Console) rewrite(line string) {
	pad := ""
	if c.width > len(line) {
		pad = strings.Repeat(" ", c.width-len(line))
	}
	fmt.Fprintf(c.out, "\r%s%s", line, pad)
	c.open, c.width = true, len(line)
}
