package sqlutil

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	driver "github.com/go-sql-driver/mysql"
)

func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, errors.New("invalid database connection settings")
	}

	cfg, err := driver.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid database connection settings: %w", err)
	}

	// IMPORTANT:
	// Do not override Timeout, ReadTimeout or WriteTimeout here.
	// These values are configured by Database.DSN().
	//
	// In particular, ReadTimeout must not be reduced to 5 seconds:
	// worldload streams large MySQL result sets.

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("cannot initialize database: %w", err)
	}

	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(3 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("cannot connect to database: %w", err)
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
