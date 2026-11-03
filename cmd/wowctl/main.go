package main

import (
	"fmt"
	"os"
	"wowserver/internal/admin"
)

func main() {
	if err := admin.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
