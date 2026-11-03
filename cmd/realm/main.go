// Command realm is a compatibility entry point for wowctl realm.
package main

import (
	"fmt"
	"os"
	"wowserver/internal/admin"
)

func main() {
	if err := admin.RunRealm(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

}
