package admin

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"time"

	"wowserver/internal/config"
	"wowserver/internal/protocol/auth/tbc"
	"wowserver/internal/realm"
	store "wowserver/internal/storage/realmd"
)

func RunRealm(args []string, output, diagnostics io.Writer) error {
	if len(args) == 0 || (args[0] != "add" && args[0] != "list") {
		return fmt.Errorf("usage: realm add NAME HOST PORT [-config path] | realm list [-config path]")
	}
	action := args[0]
	args = args[1:]
	var r realm.Realm
	if action == "add" {
		if len(args) < 3 {
			return fmt.Errorf("realm add requires NAME HOST PORT")
		}
		port, err := strconv.ParseUint(args[2], 10, 16)
		if err != nil || port == 0 {
			return fmt.Errorf("invalid realm port")
		}
		r = realm.Realm{Name: args[0], Host: args[1], Port: uint16(port), Build: 8606, Category: 1}
		if _, err = tbc.EncodeRealms([]realm.Realm{r}); err != nil {
			return err
		}
		args = args[3:]
	}
	c, err := config.Parse(args, diagnostics)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := store.Open(ctx, c.DSN())
	if err != nil {
		return err
	}
	defer db.DB.Close()
	if action == "add" {
		if err = db.AddRealm(ctx, r.Name, r.Host, r.Port); err != nil {
			return fmt.Errorf("cannot add realm (check imported schema; name must be unique)")
		}
		fmt.Fprintln(output, "Realm added (build 8606)")
		return nil
	}
	realms, err := db.List(ctx, 0, 8606)
	if err != nil {
		return err
	}
	for _, r := range realms {
		fmt.Fprintf(output, "%d\t%s\t%s:%d\tflags=%d locked=%t build=%d\n", r.ID, r.Name, r.Host, r.Port, r.Flags, r.Locked, r.Build)
	}
	return nil
}
