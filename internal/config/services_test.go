package config

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestRealmdDoesNotValidateWorldOnlySettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realmd.conf")
	if err := os.WriteFile(path, []byte("[data]\ndbc_path=''\ndbc_locale=99\nload_timeout=0s\n[world]\nrealm_id=0\nsend_queue=0\n[database.characters]\nname=''\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRealmd([]string{"-config", path}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseWorld([]string{"-config", path}, io.Discard); err == nil {
		t.Fatal("world accepted invalid settings")
	}
}
