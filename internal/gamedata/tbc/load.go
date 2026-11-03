// Package tbc maps build 8606 DBC layouts into the shared static-data model.
package tbc

import (
	"context"
	"fmt"
	"path/filepath"
	"wowserver/internal/dbc"
	"wowserver/internal/gamedata"
)

type Store interface {
	LoadStatic(context.Context) (gamedata.Rows, error)
}

func Load(ctx context.Context, directory string, locale int, store Store) (*gamedata.Catalog, error) {
	if locale < 0 || locale >= 16 {
		return nil, fmt.Errorf("DBC locale must be 0..15")
	}
	client, err := loadClient(ctx, directory, locale)
	if err != nil {
		return nil, err
	}
	rows, err := store.LoadStatic(ctx)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return gamedata.New(client, rows, 70)
}

type record struct {
	file  *dbc.File
	index int
	err   error
}

func (r *record) u(offset int) uint32 {
	if r.err != nil {
		return 0
	}
	v, e := r.file.Uint32(r.index, offset)
	r.err = e
	return v
}
func (r *record) b(offset int) byte {
	if r.err != nil {
		return 0
	}
	v, e := r.file.Byte(r.index, offset)
	r.err = e
	return v
}
func (r *record) name(first, locale int) string {
	var names [16]string
	for i := range names {
		if r.err != nil {
			return ""
		}
		names[i], r.err = r.file.String(r.index, (first+i)*4)
	}
	if names[locale] != "" {
		return names[locale]
	}
	for _, s := range names {
		if s != "" {
			return s
		}
	}
	return ""
}
func loadClient(ctx context.Context, dir string, locale int) (gamedata.ClientData, error) {
	var data gamedata.ClientData
	load := func(name string, fields, size uint32, consume func(*record)) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		f, err := dbc.Open(filepath.Join(dir, name+".dbc"), fields, size)
		if err != nil {
			return err
		}
		if f.Len() == 0 {
			return fmt.Errorf("%s.dbc is empty", name)
		}
		for i := 0; i < f.Len(); i++ {
			if i%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			r := record{file: f, index: i}
			consume(&r)
			if r.err != nil {
				return fmt.Errorf("%s.dbc row %d: %w", name, i, r.err)
			}
		}
		return nil
	}
	if err := load("ChrRaces", 69, 276, func(r *record) {
		data.Races = append(data.Races, gamedata.Race{ID: r.u(0), Faction: r.u(8), MaleDisplay: r.u(16), FemaleDisplay: r.u(20), Expansion: r.u(272), Name: r.name(14, locale)})
	}); err != nil {
		return data, err
	}
	if err := load("Faction", 53, 212, func(r *record) {
		data.Factions = append(data.Factions, gamedata.Faction{ID: r.u(0), Index: int32(r.u(4))})
	}); err != nil {
		return data, err
	}
	if err := load("ChrClasses", 58, 232, func(r *record) {
		data.Classes = append(data.Classes, gamedata.Class{ID: r.u(0), Power: r.u(8), SpellFamily: r.u(224), Name: r.name(4, locale)})
	}); err != nil {
		return data, err
	}
	if err := load("CharBaseInfo", 2, 2, func(r *record) {
		data.Pairs = append(data.Pairs, gamedata.Pair{Race: uint32(r.b(0)), Class: uint32(r.b(1))})
	}); err != nil {
		return data, err
	}
	if err := load("Map", 125, 500, func(r *record) { data.Maps = append(data.Maps, gamedata.Map{ID: r.u(0), Type: r.u(8)}) }); err != nil {
		return data, err
	}
	if err := load("CharStartOutfit", 41, 152, func(r *record) {
		v := gamedata.Outfit{Pair: gamedata.Pair{Race: uint32(r.b(4)), Class: uint32(r.b(5))}, Gender: r.b(6), ID: r.b(7)}
		for i := range v.Items {
			v.Items[i] = int32(r.u(8 + i*4))
		}
		data.Outfits = append(data.Outfits, v)
	}); err != nil {
		return data, err
	}
	if err := load("Spell", 216, 864, func(r *record) {
		data.SpellIDs = append(data.SpellIDs, r.u(0))
		data.Cooldowns = append(data.Cooldowns, gamedata.SpellCooldown{ID: r.u(0), Category: r.u(4), Recovery: r.u(92), CategoryRecovery: r.u(96)})
	}); err != nil {
		return data, err
	}
	if err := load("SpellItemEnchantment", 34, 136, func(r *record) {
		e := gamedata.Enchantment{ID: r.u(0), Visual: r.u(120), Condition: r.u(132)}
		for i := 0; i < 3; i++ {
			e.Effects[i] = r.u(4 + i*4)
			e.Amounts[i] = r.u(16 + i*4)
			e.Arguments[i] = r.u(40 + i*4)
		}
		data.Enchantments = append(data.Enchantments, e)
	}); err != nil {
		return data, err
	}
	if err := load("Item", 4, 16, func(r *record) {
		data.Items = append(data.Items, gamedata.Item{ID: r.u(0), Display: r.u(4), InventoryType: r.u(8)})
	}); err != nil {
		return data, err
	}
	// Same version markers used by pinned DBCStores, for tables loaded here.
	// WDBC has no build number: this detects stale files, not their provenance.
	spellFound, mapFound := false, false
	for _, id := range data.SpellIDs {
		if id == 53085 {
			spellFound = true
		}
	}
	for _, m := range data.Maps {
		if m.ID == 598 {
			mapFound = true
		}
	}
	if !spellFound || !mapFound {
		return data, fmt.Errorf("DBC version markers missing; re-extract from 2.4.3 build 8606")
	}
	return data, nil
}
