package mangosone8606

import (
	"strings"
	"testing"
	"wowserver/internal/domain/character"
	"wowserver/internal/gamedata"
)

func TestNativeItemRecord(t *testing.T) {
	v := strings.Fields(EncodeItem(7, 9, character.InitialItem{Template: gamedata.Item{ID: 25, Bonding: 2, MaxDurability: 20, Charges: [5]int32{-1}}, Slot: 15, Count: 1}))
	if len(v) != 60 {
		t.Fatal(len(v))
	}
	// Independent expected update-field indices from pinned UpdateFields.h.
	for index, want := range map[int]string{0: "7", 1: "1073741824", 2: "3", 3: "25", 4: "1065353216", 6: "9", 8: "9", 14: "1", 16: "4294967295", 21: "1", 58: "20", 59: "20"} {
		if v[index] != want {
			t.Fatalf("field %d: %s != %s", index, v[index], want)
		}
	}
}
