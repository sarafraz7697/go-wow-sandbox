package world

import (
	"context"
	"database/sql"
	"wowserver/internal/storage/sqlutil"
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
	return sqlutil.Check(ctx, s.DB, map[string]string{"item_template": "SELECT entry,displayid FROM item_template", "playercreateinfo": "SELECT race,class,map,zone FROM playercreateinfo"})
}
