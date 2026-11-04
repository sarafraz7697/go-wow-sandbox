package gamedata

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// New validates and atomically constructs a snapshot. This implementation
// rejects incomplete data instead of silently inventing later-level stats.
func New(client ClientData, db Rows, maxLevel uint32) (*Catalog, error) {
	c := &Catalog{races: map[uint32]Race{}, classes: map[uint32]Class{}, maps: map[uint32]Map{}, pairs: map[Pair]bool{}, starts: map[Pair]Start{}, outfits: map[outfitKey]Outfit{}, items: map[uint32]Item{}, spells: map[uint32]bool{}, startItems: map[Pair][]StartItem{}, startSpells: map[Pair][]uint32{}, actions: map[Pair][]Action{}, levels: map[levelKey]Level{}, classLevels: map[classLevelKey]ClassLevel{}, xp: map[uint32]uint32{}, reserved: map[string]bool{}, taverns: map[uint32]TavernTrigger{}}
	if maxLevel < 1 || maxLevel > 255 {
		return nil, fmt.Errorf("invalid max level")
	}
	c.itemDetails = make(map[uint32]ItemDetails, len(db.ItemDetails))
	c.cooldowns = make(map[uint32]SpellCooldown, len(client.Cooldowns))
	for _, v := range client.Cooldowns {
		c.cooldowns[v.ID] = v
	}
	for _, v := range db.ItemDetails {
		if v.ID != 0 {
			c.itemDetails[v.ID] = v
		}
	}
	c.factionIndices = make(map[uint32]int32, len(client.Factions))
	for _, v := range client.Factions {
		if v.Index >= 0 && v.Index < 128 {
			c.factionIndices[v.ID] = v.Index
		}
	}
	if err := c.loadClient(client); err != nil {
		return nil, err
	}
	if err := c.loadVisuals(client, db); err != nil {
		return nil, err
	}
	if err := c.loadCreation(client, db); err != nil {
		return nil, err
	}
	if err := c.loadProgression(db, maxLevel); err != nil {
		return nil, err
	}
	if err := c.loadTaverns(db, client); err != nil {
		return nil, err
	}
	if err := c.validateCompleteness(db, maxLevel); err != nil {
		return nil, err
	}
	c.summary = Summary{len(c.races), len(c.classes), len(c.maps), len(c.starts), len(c.outfits), len(c.spells), len(c.items), len(db.StartItems), len(db.Spells), len(db.Actions), len(c.levels), len(c.classLevels), len(c.xp), len(c.reserved), c.skippedZeroItems, len(c.taverns)}
	return c, nil
}

func (c *Catalog) loadClient(client ClientData) error {
	for _, v := range client.Races {
		if _, ok := c.races[v.ID]; ok || v.ID == 0 {
			return fmt.Errorf("duplicate/invalid race %d", v.ID)
		}
		c.races[v.ID] = v
	}
	for _, v := range client.Classes {
		if _, ok := c.classes[v.ID]; ok || v.ID == 0 {
			return fmt.Errorf("duplicate/invalid class %d", v.ID)
		}
		c.classes[v.ID] = v
	}
	for _, v := range client.Maps {
		if _, ok := c.maps[v.ID]; ok {
			return fmt.Errorf("duplicate map %d", v.ID)
		}
		c.maps[v.ID] = v
	}
	for _, v := range client.Pairs {
		_, r := c.races[v.Race]
		_, cl := c.classes[v.Class]
		if !r || !cl || c.pairs[v] {
			return fmt.Errorf("invalid race/class pair %v", v)
		}
		c.pairs[v] = true
	}
	for _, id := range client.SpellIDs {
		if c.spells[id] {
			return fmt.Errorf("duplicate spell %d", id)
		}
		c.spells[id] = true
	}
	return nil
}

