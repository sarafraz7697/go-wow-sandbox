package admin

import (
	"bytes"
	"testing"
)

func TestCommandDispatch(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown"}, {"account"}, {"realm"}, {"account", "init"}, {"realm", "add", "name", "host", "0"}} {
		var out bytes.Buffer
		if err := Run(args, &out, &out); err == nil {
			t.Fatalf("accepted invalid command %v", args)
		}
	}
	for _, args := range [][]string{{"help"}, {"account", "check", "-h"}, {"realm", "list", "-h"}} {
		var out bytes.Buffer
		if err := Run(args, &out, &out); err != nil {
			t.Fatal(err)
		}
		if out.Len() == 0 {
			t.Fatal("missing help")
		}
	}
}
