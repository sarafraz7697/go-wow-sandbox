package client

import "testing"

func TestExplicitSupport(t *testing.T) {
	if _, ok := Lookup(8606, [3]byte{2, 4, 3}); !ok {
		t.Fatal("TBC rejected")
	}
	for _, build := range []uint16{0, 8605, 8607, 12340} {
		if _, ok := Lookup(build, [3]byte{2, 4, 3}); ok {
			t.Fatalf("accepted %d", build)
		}
	}
	if _, ok := Lookup(8606, [3]byte{3, 3, 5}); ok {
		t.Fatal("accepted mismatched version")
	}
}
