package app

// PACKAGE_SKELETON_PATCH

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"

	charcreate "wowserver/internal/application/character/create"
	creationTBC "wowserver/internal/application/character/create/tbc"
	chardelete "wowserver/internal/application/character/delete"
	charlist "wowserver/internal/application/character/list"
	worldlogin "wowserver/internal/application/worldlogin"
	"wowserver/internal/config"
	"wowserver/internal/domain/character"
	"wowserver/internal/gamedata"
	dataTBC "wowserver/internal/gamedata/tbc"
	"wowserver/internal/protocol/world/tbc"
	characterstore "wowserver/internal/storage/characters"
	store "wowserver/internal/storage/realmd"
	worldstore "wowserver/internal/storage/world"
	"wowserver/internal/transport"
	worldsim "wowserver/internal/world"
	"wowserver/internal/worldload"
)

func RunWorldd(ctx context.Context, c config.WorldConfig, logOutput io.Writer) error {
	log, console := loggerWithConsole(c.LogLevel, logOutput, c.LogColor)
	// This composition root deliberately binds protocol, DBC and native persistence to 8606.
	log.Info("worldd starting", "build", 8606, "persistence", "mangosone8606", "realm_id", c.RealmID, "log_level", c.LogLevel)

	p := NewProgress(console, "worldd startup")

	p.Step(5, "Connecting to auth database")
	db, err := store.Open(ctx, c.DSN())
	if err != nil {
		return err
	}
	defer db.DB.Close()

	p.Step(10, "Checking auth database schema")
	checkCtx, end := context.WithTimeout(ctx, c.RequestTimeout)
	err = db.Check(checkCtx)
	if err == nil {
		err = db.CheckWorldRealm(checkCtx, c.RealmID)
	}
	end()
	if err != nil {
		return err
	}

	p.Step(20, "Loading static data (DBC + world DB)")
	data, err := loadStatic(ctx, c, log, p)
	if err != nil {
		return err
	}

	p.Step(60, "Connecting to character database")
	chars, err := characterstore.Open(ctx, c.CharacterDSN())
	if err != nil {
		return err
	}
	defer chars.DB.Close()
	chars.SetIdleTimeout(c.IdleTimeout)

	p.Step(65, "Checking character database schema")
	charCtx, charCancel := context.WithTimeout(ctx, c.RequestTimeout)
	err = chars.CheckEnumeration(charCtx)
	if err == nil {
		err = chars.CheckPlayer(charCtx)
	}
	charCancel()
	if err != nil {
		return err
	}
	log.Info("character database connected", "realm_id", c.RealmID)

	p.Step(70, "Loading character creation profile (DBC)")
	creationCtx, creationCancel := context.WithTimeout(ctx, c.DataLoadTimeout)
	profile, err := creationTBC.Load(creationCtx, c.DBCPath, data)
	if err != nil {
		creationCancel()
		return fmt.Errorf("creation data: %w", err)
	}
	creationPolicy := character.DefaultCreationPolicy()
	creationPolicy.AllowTwoSideAccounts = c.AllowTwoSideAccounts
	creator, err := chars.NewCreator(creationCtx, c.CharacterDSN(), c.DSN(), c.RealmID, creationPolicy)
	creationCancel()
	if err != nil {
		return fmt.Errorf("creation storage: %w", err)
	}

	p.Step(85, "Opening world listener")
	ln, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return fmt.Errorf("world listen: %w", err)
	}

	p.Step(90, "Initializing world simulation")
	// The World is the per-realm simulation registry shared by all
	// connections. It routes broadcast packets between in-world players.
	sim := worldsim.New()
	h := &tbc.Handler{Store: db, Characters: charlist.NewService(chars), Data: data, Broadcast: sim, Logger: log, RealmID: c.RealmID, SendQueue: c.SendQueue, IdleTimeout: c.IdleTimeout, RequestTimeout: c.RequestTimeout, OnAuthenticated: transport.CompleteHandshake}
	h.Creator = charcreate.NewService(profile, creator)
	h.Deleter = chardelete.NewService(chars)
	h.Players = worldlogin.NewService(chars)
	creator.Logger = log

	p.Done("worldd ready, accepting connections")
	log.Info("worldd listening", "address", ln.Addr(), "realm_id", c.RealmID, "characters", "list, create and enter world", "movement", "local + broadcast")
	server := transport.Server{MaxConnections: c.MaxConnections, SessionTimeout: c.AuthTimeout, Logger: log, Handle: h.Serve}
	return server.Serve(ctx, ln)
}
func loadStatic(ctx context.Context, c config.WorldConfig, log *slog.Logger, p *Progress) (*gamedata.Catalog, error) {
	ctx, cancel := context.WithTimeout(ctx, c.DataLoadTimeout)
	defer cancel()
	// Table loading is shown by the console ("12% > Loading creature_template..."
	// plus an in-place done/total counter); validation warnings are collected
	// quietly and summarised afterwards so they never interleave with it.
	warnings := &warningCollector{}
	db, err := worldstore.Open(ctx, c.WorldDSN())
	if err != nil {
		return nil, err
	}
	defer db.DB.Close()
	db.Log = warnings
	const bandFrom, bandTo = 20, 60 // share of the startup bar used by the world load
	db.OnTable = func(tp worldload.TableProgress) {
		pct := bandFrom + (bandTo-bandFrom)*tp.Overall/100
		switch {
		case tp.Finished:
			p.ui.TableDone(tp.Done, tp.Total, tp.Skipped)
		case tp.Done == 0:
			p.ui.TableStart(pct, tp.Name)
		default:
			p.ui.TableTick(tp.Done, tp.Total)
		}
	}
	data, err := dataTBC.LoadWithProgress(ctx, c.DBCPath, c.DBCLocale, db, nil)
	if err != nil {
		return nil, fmt.Errorf("static data: %w", err)
	}
	warnings.flush(log)
	s := data.Summary()
	if s.SkippedZeroItems > 0 {
		log.Warn("ignored item_template entry zero", "rows", s.SkippedZeroItems)
	}
	log.Info("dbc data loaded",
		"races", s.Races, "classes", s.Classes, "maps", s.Maps, "creation_pairs", s.Pairs,
		"outfits", s.Outfits, "spells", s.Spells, "levels", s.Levels, "class_levels", s.ClassLevels,
		"xp_levels", s.XP)
	log.Info("world data loaded",
		"items", s.Items, "creature_templates", s.CreatureTemplates, "go_templates", s.GameObjectTemplates,
		"quests", s.Quests, "creature_spawns", data.CreatureSpawnCount(), "go_spawns", data.GameObjectSpawnCount())
	return data, nil
}
