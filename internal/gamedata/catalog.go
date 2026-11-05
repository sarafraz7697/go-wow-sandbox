package gamedata

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"wowserver/internal/entity"
)

// New validates and atomically constructs a snapshot from the client DBC
// data plus the world entities loaded into the dataset. This
// implementation rejects incomplete data instead of silently inventing
// later-level stats.
func New(client ClientData, db *entity.Dataset, maxLevel uint32) (*Catalog, error) {
	c := &Catalog{races: map[uint32]Race{}, classes: map[uint32]Class{}, maps: map[uint32]Map{}, pairs: map[Pair]bool{}, starts: map[Pair]Start{}, outfits: map[outfitKey]Outfit{}, spells: map[uint32]bool{}, startItems: map[Pair][]StartItem{}, startSpells: map[Pair][]uint32{}, actions: map[Pair][]Action{}, levels: map[levelKey]Level{}, classLevels: map[classLevelKey]ClassLevel{}, xp: map[uint32]uint32{}, reserved: map[string]bool{}, taverns: map[uint32]TavernTrigger{}}
	if maxLevel < 1 || maxLevel > 255 {
		return nil, fmt.Errorf("invalid max level")
	}
	c.cooldowns = make(map[uint32]SpellCooldown, len(client.Cooldowns))
	for _, v := range client.Cooldowns {
		c.cooldowns[v.ID] = v
	}
	c.factionIndices = make(map[uint32]int32, len(client.Factions))
	for _, v := range client.Factions {
		if v.Index >= 0 && v.Index < 128 {
			c.factionIndices[v.ID] = v.Index
		}
	}
	c.enchantments = make(map[uint32]Enchantment, len(client.Enchantments))
	for _, v := range client.Enchantments {
		if _, ok := c.enchantments[v.ID]; ok {
			return nil, fmt.Errorf("duplicate enchantment %d", v.ID)
		}
		c.enchantments[v.ID] = v
	}
	if err := c.loadClient(client); err != nil {
		return nil, err
	}
	if err := c.loadWorldEntities(db); err != nil {
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
	c.summary = Summary{len(c.races), len(c.classes), len(c.maps), len(c.starts), len(c.outfits), len(c.spells), len(c.itemPrototypes), len(db.PlayerCreateInfoItems), len(db.PlayerCreateInfoSpells), len(db.PlayerCreateInfoActions), len(c.levels), len(c.classLevels), len(c.xp), len(c.reserved), c.skippedZeroItems, len(c.taverns), len(c.creatureTemplates), len(c.goTemplates), len(c.quests)}
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

func (c *Catalog) loadCreation(client ClientData, db *entity.Dataset) error {
	for _, v := range db.ItemPrototypes {
		// Entry zero is not a usable item identifier. The reference
		// validates item prototypes starting at 1; do not fail the
		// entire startup on it.
		if v.ItemID == 0 {
			c.skippedZeroItems++
			continue
		}
		if _, ok := c.itemPrototypes[v.ItemID]; ok || v.InventoryType > 28 {
			return fmt.Errorf("invalid item_template entry %d", v.ItemID)
		}
		c.itemPrototypes[v.ItemID] = v
	}
	// Reference keeps the database inventory type but corrects known display IDs.
	for _, v := range client.Items {
		if item, ok := c.itemPrototypes[v.ID]; ok {
			item.DisplayInfoID = v.Display
			c.itemPrototypes[v.ID] = item
		}
	}
	for key, v := range db.PlayerCreateInfos {
		pair := Pair{key.Race, key.Class}
		m, ok := c.maps[v.Map]
		if !c.pairs[pair] || !ok || m.Type != 0 {
			return fmt.Errorf("invalid playercreateinfo race=%d class=%d map=%d", pair.Race, pair.Class, v.Map)
		}
		for _, n := range []float32{v.PositionX, v.PositionY, v.PositionZ, v.Orientation} {
			if math.IsNaN(float64(n)) || math.IsInf(float64(n), 0) {
				return fmt.Errorf("non-finite start coordinates for %v", pair)
			}
		}
		if _, ok = c.starts[pair]; ok {
			return fmt.Errorf("duplicate start %v", pair)
		}
		c.starts[pair] = Start{Pair: pair, Map: v.Map, Zone: v.Zone, X: v.PositionX, Y: v.PositionY, Z: v.PositionZ, Orientation: v.Orientation}
	}
	if len(c.starts) == 0 || len(c.itemPrototypes) == 0 {
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
				if _, ok := c.itemPrototypes[uint32(id)]; !ok {
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
	for _, v := range db.PlayerCreateInfoItems {
		pair := Pair{v.Race, v.Class}
		if _, ok := c.starts[pair]; !ok {
			return fmt.Errorf("orphan start item %v", pair)
		}
		if _, ok := c.itemPrototypes[v.ItemID]; !ok || v.Amount == 0 {
			return fmt.Errorf("invalid starting item %d", v.ItemID)
		}
		c.startItems[pair] = append(c.startItems[pair], StartItem{Pair: pair, Item: v.ItemID, Amount: v.Amount})
	}
	for _, v := range db.PlayerCreateInfoSpells {
		pair := Pair{v.Race, v.Class}
		if _, ok := c.starts[pair]; !ok || !c.spells[v.Spell] {
			return fmt.Errorf("invalid start spell %d for %v", v.Spell, pair)
		}
		c.startSpells[pair] = append(c.startSpells[pair], v.Spell)
	}
	for _, v := range db.PlayerCreateInfoActions {
		pair := Pair{v.Race, v.Class}
		if _, ok := c.starts[pair]; !ok || v.Button >= 132 || v.Action >= 0x1000000 {
			return fmt.Errorf("invalid starting action %v", v)
		}
		if v.Type == 0 && !c.spells[v.Action] {
			return fmt.Errorf("action spell %d missing", v.Action)
		}
		if v.Type == 128 {
			if _, ok := c.itemPrototypes[v.Action]; !ok {
				return fmt.Errorf("action item %d missing", v.Action)
			}
		}
		c.actions[pair] = append(c.actions[pair], Action{Pair: pair, Button: v.Button, Action: v.Action, Type: v.Type})
	}
	return nil
}

// loadTaverns joins the dataset's areatrigger_tavern rows (world DB)
// against the AreaTrigger.dbc rows the client loader shipped. Only
// triggers that exist in both are kept; a tavern id with no DBC row is
// silently dropped to mirror the reference's "skip and log" behavior.
func (c *Catalog) loadTaverns(db *entity.Dataset, client ClientData) error {
	if len(db.TavernTriggers) == 0 {
		return nil
	}
	for _, t := range client.AreaTriggers {
		if _, want := db.TavernTriggers[t.ID]; !want {
			continue
		}
		if _, dup := c.taverns[t.ID]; dup {
			return fmt.Errorf("duplicate areatrigger_tavern id %d", t.ID)
		}
		c.taverns[t.ID] = t
	}
	return nil
}

func (c *Catalog) loadProgression(db *entity.Dataset, maxLevel uint32) error {
	for key, v := range db.PlayerLevelStats {
		pair := Pair{key.Race, key.Class}
		if key.Level > maxLevel {
			continue
		}
		_, raceOK := c.races[key.Race]
		_, classOK := c.classes[key.Class]
		if key.Level == 0 || !raceOK || !classOK {
			return fmt.Errorf("invalid level stats %v", pair)
		}
		lk := levelKey{pair, key.Level}
		if _, ok := c.levels[lk]; ok {
			return fmt.Errorf("duplicate level stats")
		}
		c.levels[lk] = Level{Pair: pair, Level: key.Level, Stats: [5]uint32{v.Str, v.Agi, v.Sta, v.Inte, v.Spi}}
	}
	for key, v := range db.PlayerClassLevelStats {
		if key.Level > maxLevel {
			continue
		}
		if _, ok := c.classes[key.Class]; !ok || key.Level == 0 {
			return fmt.Errorf("invalid class stats")
		}
		ck := classLevelKey{key.Class, key.Level}
		if _, ok := c.classLevels[ck]; ok {
			return fmt.Errorf("duplicate class stats")
		}
		c.classLevels[ck] = ClassLevel{Class: key.Class, Level: key.Level, Health: v.BaseHP, Mana: v.BaseMana}
	}
	for level, next := range db.PlayerXPForLevel {
		if level == 0 || level >= maxLevel {
			continue
		}
		if next == 0 {
			return fmt.Errorf("invalid XP row")
		}
		if _, ok := c.xp[level]; ok {
			return fmt.Errorf("duplicate XP level")
		}
		c.xp[level] = next
	}
	return nil
}

func (c *Catalog) validateCompleteness(db *entity.Dataset, maxLevel uint32) error {
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
	for name := range db.ReservedNames {
		if !utf8.ValidString(name) || name == "" {
			return fmt.Errorf("invalid reserved_name")
		}
		c.reserved[strings.ToLower(name)] = true
	}
	return nil
}
