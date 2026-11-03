package admin

import "testing"

func TestCreateArgs(t *testing.T) {
	t.Setenv("WOW_ACCOUNT_NAME", "ENVUSER")
	t.Setenv("WOW_ACCOUNT_PASSWORD", "ENVPASS")
	name, password, flags, err := createArgs([]string{"admin", "admin", "-config", "test.conf"})
	if err != nil || name != "admin" || password != "admin" || len(flags) != 2 {
		t.Fatal("positional args")
	}
	name, password, flags, err = createArgs([]string{"-config", "test.conf"})
	if err != nil || name != "ENVUSER" || password != "ENVPASS" || len(flags) != 2 {
		t.Fatal("env fallback")
	}
	if _, _, _, err = createArgs([]string{"admin"}); err == nil {
		t.Fatal("missing password accepted")
	}
	_, password, _, err = createArgs([]string{"admin", "-literal"})
	if err != nil || password != "-literal" {
		t.Fatal("password treated as flag")
	}
}
