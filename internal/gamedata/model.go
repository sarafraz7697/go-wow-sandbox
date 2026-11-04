// Package gamedata holds immutable startup data. It has no network or SQL
// dependency. Constructors copy inputs; accessors never expose internal maps.
package gamedata

import "sort"

type Pair struct{ Race, Class uint32 }
type Race struct {
	ID, Faction, MaleDisplay, FemaleDisplay, Expansion uint32
	// Team is ChrRaces.BaseLanguage: 7 = Alliance, 1 = Horde. Matches
	// Player::TeamForRace in the MaNGOS reference.
	Team uint32
	Name string
}
type Class struct {
	ID, Power, SpellFamily uint32
	Name                   string
}
type Map struct{ ID, Type uint32 }

// TavernTrigger is the in-game volume that puts a player in REST_TYPE_IN_TAVERN
// when entered. It comes from AreaTrigger.dbc, filtered by the rows the world
// DB's areatrigger_tavern table lists.
type TavernTrigger struct {
	ID               uint32
	Map              uint32
	X, Y, Z, Radius  float32
	BoxX, BoxY, BoxZ float32
	BoxOrientation   float32
}
type Outfit struct {
	Pair
	Gender, ID uint8
	Items      [12]int32
}
type Item struct {
	ID, Display, InventoryType                                      uint32
	Class, Subclass, BuyCount, MaxDurability, Bonding, FoodCategory uint32
	Stackable                                                       int32
	Duration                                                        uint32
	Charges                                                         [5]int32
}
type Start struct {
	Pair
	Map, Zone            uint32
	X, Y, Z, Orientation float32
}
type StartItem struct {
	Pair
	Item, Amount uint32
}
type StartSpell struct {
	Pair
	Spell uint32
}
type Action struct {
	Pair
	Button, Action, Type uint32
}
type Level struct {
	Pair
	Level uint32
	Stats [5]uint32
}
type ClassLevel struct{ Class, Level, Health, Mana uint32 }
type XP struct{ Level, Next uint32 }
type Rows struct {
	ItemDetails []ItemDetails
	Creatures   []Creature
	Starts      []Start
	Items       []Item
	StartItems  []StartItem
	Spells      []StartSpell
	Actions     []Action
	Levels      []Level
	ClassLevels []ClassLevel
	XP          []XP
	Reserved    []string
	// TavernTriggerIDs are the ids listed in the world DB's areatrigger_tavern
	// table. The catalog joins them against AreaTrigger.dbc to produce
	// TavernTrigger rows.
	TavernTriggerIDs []uint32
}
type ClientData struct {
	Cooldowns    []SpellCooldown
	Factions     []Faction
	Enchantments []Enchantment
	Items        []Item
	Races        []Race
	Classes      []Class
	Maps         []Map
	Pairs        []Pair
	Outfits      []Outfit
	SpellIDs     []uint32
	// AreaTriggers carries every row of AreaTrigger.dbc. The catalog filters
	// this against Rows.TavernTriggerIDs to keep only the inn/tavern volumes.
	AreaTriggers []TavernTrigger
}
type Summary struct{ Races, Classes, Maps, Pairs, Outfits, Spells, Items, StartItems, StartSpells, Actions, Levels, ClassLevels, XP, Reserved, SkippedZeroItems, Taverns int }
type levelKey struct {
	Pair
	Level uint32
}
type classLevelKey struct{ Class, Level uint32 }
type outfitKey struct {
	Pair
	Gender, ID uint8
}
type Catalog struct {
	cooldowns        map[uint32]SpellCooldown
	itemDetails      map[uint32]ItemDetails
	factionIndices   map[uint32]int32
	enchantments     map[uint32]Enchantment
	creatures        map[uint32]Creature
	skippedZeroItems int
	races            map[uint32]Race
	classes          map[uint32]Class
	maps             map[uint32]Map
	pairs            map[Pair]bool
	starts           map[Pair]Start
	outfits          map[outfitKey]Outfit
	items            map[uint32]Item
	spells           map[uint32]bool
	startItems       map[Pair][]StartItem
	startSpells      map[Pair][]uint32
	actions          map[Pair][]Action
	levels           map[levelKey]Level
	classLevels      map[classLevelKey]ClassLevel
	xp               map[uint32]uint32
	reserved         map[string]bool
	taverns          map[uint32]TavernTrigger
	summary          Summary
}

func (c *Catalog) Summary() Summary              { return c.summary }
func (c *Catalog) Race(id uint32) (Race, bool)   { v, ok := c.races[id]; return v, ok }
func (c *Catalog) Class(id uint32) (Class, bool) { v, ok := c.classes[id]; return v, ok }
func (c *Catalog) Map(id uint32) (Map, bool)     { v, ok := c.maps[id]; return v, ok }
func (c *Catalog) Item(id uint32) (Item, bool)   { v, ok := c.items[id]; return v, ok }
func (c *Catalog) TavernTrigger(id uint32) (TavernTrigger, bool) {
	v, ok := c.taverns[id]
	return v, ok
}
func (c *Catalog) Start(race, class uint32) (Start, bool) {
	v, ok := c.starts[Pair{race, class}]
	return v, ok
}
func (c *Catalog) Outfit(race, class uint32, gender, id uint8) (Outfit, bool) {
	v, ok := c.outfits[outfitKey{Pair{race, class}, gender, id}]
	return v, ok
}
func (c *Catalog) Level(race, class, level uint32) (Level, bool) {
	v, ok := c.levels[levelKey{Pair{race, class}, level}]
	return v, ok
}
func (c *Catalog) ClassLevel(class, level uint32) (ClassLevel, bool) {
	v, ok := c.classLevels[classLevelKey{class, level}]
	return v, ok
}
func (c *Catalog) XP(level uint32) (uint32, bool) { v, ok := c.xp[level]; return v, ok }
func (c *Catalog) StartItems(race, class uint32) []StartItem {
	return append([]StartItem(nil), c.startItems[Pair{race, class}]...)
}
func (c *Catalog) StartSpells(race, class uint32) []uint32 {
	return append([]uint32(nil), c.startSpells[Pair{race, class}]...)
}
func (c *Catalog) Actions(race, class uint32) []Action {
	return append([]Action(nil), c.actions[Pair{race, class}]...)
}

// RaceIDs and ClassIDs return the catalog's races/classes in ascending id
// order. Creation uses them to scan every playercreateinfo pair instead of
// hard-coding the Human Warrior pair.
func (c *Catalog) RaceIDs() []uint32 {
	out := make([]uint32, 0, len(c.races))
	for id := range c.races {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
func (c *Catalog) ClassIDs() []uint32 {
	out := make([]uint32, 0, len(c.classes))
	for id := range c.classes {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
