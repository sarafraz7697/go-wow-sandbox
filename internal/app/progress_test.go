package app

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProgressStepsAreClampedAndCompact(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(newConsole(&buf, false), "test")
	p.Step(-5, "Zero")
	p.Step(5, "Connecting")
	p.Step(500, "Big")
	p.Done("ready")
	want := "  0% > Zero...\n  5% > Connecting...\n100% > Big...\n100% > ready...\n"
	if buf.String() != want {
		t.Fatalf("got %q want %q", buf.String(), want)
	}
}

func TestConsoleTableLinesOnTerminalAndPipe(t *testing.T) {
	var pipe bytes.Buffer
	c := newConsole(&pipe, false)
	c.TableStart(21, "creature_template")
	c.TableTick(1000, 20000) // ignored when not a terminal
	c.TableDone(20000, 20000, 3)
	want := " 21% > Loading creature_template...\n 20000/20000 (3 skipped)\n"
	if pipe.String() != want {
		t.Fatalf("pipe got %q want %q", pipe.String(), want)
	}

	var tty bytes.Buffer
	c = newConsole(&tty, true)
	c.TableStart(22, "gameobject_template")
	c.TableTick(1000, 20000)
	c.TableDone(20000, 20000, 0)
	out := tty.String()
	if !strings.Contains(out, "\r 1000/20000") || !strings.HasSuffix(out, "\r 20000/20000\n") {
		t.Fatalf("terminal output wrong: %q", out)
	}
}

// A log record must never be glued to an in-place counter line.
func TestLogRecordClosesOpenCounterLine(t *testing.T) {
	var tty bytes.Buffer
	c := newConsole(&tty, true)
	c.TableTick(5, 10)
	_, _ = c.Write([]byte("x\n"))
	if !strings.Contains(tty.String(), " 5/10\nx\n") {
		t.Fatalf("got %q", tty.String())
	}
}

func TestCompactHandlerFormat(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(&compactHandler{mu: &sync.Mutex{}, out: &buf, level: slog.LevelInfo})
	log.With("realm", 1).Info("worldd listening", "address", "127.0.0.1:8085", "note", "two words", "took", 1500*time.Millisecond)
	log.Debug("hidden")
	line := buf.String()
	if strings.Count(line, "\n") != 1 || strings.Contains(line, "hidden") {
		t.Fatalf("unexpected output %q", line)
	}
	if len(line) < 9 || line[2] != ':' || line[5] != ':' || line[8] != ' ' {
		t.Fatalf("timestamp must be HH:MM:SS, got %q", line)
	}
	for _, part := range []string{" INF worldd listening", " realm=1", " address=127.0.0.1:8085", ` note="two words"`, " took=1.5s"} {
		if !strings.Contains(line, part) {
			t.Fatalf("missing %q in %q", part, line)
		}
	}
	if strings.Contains(line, "\x1b") || strings.Contains(line, "time=") || strings.Contains(line, "level=") {
		t.Fatalf("plain output must not contain escapes or slog keys: %q", line)
	}
}

func TestLoggerColorNeverIsPlain(t *testing.T) {
	var buf bytes.Buffer
	logger("info", &buf, "never").Warn("careful")
	if strings.Contains(buf.String(), "\x1b") || !strings.Contains(buf.String(), " WRN careful") {
		t.Fatalf("got %q", buf.String())
	}
}
