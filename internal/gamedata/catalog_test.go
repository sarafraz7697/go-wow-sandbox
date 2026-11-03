package gamedata

import (
	"errors"
	"math"
	"testing"
)

func creationFixture() (ClientData, Rows) {
	p := Pair{1, 1}
	client := ClientData{Races: []Race{{ID: 1}}, Classes: []Class{{ID: 1}}, Maps: []Map{{ID: 0}}, Pairs: []Pair{p}, SpellIDs: []uint32{100}, Outfits: []Outfit{{Pair: p, Gender: 0, Items: [12]int32{25}}, {Pair: p, Gender: 1, Items: [12]int32{25}}}}
	db := Rows{Starts: []Start{{Pair: p}}, Items: []Item{{ID: 25, Display: 123, InventoryType: 21}}, Spells: []StartSpell{{Pair: p, Spell: 100}}, StartItems: []StartItem{{Pair: p, Item: 25, Amount: 1}}, Actions: []Action{{Pair: p, Button: 0, Action: 100}}, Levels: []Level{{Pair: p, Level: 1, Stats: [5]uint32{20, 20, 20, 20, 20}}, {Pair: p, Level: 2, Stats: [5]uint32{21, 21, 21, 21, 21}}}, ClassLevels: []ClassLevel{{Class: 1, Level: 1, Health: 20}, {Class: 1, Level: 2, Health: 30}}, XP: []XP{{Level: 1, Next: 400}}, Reserved: []string{"Admin"}}
	return client, db
}
func TestZeroItemDoesNotAbortStartup(t *testing.T) {
	client, db := creationFixture()
	db.Items = append(db.Items, Item{ID: 0, Display: 45479, InventoryType: 21})
	c, e := New(client, db, 2)
	if e != nil {
		t.Fatal(e)
	}
	if _, ok := c.Item(0); ok {
		t.Fatal("zero exposed as usable item")
	}
	if c.Summary().SkippedZeroItems != 1 || c.Summary().Items != 1 {
		t.Fatal("incorrect load counts")
	}
	db.StartItems[0].Item = 0
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
	db.StartItems[0].Amount = 99
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
	for name, change := range map[string]func(*ClientData, *Rows){
		"missing stats":       func(_ *ClientData, d *Rows) { d.Levels = d.Levels[:1] },
		"missing spell":       func(_ *ClientData, d *Rows) { d.Spells[0].Spell = 999 },
		"invalid inventory":   func(_ *ClientData, d *Rows) { d.Items[0].InventoryType = 29 },
		"duplicate item":      func(_ *ClientData, d *Rows) { d.Items = append(d.Items, d.Items[0]) },
		"missing outfit item": func(c *ClientData, _ *Rows) { c.Outfits[0].Items[0] = 999 },
		"instance start":      func(c *ClientData, _ *Rows) { c.Maps[0].Type = 1 },
		"NaN":                 func(_ *ClientData, d *Rows) { d.Starts[0].X = float32(math.NaN()) },
		"empty schema":        func(_ *ClientData, d *Rows) { *d = Rows{} },
	} {
		t.Run(name, func(t *testing.T) {
			client, db := creationFixture()
			change(&client, &db)
			if _, e := New(client, db, 2); e == nil {
				t.Fatal("invalid data accepted")
			}
		})
	}
}
