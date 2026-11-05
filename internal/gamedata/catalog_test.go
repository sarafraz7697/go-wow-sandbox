package gamedata

import (
	"errors"
	"math"
	"testing"
	"wowserver/internal/entity"
)

func creationFixture() (ClientData, *entity.Dataset) {
	p := Pair{1, 1}
	client := ClientData{Races: []Race{{ID: 1}}, Classes: []Class{{ID: 1}}, Maps: []Map{{ID: 0}}, Pairs: []Pair{p}, SpellIDs: []uint32{100}, Outfits: []Outfit{{Pair: p, Gender: 0, Items: [12]int32{25}}, {Pair: p, Gender: 1, Items: [12]int32{25}}}}
	db := entity.NewDataset()
	db.PlayerCreateInfos[entity.RaceClassKey{Race: 1, Class: 1}] = entity.PlayerCreateInfo{Map: 0}
	db.ItemPrototypes[25] = entity.ItemPrototype{ItemID: 25, DisplayInfoID: 123, InventoryType: 21}
	db.PlayerCreateInfoSpells = []entity.PlayerCreateInfoSpell{{Race: 1, Class: 1, Spell: 100}}
	db.PlayerCreateInfoItems = []entity.PlayerCreateInfoItem{{Race: 1, Class: 1, ItemID: 25, Amount: 1}}
	db.PlayerCreateInfoActions = []entity.PlayerCreateInfoAction{{Race: 1, Class: 1, Button: 0, Action: 100}}
	db.PlayerLevelStats[entity.RaceClassLevelKey{Race: 1, Class: 1, Level: 1}] = entity.PlayerLevelStats{Level: 1, Str: 20, Agi: 20, Sta: 20, Inte: 20, Spi: 20}
	db.PlayerLevelStats[entity.RaceClassLevelKey{Race: 1, Class: 1, Level: 2}] = entity.PlayerLevelStats{Level: 2, Str: 21, Agi: 21, Sta: 21, Inte: 21, Spi: 21}
	db.PlayerClassLevelStats[entity.ClassLevelKey{Class: 1, Level: 1}] = entity.PlayerClassLevelStats{Class: 1, Level: 1, BaseHP: 20}
	db.PlayerClassLevelStats[entity.ClassLevelKey{Class: 1, Level: 2}] = entity.PlayerClassLevelStats{Class: 1, Level: 2, BaseHP: 30}
	db.PlayerXPForLevel[1] = 400
	db.ReservedNames["Admin"] = struct{}{}
	return client, db
}

func TestZeroItemDoesNotAbortStartup(t *testing.T) {
	client, db := creationFixture()
	db.ItemPrototypes[0] = entity.ItemPrototype{ItemID: 0, DisplayInfoID: 45479, InventoryType: 21}
	c, e := New(client, db, 2)
	if e != nil {
		t.Fatal(e)
	}
	if _, ok := c.ItemPrototype(0); ok {
		t.Fatal("zero exposed as usable item")
	}
	if c.Summary().SkippedZeroItems != 1 || c.Summary().Items != 1 {
		t.Fatal("incorrect load counts")
	}
	db.PlayerCreateInfoItems[0].ItemID = 0
	if _, e := New(client, db, 2); e == nil {
		t.Fatal("starting item reference to zero accepted")
	}
}
func TestCatalogOwnershipAndNames(t *testing.T) {
	client, db := creationFixture()
	c, e := New(client, db, 2)
	if e != nil {
		t.Fatal(e)
	}
	db.PlayerCreateInfoItems[0].Amount = 99
	items := c.StartItems(1, 1)
	items[0].Amount = 98
	spells := c.StartSpells(1, 1)
	spells[0] = 999
	if c.StartItems(1, 1)[0].Amount != 1 || c.StartSpells(1, 1)[0] != 100 {
		t.Fatal("mutable slice escaped")
	}
	v, _ := c.Start(1, 1)
	v.Zone = 99
	again, _ := c.Start(1, 1)
	if again.Zone != 0 {
		t.Fatal("mutable start")
	}
	if _, e := c.NormalizeName("ADMIN", false); !errors.Is(e, ErrNameReserved) {
		t.Fatal(e)
	}
	if n, e := c.NormalizeName("aDMIN", true); e != nil || n != "Admin" {
		t.Fatal(n, e)
	}
	for _, n := range []string{"A", "Name1", "two words", "نام"} {
		if _, e := c.NormalizeName(n, false); e == nil {
			t.Fatal("invalid name", n)
		}
	}
}
func TestInvalidCreationData(t *testing.T) {
	for name, change := range map[string]func(*ClientData, *entity.Dataset){
		"missing stats": func(_ *ClientData, d *entity.Dataset) {
			delete(d.PlayerLevelStats, entity.RaceClassLevelKey{Race: 1, Class: 1, Level: 2})
		},
		"missing spell": func(_ *ClientData, d *entity.Dataset) { d.PlayerCreateInfoSpells[0].Spell = 999 },
		"invalid inventory": func(_ *ClientData, d *entity.Dataset) {
			it := d.ItemPrototypes[25]
			it.InventoryType = 29
			d.ItemPrototypes[25] = it
		},
		// "duplicate item" is no longer representable: the dataset
		// keys prototypes by entry, so the loader-side duplicate
		// check owns it.
		"missing outfit item": func(c *ClientData, _ *entity.Dataset) { c.Outfits[0].Items[0] = 999 },
		"instance start":      func(c *ClientData, _ *entity.Dataset) { c.Maps[0].Type = 1 },
		"NaN": func(_ *ClientData, d *entity.Dataset) {
			st := d.PlayerCreateInfos[entity.RaceClassKey{Race: 1, Class: 1}]
			st.PositionX = float32(math.NaN())
			d.PlayerCreateInfos[entity.RaceClassKey{Race: 1, Class: 1}] = st
		},
		"empty schema": func(_ *ClientData, d *entity.Dataset) { *d = *entity.NewDataset() },
	} {
		{
			t.Run(name, func(t *testing.T) {
				client, db := creationFixture()
				change(&client, db)
				if _, e := New(client, db, 2); e == nil {
					t.Fatal("invalid data accepted")
				}
			})
		}
	}
}
