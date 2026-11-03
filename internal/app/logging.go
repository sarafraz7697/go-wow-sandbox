package app

import (
	"bytes"
	"io"
	"log/slog"
	"os"
)

func logger(levelName string, output io.Writer, colors ...string) *slog.Logger {
	var level slog.Level
	_ = level.UnmarshalText([]byte(levelName))
	mode := "auto"
	if len(colors) > 0 && colors[0] != "" {
		mode = colors[0]
	}
	enabled := mode == "always"
	if mode == "auto" && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb" {
		if file, ok := output.(*os.File); ok {
			if info, err := file.Stat(); err == nil {
				enabled = info.Mode()&os.ModeCharDevice != 0
			}
		}
	}
	if enabled {
		output = colorWriter{output}
	}
	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: level}))
}

// TextHandler serializes writes; keep each colored record in a single write.
type colorWriter struct{ output io.Writer }

func (w colorWriter) Write(p []byte) (int, error) {
	color := "\x1b[36m"
	switch {
	case bytes.Contains(p, []byte(" level=ERROR ")):
		color = "\x1b[31m"
	case bytes.Contains(p, []byte(" level=WARN ")):
		color = "\x1b[33m"
	case bytes.Contains(p, []byte(" level=INFO ")):
		color = "\x1b[32m"
	}
	line := append([]byte(color), bytes.TrimSuffix(p, []byte("\n"))...)
	line = append(line, []byte("\x1b[0m\n")...)
	n, err := w.output.Write(line)
	if err != nil {
		return 0, err
	}
	if n != len(line) {
		return 0, io.ErrShortWrite
	}
	return len(p), nil
}
