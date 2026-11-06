// Package gamedata holds immutable startup data. It has no network or SQL
// dependency. Constructors copy inputs; accessors never expose internal maps.
//
// The world-database entities live in internal/entity (MaNGOS-one parity
// structs loaded by internal/worldload). The catalog consumes an
// entity.Dataset once at startup and exposes typed lookups plus the
// client-side DBC data.
package gamedata

import (
	"sort"

	"wowserver/internal/entity"
)

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

// Start, StartItem, StartSpell, Action, Level, ClassLevel and XP are the
// catalog views over the entity.PlayerCreate* tables. The entity structs
// in internal/entity are the raw MaNGOS-parity rows; these views keep the
// creation pipeline's compact field names.
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

// CreatureSpawnRef pairs a spawn GUID with its CreatureData the way the
// reference's mCreatureDataMap (guid -> CreatureData) does.
type CreatureSpawnRef struct {
	GUID uint32
	Data entity.CreatureData
}

// GameObjectSpawnRef pairs a spawn GUID with its GameObjectData.
type GameObjectSpawnRef struct {
	GUID uint32
	Data entity.GameObjectData
}

// Creature is the pet lookup view over creature_template (ID + Family),
// kept for the character list's pet info line.
type Creature struct{ ID, Family uint32 }

type ClientData struct {
	Cooldowns    []SpellCooldown
	Factions     []Faction
	// FactionTemplates carries every row of FactionTemplate.dbc.
	FactionTemplates []FactionTemplate
	Enchantments []Enchantment
	Items        []ClientItem
	Races        []Race
	Classes      []Class
	Maps         []Map
	Pairs        []Pair
	Outfits      []Outfit
	SpellIDs     []uint32
	// AreaTriggers carries every row of AreaTrigger.dbc. The catalog filters
	// this against the dataset's areatrigger_tavern rows to keep only the
	// inn/tavern volumes.
	AreaTriggers []TavernTrigger
}
type Summary struct {
	Races, Classes, Maps, Pairs, Outfits, Spells, Items, StartItems, StartSpells, Actions, Levels, ClassLevels, XP, Reserved, SkippedZeroItems, Taverns, CreatureTemplates, GameObjectTemplates, Quests int
}
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
	cooldowns         map[uint32]SpellCooldown
	itemPrototypes    map[uint32]entity.ItemPrototype
	factionIndices    map[uint32]int32
	factions          map[uint32]Faction
	factionTemplates  map[uint32]FactionTemplate
	vendorItems       map[uint32][]entity.VendorItem
	vendorTemplates   map[uint32][]entity.VendorItem
	creatureByGUID    map[uint32]int
	conditions        map[uint32]entity.PlayerCondition
	enchantments      map[uint32]Enchantment
	creatures         map[uint32]Creature
	creatureTemplates map[uint32]entity.CreatureInfo
	goTemplates       map[uint32]entity.GameObjectInfo
	quests            map[uint32]*entity.Quest
	creatureModels    map[uint32]entity.CreatureModelInfo
	classLevelStats   map[entity.CreatureClassLevelKey]entity.CreatureClassLvlStats
	creatureSpawns    []CreatureSpawnRef
	goSpawns          []GameObjectSpawnRef
	skippedZeroItems  int
	races             map[uint32]Race
	classes           map[uint32]Class
	maps              map[uint32]Map
	pairs             map[Pair]bool
	starts            map[Pair]Start
	outfits           map[outfitKey]Outfit
	spells            map[uint32]bool
	startItems        map[Pair][]StartItem
	startSpells       map[Pair][]uint32
	actions           map[Pair][]Action
	levels            map[levelKey]Level
	classLevels       map[classLevelKey]ClassLevel
	xp                map[uint32]uint32
	reserved          map[string]bool
	taverns           map[uint32]TavernTrigger
	summary           Summary
}

func (c *Catalog) Summary() Summary              { return c.summary }
func (c *Catalog) Race(id uint32) (Race, bool)   { v, ok := c.races[id]; return v, ok }
func (c *Catalog) Class(id uint32) (Class, bool) { v, ok := c.classes[id]; return v, ok }
func (c *Catalog) Map(id uint32) (Map, bool)     { v, ok := c.maps[id]; return v, ok }
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

// CreatureSpawnsNear returns all creature spawns on the given map whose
// position is within radius of (x, y, z). Used by the protocol layer to
// build CREATE_OBJECT blocks for nearby creatures when a player enters
// the world or moves into visibility range.
func (c *Catalog) CreatureSpawnsNear(mapID uint32, x, y, z, radius float32) []CreatureSpawnRef {
	radius2 := radius * radius
	out := make([]CreatureSpawnRef, 0, 32)
	for _, s := range c.creatureSpawns {
		if s.Data.MapID != mapID {
			continue
		}
		dx, dy, dz := s.Data.PosX-x, s.Data.PosY-y, s.Data.PosZ-z
		if dx*dx+dy*dy+dz*dz <= radius2 {
			out = append(out, s)
		}
	}
	return out
}

// GameObjectSpawnsNear returns all gameobject spawns on the given map whose
// position is within radius of (x, y, z).
func (c *Catalog) GameObjectSpawnsNear(mapID uint32, x, y, z, radius float32) []GameObjectSpawnRef {
	radius2 := radius * radius
	out := make([]GameObjectSpawnRef, 0, 32)
	for _, s := range c.goSpawns {
		if s.Data.MapID != mapID {
			continue
		}
		dx, dy, dz := s.Data.PosX-x, s.Data.PosY-y, s.Data.PosZ-z
		if dx*dx+dy*dy+dz*dz <= radius2 {
			out = append(out, s)
		}
	}
	return out
}

// CreatureTemplateCount, GameObjectTemplateCount, CreatureSpawnCount,
// GameObjectSpawnCount return the number of loaded entries. Used for
// startup logging.
func (c *Catalog) CreatureTemplateCount() int   { return len(c.creatureTemplates) }
func (c *Catalog) GameObjectTemplateCount() int { return len(c.goTemplates) }
func (c *Catalog) CreatureSpawnCount() int      { return len(c.creatureSpawns) }
func (c *Catalog) GameObjectSpawnCount() int    { return len(c.goSpawns) }
