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
	creationTBC "wowserver/internal/character/tbc"
	"wowserver/internal/config"
	"wowserver/internal/gamedata"
	dataTBC "wowserver/internal/gamedata/tbc"
	"wowserver/internal/protocol/world/tbc"
	characterstore "wowserver/internal/storage/characters"
	store "wowserver/internal/storage/realmd"
	worldstore "wowserver/internal/storage/world"
	"wowserver/internal/transport"
)

func run() error {
	c, err := config.ParseWorld(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	var level slog.Level
	_ = level.UnmarshalText([]byte(c.LogLevel))
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	log.Info("worldd starting", "phase", 8, "realm_id", c.RealmID, "log_level", c.LogLevel)
	db, err := store.Open(ctx, c.DSN())
	if err != nil {
		return err
	}
	defer db.DB.Close()
	checkCtx, end := context.WithTimeout(ctx, c.RequestTimeout)
	err = db.Check(checkCtx)
	if err == nil {
		err = db.CheckWorldRealm(checkCtx, c.RealmID)
	}
	end()
	if err != nil {
		return err
	}
	data, err := loadStatic(ctx, c, log)
	if err != nil {
		return err
	}
	chars, err := characterstore.Open(ctx, c.CharacterDSN())
	if err != nil {
		return err
	}
	defer chars.DB.Close()
	charCtx, charCancel := context.WithTimeout(ctx, c.RequestTimeout)
	err = chars.CheckEnumeration(charCtx)
	charCancel()
	if err != nil {
		return err
	}
	log.Info("character database connected", "realm_id", c.RealmID)
	creationCtx, creationCancel := context.WithTimeout(ctx, c.DataLoadTimeout)
	profile, err := creationTBC.Load(creationCtx, c.DBCPath, data)
	if err != nil {
		creationCancel()
		return fmt.Errorf("creation data: %w", err)
	}
	creator, err := chars.NewCreator(creationCtx, c.CharacterDSN(), c.DSN(), c.RealmID)
	creationCancel()
	if err != nil {
		return fmt.Errorf("creation storage: %w", err)
	}
	ln, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return fmt.Errorf("world listen: %w", err)
	}
	log.Info("worldd listening", "address", ln.Addr(), "realm_id", c.RealmID, "characters", "list and create Human Warrior (phase 8)")
	h := &tbc.Handler{Store: db, Characters: chars, Data: data, Logger: log, RealmID: c.RealmID, SendQueue: c.SendQueue, IdleTimeout: c.IdleTimeout, RequestTimeout: c.RequestTimeout, OnAuthenticated: transport.CompleteHandshake}
	h.Creator, h.Creation = creator, profile
	creator.Logger = log
	server := transport.Server{MaxConnections: c.MaxConnections, SessionTimeout: c.AuthTimeout, Logger: log, Handle: h.Serve}
	return server.Serve(ctx, ln)
}
func loadStatic(ctx context.Context, c config.Config, log *slog.Logger) (*gamedata.Catalog, error) {
	ctx, cancel := context.WithTimeout(ctx, c.DataLoadTimeout)
	defer cancel()
	log.Info("loading static data", "dbc_path", c.DBCPath, "dbc_locale", c.DBCLocale)
	db, err := worldstore.Open(ctx, c.WorldDSN())
	if err != nil {
		return nil, err
	}
	defer db.DB.Close()
	data, err := dataTBC.Load(ctx, c.DBCPath, c.DBCLocale, db)
	if err != nil {
		return nil, fmt.Errorf("static data: %w", err)
	}
	s := data.Summary()
	if s.SkippedZeroItems > 0 {
		log.Warn("ignored item_template entry zero", "rows", s.SkippedZeroItems)
	}
	log.Info("static data loaded", "races", s.Races, "classes", s.Classes, "maps", s.Maps, "creation_pairs", s.Pairs, "outfits", s.Outfits, "spells", s.Spells, "items", s.Items, "start_items", s.StartItems, "start_spells", s.StartSpells, "actions", s.Actions, "levels", s.Levels, "class_levels", s.ClassLevels, "xp_levels", s.XP, "reserved_names", s.Reserved)
	return data, nil
}
func main() {
	if err := run(); err != nil {
		slog.Error("worldd stopped", "error", err)
		os.Exit(1)
	}
}
