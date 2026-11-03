package app

import (
	"io"
	"log/slog"
)

func logger(levelName string, output io.Writer) *slog.Logger {
	var level slog.Level
	_ = level.UnmarshalText([]byte(levelName))
	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: level}))
}
