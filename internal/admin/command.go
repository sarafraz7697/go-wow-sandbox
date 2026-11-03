// Package admin implements the shared administrative commands.
package admin

import (
	"fmt"
	"io"
)

func Run(args []string, output, diagnostics io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: wowctl account create USER PASSWORD | account check | realm list | realm add NAME HOST PORT")
	}
	switch args[0] {
	case "account":
		return RunAccount(args[1:], output, diagnostics)
	case "realm":
		return RunRealm(args[1:], output, diagnostics)
	case "-h", "--help", "help":
		fmt.Fprintln(output, "wowctl account create USER PASSWORD [-config path]\nwowctl account check [-config path]\nwowctl realm list [-config path]\nwowctl realm add NAME HOST PORT [-config path]")
		return nil
	default:
		return fmt.Errorf("unknown command; use wowctl help")
	}
}
