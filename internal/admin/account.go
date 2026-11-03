package admin

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"wowserver/internal/domain/account"
	"wowserver/internal/config"
	"wowserver/internal/storage/characters"
	store "wowserver/internal/storage/realmd"
	"wowserver/internal/storage/world"
)

func createArgs(args []string) (name, password string, flags []string, err error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return os.Getenv("WOW_ACCOUNT_NAME"),
			os.Getenv("WOW_ACCOUNT_PASSWORD"), args, nil
	}
	if len(args) < 2 {
		return "", "", nil, fmt.Errorf("usage: account create USERNAME PASSWORD [-config configs/realmd.conf]")
	}
	return args[0], args[1], args[2:], nil
}

func RunAccount(args []string, output, diagnostics io.Writer) error {
	if len(args) < 1 || (args[0] != "create" && args[0] != "check") {
		return fmt.Errorf("usage: account create USERNAME PASSWORD [-config path], or account check [-config path]; schema must be imported externally")
	}
	action := args[0]
	args = args[1:]
	var rawName, rawPassword string
	var err error
	if action == "create" {
		rawName, rawPassword, args, err = createArgs(args)
		if err != nil {
			return err
		}
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
	if err = db.Check(ctx); err != nil {
		return err
	}
	if action == "check" {
		fmt.Fprintln(output, "realmd connection and required schema OK")
		chars, err := characters.Open(ctx, c.CharacterDSN())
		if err != nil {
			return fmt.Errorf("characters: %w", err)
		}
		defer chars.DB.Close()
		if err = chars.Check(ctx); err != nil {
			return err
		}
		fmt.Fprintln(output, "characters connection and required schema OK")
		w, err := world.Open(ctx, c.WorldDSN())
		if err != nil {
			return fmt.Errorf("world: %w", err)
		}
		defer w.DB.Close()
		if err = w.Check(ctx); err != nil {
			return err
		}
		fmt.Fprintln(output, "world connection and required schema OK")
		return nil
	}
	name, err := account.Normalize(rawName)
	if err != nil {
		return err
	}
	password, err := account.Normalize(rawPassword)
	if err != nil {
		return err
	}
	return db.Create(ctx, name, password)
}
