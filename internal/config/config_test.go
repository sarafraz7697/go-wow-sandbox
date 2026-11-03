package config

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mysql "github.com/go-sql-driver/mysql"
)

func TestConfiguration(t *testing.T) {
	if _, err := Parse(nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"-max-connections=0"}, {"-auth-timeout=-1s"}, {"-listen=invalid"}, {"unexpected"}, {"-unknown"}} {
		if _, err := Parse(args, io.Discard); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestFileAndOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realmd.conf")
	content := "[network]\nlisten=127.0.0.1:4000\nmax_connections=12\nauth_timeout=20s\n[database]\nhost=localhost\nport=3307\nname=example\nuser=test\npassword='p#;=@:word'\n[logging]\nlevel=warn\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WOW_AUTH_DSN", "")
	t.Setenv("WOW_MAX_CONNECTIONS", "23")
	t.Setenv("WOW_LISTEN", "127.0.0.1:5000")
	c, err := Parse([]string{"-config", path, "-listen", "127.0.0.1:6000"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "127.0.0.1:6000" || c.MaxConnections != 23 || c.LogLevel != "warn" {
		t.Fatal("wrong precedence")
	}
	d, err := mysql.ParseDSN(c.DSN())
	if err != nil {
		t.Fatal(err)
	}
	if d.Passwd != "p#;=@:word" || d.Addr != "localhost:3307" || d.DBName != "example" {
		t.Fatal("DSN did not preserve fields")
	}
	t.Setenv("WOW_AUTH_DSN", "legacy:secret@tcp(localhost:3308)/legacy")
	c, err = Parse([]string{"-config", path}, io.Discard)
	if err != nil || c.DSN() != os.Getenv("WOW_AUTH_DSN") {
		t.Fatal("legacy override", err)
	}
}

func TestInvalidFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realmd.conf")
	if _, err := Parse([]string{"-config", path}, io.Discard); err == nil {
		t.Fatal("explicit missing file accepted")
	}
	for _, content := range []string{
		"[wrong]\nx=1", "password=secret", "[database]\nport=secret",
		"[database]\npassword='secret", "[database]\npassword=secret\npassword=another",
		"[database]\nunknown=secret", "[database]\nport=70000", "[logging]\nlevel=secret",
	} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := Parse([]string{"-config", path}, io.Discard)
		if err == nil {
			t.Fatal("invalid config accepted")
		}
		if strings.Contains(err.Error(), "secret") {
			t.Fatal("error exposed value")
		}
	}
}

func TestThreeDatabaseConfigurations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realmd.conf")
	content := "[database.login]\nname=realmd\nport=3306\n[database.world]\nname=mangos1\nport=3307\n[database.characters]\nname=character1\nport=3308\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WOW_AUTH_DSN", "")
	c, err := Parse([]string{"-config", path}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct{ dsn, name, port string }{{c.DSN(), "realmd", "3306"}, {c.WorldDSN(), "mangos1", "3307"}, {c.CharacterDSN(), "character1", "3308"}} {
		d, e := mysql.ParseDSN(entry.dsn)
		if e != nil || d.DBName != entry.name || !strings.HasSuffix(d.Addr, ":"+entry.port) {
			t.Fatal("database config aliasing")
		}
	}
}
