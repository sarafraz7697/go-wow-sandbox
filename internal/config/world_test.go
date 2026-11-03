package config

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWorldConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worldd.conf")
	if err := os.WriteFile(path, []byte("[world]\nrealm_id=7\nsend_queue=16\n[network]\nlisten=127.0.0.1:8088\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := ParseWorld([]string{"-config", path}, io.Discard)
	if err != nil || c.RealmID != 7 || c.SendQueue != 16 || c.Listen != "127.0.0.1:8088" {
		t.Fatal("world config", err)
	}
	for _, content := range []string{"[world]\nrealm_id=0", "[world]\nrealm_id=4294967296", "[world]\nsend_queue=0", "[world]\nsend_queue=1025"} {
		if err = os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = ParseWorld([]string{"-config", path}, io.Discard); err == nil {
			t.Fatal("invalid world config accepted")
		}
	}
}
