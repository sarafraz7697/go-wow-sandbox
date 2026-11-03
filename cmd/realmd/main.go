package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"

	"wowserver/internal/config"
	"wowserver/internal/protocol/auth/tbc"
	store "wowserver/internal/storage/realmd"
	"wowserver/internal/transport"
)

func run() error {
	c, err := config.Parse(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	var level slog.Level
	_ = level.UnmarshalText([]byte(c.LogLevel))
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	logger.Info("realmd starting", "log_level", c.LogLevel)
	logger.Debug("connecting to authentication database")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	db, err := store.Open(ctx, c.DSN())
	if err != nil {
		return err
	}
	defer db.DB.Close()
	if err = db.Check(ctx); err != nil {
		return err
	}
	logger.Info("authentication database connected")
	ln, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	logger.Info("realmd listening", "address", ln.Addr(), "phase", 3)
	s := transport.Server{MaxConnections: c.MaxConnections, SessionTimeout: c.AuthTimeout, Logger: logger,
		Handle: (tbc.Handler{Store: db, Realms: db, Sessions: db, Logger: logger,
			IdleTimeout: c.IdleTimeout, RequestTimeout: c.RequestTimeout, OnAuthenticated: transport.CompleteHandshake}).Serve,
	}
	return s.Serve(ctx, ln)
}

func main() {
	if err := run(); err != nil {
		slog.Error("realmd stopped", "error", err)
		os.Exit(1)
	}
}
