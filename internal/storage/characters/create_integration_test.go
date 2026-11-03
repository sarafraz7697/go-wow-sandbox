package characters

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	mysql "github.com/go-sql-driver/mysql"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"wowserver/internal/account"
	"wowserver/internal/domain/character"
	"wowserver/internal/gamedata"
)

// Explicit opt-in, loopback sandbox only. Every run creates fresh schemas;
// neither existing accounts nor existing characters are changed.
func TestCreationTransactions(t *testing.T) {
	dsn := os.Getenv("WOW_CREATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set WOW_CREATION_TEST_DSN to the isolated MySQL sandbox on 3308")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:3308" {
		t.Fatal("creation tests require 127.0.0.1:3308")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	root, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	prefix := fmt.Sprintf("wow_test_creation_%d", time.Now().UnixNano())
	chars, login := prefix+"_chars", prefix+"_login"
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := root.ExecContext(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	for _, name := range []string{chars, login} {
		exec("CREATE DATABASE `" + name + "`")
		defer func(name string) {
			cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
			defer c()
			if _, e := root.ExecContext(cleanup, "DROP DATABASE `"+name+"`"); e != nil {
				t.Error(e)
			}
		}(name)
	}
	for _, table := range []string{"characters", "character_homebind", "character_inventory", "item_instance", "character_spell", "character_action", "character_skills", "character_reputation", "mail_items"} {
		exec("CREATE TABLE `" + chars + "`." + table + " LIKE character1." + table)
	}
	for _, table := range []string{"account", "realmlist", "realmcharacters", "account_banned", "ip_banned"} {
		exec("CREATE TABLE `" + login + "`." + table + " LIKE realmd." + table)
	}
	exec("INSERT INTO `" + login + "`.realmlist (id,name,address,realmbuilds) VALUES (1,'Test','127.0.0.1:8085','8606'),(2,'Other','127.0.0.1:8086','8606')")
	for i := 1; i <= 5; i++ {
		exec("INSERT INTO `"+login+"`.account (id,username,sessionkey,expansion) VALUES (?,?,?,1)", i, fmt.Sprintf("TEST%d", i), strings.Repeat("01", 40))
	}
	cc, ac := *cfg, *cfg
	cc.DBName = chars
	ac.DBName = login
	s, err := Open(ctx, cc.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	creator, err := s.NewCreator(ctx, cc.FormatDSN(), ac.FormatDSN(), 1, character.DefaultCreationPolicy())
	if err != nil {
		t.Fatal(err)
	}
	session := func(id uint64) account.WorldSession {
		a := account.WorldSession{ID: id, Expansion: 1}
		for i := range a.Key {
			a.Key[i] = 1
		}
		return a
	}
	plan := func(name string) character.Creation {
		return character.Creation{Request: character.CreateRequest{Name: name, Race: 1, Class: 1}, Health: 60, Spells: []uint32{78}, Actions: []gamedata.Action{{Button: 0, Action: 78}}, Skills: []character.InitialSkill{{ID: 43, Value: 1, Max: 5}}, Reputations: []character.InitialReputation{{Faction: 72}}, Items: []character.InitialItem{{Template: gamedata.Item{ID: 25, MaxDurability: 20}, Slot: 15, Count: 1}}}
	}
	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		if e := root.QueryRowContext(ctx, q, args...).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	t.Run("same_name_different_accounts", func(t *testing.T) {
		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for _, id := range []uint64{1, 2} {
			wg.Add(1)
			go func(id uint64) {
				defer wg.Done()
				<-start
				_, e := creator.Create(ctx, session(id), "127.0.0.1", plan("Collision"))
				results <- e
			}(id)
		}
		close(start)
		wg.Wait()
		close(results)
		success, duplicate := 0, 0
		for e := range results {
			if e == nil {
				success++
			} else if errors.Is(e, character.ErrNameInUse) {
				duplicate++
			} else {
				t.Error(e)
			}
		}
		if success != 1 || duplicate != 1 {
			t.Fatalf("success=%d duplicate=%d", success, duplicate)
		}
		if n := count("SELECT COUNT(*) FROM `" + chars + "`.characters WHERE name='Collision'"); n != 1 {
			t.Fatal(n)
		}
	})
	t.Run("rollback_after_partial_writes", func(t *testing.T) {
		broken := plan("Rollback")
		broken.Spells = []uint32{78, 78}
		if _, e := creator.Create(ctx, session(3), "127.0.0.1", broken); e == nil {
			t.Fatal("duplicate spell should fail")
		}
		if n := count("SELECT COUNT(*) FROM `" + chars + "`.characters WHERE account=3"); n != 0 {
			t.Fatal("partial character", n)
		}
		if n := count("SELECT COUNT(*) FROM `" + chars + "`.character_homebind"); n != 1 {
			t.Fatal("partial homebind", n)
		}
		if n := count("SELECT COUNT(*) FROM `" + login + "`.realmcharacters WHERE acctid=3"); n != 0 {
			t.Fatal("partial counter", n)
		}
		// Failed transaction must release its advisory lock and connection.
		if _, e := creator.Create(ctx, session(3), "127.0.0.1", plan("Recovery")); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("last_realm_slot_concurrent", func(t *testing.T) {
		for i := 0; i < 9; i++ {
			exec("INSERT INTO `"+chars+"`.characters (guid,account,name,race,class) VALUES (?,4,?,1,1)", 100+i, fmt.Sprintf("Existing%d", i))
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, name := range []string{"Lastone", "Lasttwo"} {
			go func(name string) {
				<-start
				_, e := creator.Create(ctx, session(4), "127.0.0.1", plan(name))
				results <- e
			}(name)
		}
		close(start)
		success, full := 0, 0
		for i := 0; i < 2; i++ {
			e := <-results
			if e == nil {
				success++
			} else if errors.Is(e, character.ErrRealmFull) {
				full++
			} else {
				t.Error(e)
			}
		}
		if success != 1 || full != 1 {
			t.Fatalf("success=%d full=%d", success, full)
		}
		if n := count("SELECT numchars FROM `" + login + "`.realmcharacters WHERE acctid=4 AND realmid=1"); n != 10 {
			t.Fatal(n)
		}
	})
	t.Run("account_limit", func(t *testing.T) {
		exec("INSERT INTO `" + login + "`.realmcharacters (realmid,acctid,numchars) VALUES (2,5,50)")
		if _, e := creator.Create(ctx, session(5), "127.0.0.1", plan("Accountfull")); !errors.Is(e, character.ErrAccountFull) {
			t.Fatal(e)
		}
	})
	t.Run("revoked_session", func(t *testing.T) {
		a := session(1)
		a.Key[0] = 2
		if _, e := creator.Create(ctx, a, "127.0.0.1", plan("Revoked")); !errors.Is(e, account.ErrChanged) {
			t.Fatal(e)
		}
	})
	t.Run("rollback_on_final_counter_write", func(t *testing.T) {
		tables := []string{"characters", "character_homebind", "character_spell", "character_action", "character_skills", "character_reputation", "item_instance", "character_inventory"}
		before := map[string]int{}
		for _, table := range tables {
			before[table] = count("SELECT COUNT(*) FROM `" + chars + "`." + table)
		}
		oldCount := count("SELECT numchars FROM `" + login + "`.realmcharacters WHERE acctid=3 AND realmid=1")
		exec("CREATE TRIGGER `" + login + "`.fail_counter BEFORE INSERT ON `" + login + "`.realmcharacters FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected counter failure'")
		defer exec("DROP TRIGGER `" + login + "`.fail_counter")
		if _, e := creator.Create(ctx, session(3), "127.0.0.1", plan("Latefailure")); e == nil {
			t.Fatal("counter failure not propagated")
		}
		for _, table := range tables {
			if n := count("SELECT COUNT(*) FROM `" + chars + "`." + table); n != before[table] {
				t.Fatalf("%s: partial rows remain", table)
			}
		}
		if n := count("SELECT numchars FROM `" + login + "`.realmcharacters WHERE acctid=3 AND realmid=1"); n != oldCount {
			t.Fatal("counter changed after rollback")
		}
	})
}
