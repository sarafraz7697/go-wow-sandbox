// Command account is a compatibility entry point for wowctl account.
package main

import (
	"fmt"
	"os"
	"wowserver/internal/admin"
)

func main() {
	if err := admin.RunAccount(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Done")
}
