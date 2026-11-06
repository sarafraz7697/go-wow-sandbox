package app

import (
	"fmt"
	"log/slog"
)

// warningCollector implements worldload.Logger. It keeps the loaders'
// messages instead of printing them, because the loading display owns the
// console while tables stream.
type warningCollector struct {
	infos, warns []string
}

func (w *warningCollector) Info(msg string, args ...any) {
	w.infos = append(w.infos, format(msg, args))
}

func (w *warningCollector) Warn(msg string, args ...any) {
	w.warns = append(w.warns, format(msg, args))
}

func format(msg string, args []any) string {
	for i := 0; i+1 < len(args); i += 2 {
		msg += fmt.Sprintf(" %v=%v", args[i], args[i+1])
	}
	return msg
}

// flush logs one summary warning (details at debug level) and the notes.
func (w *warningCollector) flush(log *slog.Logger) {
	for _, m := range w.infos {
		log.Info(m)
	}
	if len(w.warns) == 0 {
		return
	}
	for _, m := range w.warns {
		log.Debug(m)
	}
	log.Warn("world data validation dropped rows", "messages", len(w.warns), "details", "set logging.level=debug")
}
