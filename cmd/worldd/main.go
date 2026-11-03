package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"wowserver/internal/app"
	"wowserver/internal/config"
)

func run() error {
	c, err := config.ParseWorld(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	return app.RunWorldd(ctx, c, os.Stderr)
}
func main() {
	if err := run(); err != nil {
		slog.Error("worldd stopped", "error", err)
		os.Exit(1)
	}
}
