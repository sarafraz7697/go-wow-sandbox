package sqlutil

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	driver "github.com/go-sql-driver/mysql"
	"time"
)

func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	cfg, err := driver.ParseDSN(dsn)
	if err != nil || dsn == "" {
		return nil, errors.New("invalid database connection settings")
	}
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 5 * time.Second
	cfg.WriteTimeout = 5 * time.Second
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, errors.New("cannot initialize database")
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(3 * time.Minute)
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, errors.New("cannot connect to database; check address and credentials")
	}
	return db, nil
}

// Check only prepares zero-row reads. Schema is owned by the imported MaNGOS DB.
func Check(ctx context.Context, db *sql.DB, queries map[string]string) error {
	for table, query := range queries {
		rows, err := db.QueryContext(ctx, query+" LIMIT 0")
		if err != nil {
			return fmt.Errorf("incompatible or inaccessible table %s", table)
		}
		rows.Close()
	}
	return nil
}
