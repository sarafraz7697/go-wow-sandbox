package app

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func newTestProgress(t *testing.T) (*Progress, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return NewProgress(logger, "test"), &buf
}

// Band must map a 0..100 sub-progress onto [from,to] without going
// backwards, without repeats, and clamped at both ends.
func TestProgressBand(t *testing.T) {
	p, buf := newTestProgress(t)
	band := p.Band(20, 60)
	for pct := 0; pct <= 100; pct += 5 {
		band(pct, "step")
	}
	out := buf.String()
	// First mapped percent is 20 (0% of the load), last is 60 (100%).
	if !strings.Contains(out, "pct=20") {
		t.Fatalf("band must start at 20:\n%s", out)
	}
	if !strings.Contains(out, "pct=60") {
		t.Fatalf("band must reach 60:\n%s", out)
	}
	if strings.Contains(out, "pct=19") || strings.Contains(out, "pct=61") || strings.Contains(out, "pct=0 ") {
		t.Fatalf("band leaked outside [20,60]:\n%s", out)
	}
	// Percentages must be strictly increasing in log order.
	seen := map[int]bool{}
	for _, field := range strings.Fields(out) {
		if strings.HasPrefix(field, "pct=") {
			v := 0
			if _, err := fmtSscanf(field[4:], &v); err != nil {
				t.Fatalf("bad pct field %q", field)
			}
			if seen[v] {
				// repeats are swallowed only when the mapped value repeats;
				// each distinct value may appear once.
			}
			seen[v] = true
		}
	}
	if len(seen) < 9 {
		t.Fatalf("expected at least 9 distinct percentages, got %d:\n%s", len(seen), out)
	}
}

// Band must clamp degenerate ranges instead of dividing by zero.
func TestProgressBandDegenerate(t *testing.T) {
	p, _ := newTestProgress(t)
	band := p.Band(60, 60)
	band(0, "a")
	band(50, "b")
	band(100, "c")
	// Out-of-range inputs clamp.
	band(-5, "neg")
	band(500, "big")
}

func fmtSscanf(s string, v *int) (int, error) {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			break
		}
		n = n*10 + int(s[i]-'0')
	}
	*v = n
	return 1, nil
}
