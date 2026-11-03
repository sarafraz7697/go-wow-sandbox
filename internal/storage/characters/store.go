package characters

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
	return sqlutil.Check(ctx, s.DB, map[string]string{"characters": "SELECT guid,account,name,race,class,gender,level FROM characters"})
}

func (s *Store) CheckEnumeration(ctx context.Context) error {
	return sqlutil.Check(ctx, s.DB, map[string]string{
		"characters":    "SELECT guid,account,name,race,class,gender,level,playerBytes,playerBytes2,zone,map,position_x,position_y,position_z,playerFlags,at_login,equipmentCache,deleteDate FROM characters",
		"guild_member":  "SELECT guid,guildid FROM guild_member",
		"character_pet": "SELECT id,owner,entry,modelid,level,slot FROM character_pet",
	})
}
