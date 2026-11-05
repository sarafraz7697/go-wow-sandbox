package world

import (
	"context"
	"database/sql"
	"fmt"

	"wowserver/internal/entity"
	"wowserver/internal/storage/sqlutil"
	"wowserver/internal/worldload"
)

type Store struct{ DB *sql.DB }

func Open(ctx context.Context, dsn string) (*Store, error) {
	db, err := sqlutil.Open(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &Store{db}, nil
}
func (s *Store) Check(ctx context.Context) error {
	return sqlutil.Check(ctx, s.DB, map[string]string{
		"creature_template":   "SELECT `Entry` FROM `creature_template`",
		"gameobject_template": "SELECT `entry` FROM `gameobject_template`",
		"item_template":       "SELECT `entry` FROM `item_template`",
		"playercreateinfo":    "SELECT race,class,map,zone FROM `playercreateinfo`",
	})
}

// LoadWorldEntities runs every registered worldload loader against the
// world database in MaNGOS order, reporting live row-weighted progress
// through the callback. The MaNGOS tables may be MyISAM: a transaction
// cannot promise a consistent snapshot across tables, so administrators
// must not edit static data during load (same contract as the previous
// static loader). Only SELECT is used; nothing queries after startup.
func (s *Store) LoadWorldEntities(ctx context.Context, dest *entity.Dataset, progress func(pct int, step string)) error {
	opts := worldload.RunOptions{Logger: slogNoop{}}
	if progress != nil {
		opts.Progress = progress
	}
	report, err := worldload.Run(ctx, s.DB, dest, opts)
	if err != nil {
		return err
	}
	if report != nil && report.TotalRows() == 0 {
		return fmt.Errorf("world database has no entity rows; a schema-only dump is insufficient")
	}
	return nil
}

// slogNoop keeps worldload.Run's logger interface satisfied without
// coupling the store to a logger; the app layer passes its own through
// the progress callback.
type slogNoop struct{}

func (slogNoop) Info(msg string, args ...any) {}
func (slogNoop) Warn(msg string, args ...any) {}
