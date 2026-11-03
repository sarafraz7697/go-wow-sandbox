package account

import "testing"

func TestNormalization(t *testing.T) {
	if s, err := Normalize("testUser"); err != nil || s != "TESTUSER" {
		t.Fatal(s, err)
	}
	for _, s := range []string{"", "12345678901234567", "has space", "a:b", "a\x00b", "نام"} {
		if _, err := Normalize(s); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
}
