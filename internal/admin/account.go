package admin

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"wowserver/internal/config"
	"wowserver/internal/domain/account"
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
	if len(args) < 1 {
		return fmt.Errorf("usage: account create USER PASSWORD | account check | account character reset-online [GUID] [-config path]")
	}
	switch args[0] {
	case "create", "check":
		return runAccountLegacy(args, output, diagnostics)
	case "character":
		return runAccountCharacter(args[1:], output, diagnostics)
	default:
		return fmt.Errorf("unknown account subcommand; use create, check or character")
	}
}

func runAccountLegacy(args []string, output, diagnostics io.Writer) error {
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

// runAccountCharacter handles "account character reset-online [GUID]".
// GUID is optional; omitting it resets every stale online flag.
func runAccountCharacter(args []string, output, diagnostics io.Writer) error {
	if len(args) < 1 || args[0] != "reset-online" {
		return fmt.Errorf("usage: account character reset-online [GUID] [-config path]")
	}
	args = args[1:]
	var guid uint64
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		g, err := strconv.ParseUint(args[0], 10, 32)
		if err != nil {
			return fmt.Errorf("invalid GUID")
		}
		guid = g
		args = args[1:]
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
	chars, err := characters.Open(ctx, c.CharacterDSN())
	if err != nil {
		return fmt.Errorf("characters: %w", err)
	}
	defer chars.DB.Close()
	n, err := chars.ResetOnline(ctx, uint32(guid))
	if err != nil {
		return err
	}
	if guid == 0 {
		fmt.Fprintf(output, "reset online flag for %d character(s)\n", n)
	} else if n == 0 {
		fmt.Fprintln(output, "character was not online (no change)")
	} else {
		fmt.Fprintf(output, "reset online flag for character guid %d\n", guid)
	}
	return nil
}
