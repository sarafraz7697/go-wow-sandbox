package worldload

import (
	"context"
	"database/sql/driver"
	"testing"
)

func TestItemTemplateLoader(t *testing.T) {
	ResetForTest()
	db, srv := newFakeDB(t)
	srv.put("SELECT COUNT(*) FROM `item_template`", rows1(2))

	// Column count the loader scans: 26 header + 20 stats + 15 damage +
	// 10 res/delay + 35 spells + 18 misc (bonding..TotemCategory) +
	// 6 sockets + 5 tail = 135.
	total := 26 + 2*10 + 3*5 + 10 + 7*5 + 18 + 2*3 + 5
	cols := make([]string, total)
	row := make([]driver.Value, total)
	for i := range cols {
		cols[i] = "c"
	}
	row[0] = int64(6948) // entry
	row[1] = int64(2)    // class = weapon
	row[2] = int64(10)   // subclass = sword
	row[4] = "Hearthstone"
	row[5] = int64(6418) // displayid
	row[6] = int64(1)    // Quality
	row[11] = int64(0)   // InventoryType
	row[25] = int64(20)  // ContainerSlots (maxcount=23, stackable=24, ContainerSlots=25)
	// stat_type1/stat_value1 come right after ContainerSlots.
	row[26], row[27] = int64(7), int64(10) // stat type 7 (stamina), +10
	// First spell block starts after stats+damage+res/delay.
	spellBase := 26 + 20 + 15 + 10
	row[spellBase] = int64(3)    // spellid_1
	row[spellBase+2] = int64(-1) // spellcharges_1
	row[spellBase+4] = int64(-1) // spellcooldown_1 = -1 (use dbc cooldown)
	row[107] = "Feels like home" // description (right after bonding at index 106)
	row[134] = int64(0)          // duration

	zeroRow := make([]driver.Value, total)
	zeroRow[0] = int64(0)

	srv.put(itemTemplateQuery(), &fakeRows{cols: cols, data: [][]driver.Value{row, zeroRow}})

	Register(Loader{Name: "item_template", Order: OrderItemTemplate, CountQuery: "SELECT COUNT(*) FROM `item_template`", Load: loadItemPrototypes})

	dest := newDatasetForTest()
	rep, err := Run(context.Background(), db, dest, RunOptions{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(dest.ItemPrototypes) != 1 {
		t.Fatalf("item prototypes = %d, want 1 (entry 0 skipped)", len(dest.ItemPrototypes))
	}
	it := dest.ItemPrototypes[6948]
	if it.Name1 != "Hearthstone" || it.Class != 2 || it.DisplayInfoID != 6418 || it.Quality != 1 {
		t.Fatalf("item header wrong: %+v", it)
	}
	if it.ItemStat[0].ItemType != 7 || it.ItemStat[0].Value != 10 {
		t.Fatalf("item stat wrong: %+v", it.ItemStat[0])
	}
	if it.Spells[0].SpellID != 3 || it.Spells[0].SpellCharges != -1 || it.Spells[0].SpellCooldown != -1 {
		t.Fatalf("item spell wrong: %+v", it.Spells[0])
	}
	if it.Description != "Feels like home" {
		t.Fatalf("item description wrong: %q", it.Description)
	}
	if rep.TotalSkipped() != 1 {
		t.Fatalf("skipped = %d, want 1", rep.TotalSkipped())
	}
}
