package app

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// logger builds the compact console logger used by realmd and worldd.
func logger(levelName string, output io.Writer, colors ...string) *slog.Logger {
	l, _ := loggerWithConsole(levelName, output, colors...)
	return l
}

// loggerWithConsole also returns the Console so worldd can print its
// loading display through the same writer as the log records.
func loggerWithConsole(levelName string, output io.Writer, colors ...string) (*slog.Logger, *Console) {
	var level slog.Level
	_ = level.UnmarshalText([]byte(levelName))
	mode := "auto"
	if len(colors) > 0 && colors[0] != "" {
		mode = colors[0]
	}
	tty := isTerminal(output)
	enabled := mode == "always"
	if mode == "auto" && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb" {
		enabled = tty
	}
	if enabled && tty && !enableVirtualTerminal(output) {
		// A console that cannot interpret ANSI escapes would print them as
		// "?[32m" garbage; fall back to plain text instead.
		enabled = false
	}
	console := newConsole(output, tty)
	return slog.New(&compactHandler{mu: &sync.Mutex{}, out: console, level: level, color: enabled}), console
}

func isTerminal(output io.Writer) bool {
	if file, ok := output.(*os.File); ok {
		if info, err := file.Stat(); err == nil {
			return info.Mode()&os.ModeCharDevice != 0
		}
	}
	return false
}

// compactHandler writes one short line per record:
//
//	17:42:05 INF worldd listening address=127.0.0.1:8085 realm_id=1
//
// Only ASCII is emitted, so the output is identical on every console code
// page.
type compactHandler struct {
	mu    *sync.Mutex
	out   io.Writer
	level slog.Leveler
	color bool
	attrs []slog.Attr
	group string
}

func (h *compactHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func (h *compactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.attrs = append(append([]slog.Attr(nil), h.attrs...), qualify(h.group, attrs)...)
	return &c
}

func (h *compactHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	c.group = h.group + name + "."
	return &c
}

func qualify(prefix string, attrs []slog.Attr) []slog.Attr {
	if prefix == "" {
		return attrs
	}
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		a.Key = prefix + a.Key
		out[i] = a
	}
	return out
}

func (h *compactHandler) Handle(_ context.Context, r slog.Record) error {
	t := r.Time
	if t.IsZero() {
		t = time.Now()
	}
	var b bytes.Buffer
	b.Grow(128)
	b.WriteString(h.paint("90", t.Format("15:04:05")))
	b.WriteByte(' ')
	b.WriteString(h.paint(levelColor(r.Level), levelTag(r.Level)))
	b.WriteByte(' ')
	b.WriteString(r.Message)
	for _, a := range h.attrs {
		writeAttr(&b, "", a)
	}
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(&b, h.group, a)
		return true
	})
	b.WriteByte('\n')
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.out.Write(b.Bytes())
	return err
}

func (h *compactHandler) paint(code, s string) string {
	if !h.color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func levelTag(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "ERR"
	case l >= slog.LevelWarn:
		return "WRN"
	case l >= slog.LevelInfo:
		return "INF"
	}
	return "DBG"
}

func levelColor(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "31"
	case l >= slog.LevelWarn:
		return "33"
	case l >= slog.LevelInfo:
		return "32"
	}
	return "36"
}

func writeAttr(b *bytes.Buffer, prefix string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	if a.Value.Kind() == slog.KindGroup {
		for _, g := range a.Value.Group() {
			writeAttr(b, prefix+a.Key+".", g)
		}
		return
	}
	b.WriteByte(' ')
	b.WriteString(prefix + a.Key)
	b.WriteByte('=')
	var s string
	switch a.Value.Kind() {
	case slog.KindTime:
		s = a.Value.Time().Format("15:04:05")
	case slog.KindDuration:
		s = a.Value.Duration().String()
	default:
		s = a.Value.String()
	}
	if s == "" || strings.ContainsAny(s, " \t\r\n\"=") {
		s = strconv.Quote(s)
	}
	b.WriteString(s)
}
