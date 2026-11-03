package app

import (
	"context"
	"fmt"
	"io"

	"net"

	"wowserver/internal/config"
	"wowserver/internal/protocol/auth/tbc"
	store "wowserver/internal/storage/realmd"
	"wowserver/internal/transport"
)

func RunRealmd(ctx context.Context, c config.RealmdConfig, logOutput io.Writer) error {
	logger := logger(c.LogLevel, logOutput)
	logger.Info("realmd starting", "log_level", c.LogLevel)
	logger.Debug("connecting to authentication database")
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
	logger.Info("realmd listening", "address", ln.Addr())
	s := transport.Server{MaxConnections: c.MaxConnections, SessionTimeout: c.AuthTimeout, Logger: logger,
		Handle: (tbc.Handler{Store: db, Realms: db, Sessions: db, Logger: logger,
			IdleTimeout: c.IdleTimeout, RequestTimeout: c.RequestTimeout, OnAuthenticated: transport.CompleteHandshake}).Serve,
	}
	return s.Serve(ctx, ln)
}