func (c *Catalog) loadCreation(client ClientData, db Rows) error {
	for _, v := range db.Items {
		// Entry zero is not a usable item identifier. The reference validates
		// item prototypes starting at 1; do not fail the entire startup on it.
		if v.ID == 0 {
			c.skippedZeroItems++
			continue
		}
		if _, ok := c.items[v.ID]; ok || v.InventoryType > 28 {
			return fmt.Errorf("invalid item_template entry %d", v.ID)
		}
		c.items[v.ID] = v
	}
	// Reference keeps the database inventory type but corrects known display IDs.
	for _, v := range client.Items {
		if item, ok := c.items[v.ID]; ok {
			item.Display = v.Display
			c.items[v.ID] = item
		}
	}
	for _, v := range db.Starts {
		m, ok := c.maps[v.Map]
		if !c.pairs[v.Pair] || !ok || m.Type != 0 {
			return fmt.Errorf("invalid playercreateinfo race=%d class=%d map=%d", v.Race, v.Class, v.Map)
		}
		for _, n := range []float32{v.X, v.Y, v.Z, v.Orientation} {
			if math.IsNaN(float64(n)) || math.IsInf(float64(n), 0) {
				return fmt.Errorf("non-finite start coordinates for %v", v.Pair)
			}
		}
		if _, ok = c.starts[v.Pair]; ok {
			return fmt.Errorf("duplicate start %v", v.Pair)
		}
		c.starts[v.Pair] = v
	}
	if len(c.starts) == 0 || len(c.items) == 0 {
		return fmt.Errorf("world database has no creation/item data; a schema-only dump is insufficient")
	}
	for _, v := range client.Outfits {
		if !c.pairs[v.Pair] {
			continue
		}
		if v.Gender > 1 {
			return fmt.Errorf("invalid outfit gender")
		}
		for _, id := range v.Items {
			if id > 0 {
				if _, ok := c.items[uint32(id)]; !ok {
					return fmt.Errorf("outfit item %d missing in item_template", id)
				}
			}
		}
		key := outfitKey{v.Pair, v.Gender, v.ID}
		if _, ok := c.outfits[key]; ok {
			return fmt.Errorf("duplicate outfit %v", key)
		}
		c.outfits[key] = v
	}
	for _, v := range db.StartItems {
		if _, ok := c.starts[v.Pair]; !ok {
			return fmt.Errorf("orphan start item %v", v.Pair)
		}
		if _, ok := c.items[v.Item]; !ok || v.Amount == 0 {
			return fmt.Errorf("invalid starting item %d", v.Item)
		}
		c.startItems[v.Pair] = append(c.startItems[v.Pair], v)
	}
	for _, v := range db.Spells {
		if _, ok := c.starts[v.Pair]; !ok || !c.spells[v.Spell] {
			return fmt.Errorf("invalid start spell %d for %v", v.Spell, v.Pair)
		}
		c.startSpells[v.Pair] = append(c.startSpells[v.Pair], v.Spell)
	}
	for _, v := range db.Actions {
		if _, ok := c.starts[v.Pair]; !ok || v.Button >= 132 || v.Action >= 0x1000000 {
			return fmt.Errorf("invalid starting action %v", v)
		}
		if v.Type == 0 && !c.spells[v.Action] {
			return fmt.Errorf("action spell %d missing", v.Action)
		}
		if v.Type == 128 {
			if _, ok := c.items[v.Action]; !ok {
				return fmt.Errorf("action item %d missing", v.Action)
			}
		}
		c.actions[v.Pair] = append(c.actions[v.Pair], v)
	}
	return nil
}

// loadTaverns joins Rows.TavernTriggerIDs (the world DB's areatrigger_tavern
// table) against the AreaTrigger.dbc rows the client loader shipped. Only
// triggers that exist in both are kept; a tavern id with no DBC row is
// silently dropped to mirror the reference's "skip and log" behavior.
func (c *Catalog) loadTaverns(db Rows, client ClientData) error {
	if len(db.TavernTriggerIDs) == 0 {
		return nil
	}
	want := make(map[uint32]bool, len(db.TavernTriggerIDs))
	for _, id := range db.TavernTriggerIDs {
		want[id] = true
	}
	for _, t := range client.AreaTriggers {
		if !want[t.ID] {
			continue
		}
		if _, dup := c.taverns[t.ID]; dup {
			return fmt.Errorf("duplicate areatrigger_tavern id %d", t.ID)
		}
		c.taverns[t.ID] = t
	}
	return nil
}

func (c *Catalog) loadProgression(db Rows, maxLevel uint32) error {
	for _, v := range db.Levels {
		if v.Level > maxLevel {
			continue
		}
		_, raceOK := c.races[v.Race]
		_, classOK := c.classes[v.Class]
		if v.Level == 0 || !raceOK || !classOK {
			return fmt.Errorf("invalid level stats %v", v.Pair)
		}
		key := levelKey{v.Pair, v.Level}
		if _, ok := c.levels[key]; ok {
			return fmt.Errorf("duplicate level stats")
		}
		c.levels[key] = v
	}
	for _, v := range db.ClassLevels {
		if v.Level > maxLevel {
			continue
		}
		if _, ok := c.classes[v.Class]; !ok || v.Level == 0 {
			return fmt.Errorf("invalid class stats")
		}
		key := classLevelKey{v.Class, v.Level}
		if _, ok := c.classLevels[key]; ok {
			return fmt.Errorf("duplicate class stats")
		}
		c.classLevels[key] = v
	}
	for _, v := range db.XP {
		if v.Level == 0 || v.Level >= maxLevel {
			continue
		}
		if v.Next == 0 {
			return fmt.Errorf("invalid XP row")
		}
		if _, ok := c.xp[v.Level]; ok {
			return fmt.Errorf("duplicate XP level")
		}
		c.xp[v.Level] = v.Next
	}
	return nil
}

func (c *Catalog) validateCompleteness(db Rows, maxLevel uint32) error {
	for pair := range c.starts {
		if len(c.startSpells[pair]) == 0 {
			return fmt.Errorf("missing starting spells for %v", pair)
		}
		for gender := uint8(0); gender < 2; gender++ {
			found := false
			for key := range c.outfits {
				if key.Pair == pair && key.Gender == gender {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("missing starting outfit for %v gender=%d", pair, gender)
			}
		}
		for level := uint32(1); level <= maxLevel; level++ {
			v, ok := c.levels[levelKey{pair, level}]
			if !ok || v.Stats[0] == 0 {
				return fmt.Errorf("missing stats race=%d class=%d level=%d", pair.Race, pair.Class, level)
			}
			cl, ok := c.classLevels[classLevelKey{pair.Class, level}]
			if !ok || cl.Health == 0 {
				return fmt.Errorf("missing class stats class=%d level=%d", pair.Class, level)
			}
			if level < maxLevel && c.xp[level] == 0 {
				return fmt.Errorf("missing XP level=%d", level)
			}
		}
	}
	for _, v := range db.Reserved {
		if !utf8.ValidString(v) || v == "" {
			return fmt.Errorf("invalid reserved_name")
		}
		c.reserved[strings.ToLower(v)] = true
	}
	return nil
}
