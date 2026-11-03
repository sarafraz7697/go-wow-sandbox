package tbc

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wowserver/internal/storage/world"
)

// Explicit opt-in, SELECT-only against the existing populated world database.
func TestExistingStaticData(t *testing.T) {
	dsn := os.Getenv("WOW_STATIC_TEST_DSN")
	if dsn == "" {
		t.Skip("set WOW_STATIC_TEST_DSN for read-only integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, err := world.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.DB.Close()
	data, err := Load(ctx, filepath.Join("..", "..", "..", "dbc"), 0, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("loaded: %+v", data.Summary())
	for _, pair := range [][2]uint32{{1, 1}, {2, 7}, {10, 2}, {11, 7}} {
		if _, ok := data.Start(pair[0], pair[1]); !ok {
			t.Fatalf("missing start %v", pair)
		}
		for _, level := range []uint32{1, 70} {
			if _, ok := data.Level(pair[0], pair[1], level); !ok {
				t.Fatalf("missing level %v/%d", pair, level)
			}
		}
	}
}
