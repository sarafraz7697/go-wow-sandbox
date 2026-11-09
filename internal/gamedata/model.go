// Package gamedata holds immutable startup data. It has no network or SQL
// dependency. Constructors copy inputs; accessors never expose internal maps.
//
// The world-database entities live in internal/entity (MaNGOS-one parity
// structs loaded by internal/worldload). The catalog consumes an
// entity.Dataset once at startup and exposes typed lookups plus the
// client-side DBC data.
package gamedata

import (
	"sync"
	"sort"

	"wowserver/internal/entity"
)

type Pair struct{ Race, Class uint32 }
type Race struct {
	ID, Faction, MaleDisplay, FemaleDisplay, Expansion uint32
	// Team is ChrRaces.BaseLanguage: 7 = Alliance, 1 = Horde. Matches
	// Player::TeamForRace in the MaNGOS reference.
	Team uint32
	// Cinematic is ChrRaces.CinematicSequenceID.
	Cinematic uint32
	Name      string
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
	// RegenHP, RegenHPSpirit, RegenMPSpirit are the GameTables
	// gtOCTRegenHP, gtRegenHPPerSpt and gtRegenMPPerSpt (one float per
	// (class-1)*100+level-1). Nil when a table is not shipped.
	RegenHP, RegenHPSpirit, RegenMPSpirit []float32
	// MeleeCritBase and MeleeCritRatio are gtChanceToMeleeCritBase (per
	// class) and gtChanceToMeleeCrit (class*100+level), GT_MAX_LEVEL = 100.
	MeleeCritBase, MeleeCritRatio []float32
	// SpellCritBase and SpellCritRatio are gtChanceToSpellCritBase / gtChanceToSpellCrit.
	SpellCritBase, SpellCritRatio []float32
	// CombatRatings is gtCombatRatings (one float per row, row index
	// cr*GT_MAX_LEVEL + level-1). Nil when the table is not shipped.
	CombatRatings []float32
	// Spells, CastTimes and Durations feed casting (Spell.dbc, SpellCastTimes.dbc, SpellDuration.dbc).
	Spells    []SpellInfo
	CastTimes map[uint32]CastTime
	Durations map[uint32]Duration
	// SpellRanges is SpellRange.dbc.
	SpellRanges map[uint32]SpellRange
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
	maxLevel uint32 // highest level stored in the progression tables
	catOnce sync.Once
	byCat   map[uint32][]uint32 // spell category -> spell ids (sSpellCategoryStore)
	meleeCritBase, meleeCritRatio []float32
	spellCritBase, spellCritRatio []float32
	combatRatings []float32
	regenHP, regenHPSpirit, regenMPSpirit []float32
	spellInfo map[uint32]*SpellInfo
	castTimes map[uint32]CastTime
	durations map[uint32]Duration
	spellRanges map[uint32]SpellRange
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
	spatial           *spatialIndex
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
// Level ports ObjectMgr::GetPlayerLevelInfo: levels above the stored table
// are extrapolated from the last stored level (BuildPlayerLevelInfo).
func (c *Catalog) Level(race, class, level uint32) (Level, bool) {
	if level < 1 {
		return Level{}, false
	}
	pair := Pair{race, class}
	if level <= c.maxLevel {
		v, ok := c.levels[levelKey{pair, level}]
		return v, ok
	}
	v, ok := c.levels[levelKey{pair, c.maxLevel}]
	if !ok {
		return Level{}, false
	}
	v.Level = level
	for lvl := int(c.maxLevel) - 1; lvl < int(level); lvl++ {
		for i, add := range extrapolatedStatGain(class, lvl) {
			v.Stats[i] += add
		}
	}
	return v, true
}

// ClassLevel ports GetPlayerClassLevelInfo: levels above the table use the
// last stored row.
func (c *Catalog) ClassLevel(class, level uint32) (ClassLevel, bool) {
	if level < 1 {
		return ClassLevel{}, false
	}
	if level > c.maxLevel {
		level = c.maxLevel
	}
	v, ok := c.classLevels[classLevelKey{class, level}]
	return v, ok
}

// extrapolatedStatGain is the per-level stat gain of BuildPlayerLevelInfo,
// ordered strength, agility, stamina, intellect, spirit.
func extrapolatedStatGain(class uint32, lvl int) [5]uint32 {
	b := func(c bool) uint32 {
		if c {
			return 1
		}
		return 0
	}
	// pick(a, b, c) = (lvl > a ? 2 : (lvl > b ? 1 : 0)) style helpers
	odd, even := lvl%2 != 0, lvl%2 == 0
	var str, sta, agi, intel, spi uint32
	switch class {
	case 1: // warrior
		if lvl > 23 {
			str, sta = 2, 2
		} else if lvl > 1 {
			str, sta = 1, 1
		}
		agi = b(lvl > 36 || (lvl > 6 && odd))
		intel = b(lvl > 9 && even)
		spi = b(lvl > 9 && even)
	case 2: // paladin
		str = b(lvl > 3)
		if lvl > 33 {
			sta = 2
		} else if lvl > 1 {
			sta = 1
		}
		agi = b(lvl > 38 || (lvl > 7 && even))
		intel = b(lvl > 6 && odd)
		spi = b(lvl > 7)
	case 3: // hunter
		str, sta = b(lvl > 4), b(lvl > 4)
		if lvl > 33 {
			agi = 2
		} else if lvl > 1 {
			agi = 1
		}
		intel = b(lvl > 8 && odd)
		spi = b(lvl > 38 || (lvl > 9 && even))
	case 4: // rogue
		str, sta = b(lvl > 5), b(lvl > 4)
		if lvl > 16 {
			agi = 2
		} else if lvl > 1 {
			agi = 1
		}
		intel = b(lvl > 8 && even)
		spi = b(lvl > 38 || (lvl > 9 && even))
	case 5: // priest
		str = b(lvl > 9 && even)
		sta = b(lvl > 5)
		agi = b(lvl > 38 || (lvl > 8 && odd))
		if lvl > 22 {
			intel = 2
		} else if lvl > 1 {
			intel = 1
		}
		spi = b(lvl > 3)
	case 7: // shaman
		str = b(lvl > 34 || (lvl > 6 && odd))
		sta = b(lvl > 4)
		agi = b(lvl > 7 && even)
		intel = b(lvl > 5)
		spi = b(lvl > 4)
	case 8: // mage
		str = b(lvl > 9 && even)
		sta = b(lvl > 5)
		agi = b(lvl > 9 && even)
		if lvl > 24 {
			intel = 2
		} else if lvl > 1 {
			intel = 1
		}
		if lvl > 33 {
			spi = 2
		} else if lvl > 2 {
			spi = 1
		}
	case 9: // warlock
		str = b(lvl > 9 && even)
		agi = b(lvl > 9 && even)
		if lvl > 38 {
			sta, spi = 2, 2
		} else if lvl > 3 {
			sta, spi = 1, 1
		}
		if lvl > 33 {
			intel = 2
		} else if lvl > 2 {
			intel = 1
		}
	case 11: // druid
		if lvl > 38 {
			str, agi, intel, spi = 2, 2, 3, 3
		} else {
			str = b(lvl > 6 && odd)
			agi = b(lvl > 8 && odd)
			intel = b(lvl > 4)
			spi = b(lvl > 5)
		}
		if lvl > 32 {
			sta = 2
		} else if lvl > 4 {
			sta = 1
		}
	}
	return [5]uint32{str, agi, sta, intel, spi}
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
	if c.spatial != nil {
		x0, x1, y0, y1 := cellRange(x, y, radius)
		for cx := x0; cx <= x1; cx++ {
			for cy := y0; cy <= y1; cy++ {
				for _, i := range c.spatial.creatures[spatialKey{mapID, cx, cy}] {
					s := c.creatureSpawns[i]
					dx, dy, dz := s.Data.PosX-x, s.Data.PosY-y, s.Data.PosZ-z
					if dx*dx+dy*dy+dz*dz <= radius2 {
						out = append(out, s)
					}
				}
			}
		}
		return out
	}
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
	if c.spatial != nil {
		x0, x1, y0, y1 := cellRange(x, y, radius)
		for cx := x0; cx <= x1; cx++ {
			for cy := y0; cy <= y1; cy++ {
				for _, i := range c.spatial.objects[spatialKey{mapID, cx, cy}] {
					s := c.goSpawns[i]
					dx, dy, dz := s.Data.PosX-x, s.Data.PosY-y, s.Data.PosZ-z
					if dx*dx+dy*dy+dz*dz <= radius2 {
						out = append(out, s)
					}
				}
			}
		}
		return out
	}
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

// RegenRatios returns gtOCTRegenHP, gtRegenHPPerSpt and gtRegenMPPerSpt for a
// class and level (GT_MAX_LEVEL = 100); a missing table or row yields 0.
func (c *Catalog) RegenRatios(class, level uint32) (hpBase, hpSpirit, mpSpirit float32) {
	if level > 100 {
		level = 100
	}
	if class == 0 || level == 0 {
		return
	}
	i := int((class-1)*100 + level - 1)
	at := func(t []float32) float32 {
		if i < len(t) {
			return t[i]
		}
		return 0
	}
	return at(c.regenHP), at(c.regenHPSpirit), at(c.regenMPSpirit)
}

// MeleeCritFromAgility ports Player::GetMeleeCritFromAgility (percent).
func (c *Catalog) MeleeCritFromAgility(class, level uint32, agility uint32) float32 {
	const gtMaxLevel = 100
	if class == 0 {
		return 0
	}
	level = min(level, gtMaxLevel)
	bi, ri := int(class-1), int((class-1)*gtMaxLevel+level-1)
	if bi >= len(c.meleeCritBase) || ri >= len(c.meleeCritRatio) {
		return 0
	}
	return (c.meleeCritBase[bi] + float32(agility)*c.meleeCritRatio[ri]) * 100
}

// SpellCritFromIntellect ports Player::GetSpellCritFromIntellect (percent).
func (c *Catalog) SpellCritFromIntellect(class, level uint32, intellect uint32) float32 {
	const gtMaxLevel = 100
	if class == 0 {
		return 0
	}
	level = min(level, gtMaxLevel)
	bi, ri := int(class-1), int((class-1)*gtMaxLevel+level-1)
	if bi >= len(c.spellCritBase) || ri >= len(c.spellCritRatio) {
		return 0
	}
	return (c.spellCritBase[bi] + float32(intellect)*c.spellCritRatio[ri]) * 100
}

// Combat rating indexes (CR_* of Unit.h enum CombatRating): the order of
// the PLAYER_FIELD_COMBAT_RATING_1 block.
const (
	CRWeaponSkill = iota
	CRDefenseSkill
	CRDodge
	CRParry
	CRBlock
	CRHitMelee
	CRHitRanged
	CRHitSpell
	CRCritMelee
	CRCritRanged
	CRCritSpell
	CRHitTakenMelee
	CRHitTakenRanged
	CRHitTakenSpell
	CRCritTakenMelee
	CRCritTakenRanged
	CRCritTakenSpell
	CRHasteMelee
	CRHasteRanged
	CRHasteSpell
	CRWeaponSkillMainHand
	CRWeaponSkillOffHand
	CRWeaponSkillRanged
	CRExpertise
)

// ItemWeaponSkills is item_weapon_skills[] of Item::GetSkill, indexed by
// weapon subclass; 0 entries have no weapon skill of their own.
var ItemWeaponSkills = [21]uint32{44, 172, 45, 46, 54, 160, 229, 43, 55, 0, 136, 0, 0, 162, 0, 173, 176, 253, 226, 228, 356}

// RatingMultiplier ports Player::GetRatingMultiplier: 1 / gtCombatRatings
// at row cr*GT_MAX_LEVEL + level-1. A missing table or row yields 1 like
// the reference's failed LookupEntry.
func (c *Catalog) RatingMultiplier(cr, level uint32) float32 {
	const gtMaxLevel = 100
	if level > gtMaxLevel {
		level = gtMaxLevel
	}
	i := int(cr)*gtMaxLevel + int(level) - 1
	if i < 0 || i >= len(c.combatRatings) || c.combatRatings[i] == 0 {
		return 1
	}
	return 1 / c.combatRatings[i]
}

// RatingBonus ports Player::GetRatingBonusValue: rating points converted
// to their percentage (or skill/expertise) bonus at the given level.
func (c *Catalog) RatingBonus(cr, level uint32, rating int32) float32 {
	return float32(rating) * c.RatingMultiplier(cr, level)
}

// dodgeBase and dodgeCritMul are the hardcoded tables of
// Player::GetDodgeFromAgility, indexed by class-1.
var (
	dodgeBase   = [11]float32{0.0075, 0.00652, -0.0545, -0.0059, 0.03183, 0.0114, 0.0167, 0.034575, 0.02011, 0, -0.0187}
	dodgeCritMul = [11]float32{1.1, 1.0, 1.6, 2.0, 1.0, 1.0, 1.0, 1.0, 1.0, 0, 1.7}
)

// DodgeFromAgility ports Player::GetDodgeFromAgility (percent): the class
// base plus agility times the crit-per-agility ratio with the class dodge
// multiplier.
func (c *Catalog) DodgeFromAgility(class, level, agility uint32) float32 {
	const gtMaxLevel = 100
	if class == 0 || class > 11 {
		return 0
	}
	level = min(level, gtMaxLevel)
	ri := int(class-1)*gtMaxLevel + int(level) - 1
	if ri >= len(c.meleeCritRatio) {
		return 0
	}
	dodge := dodgeBase[class-1] + float32(agility)*c.meleeCritRatio[ri]*dodgeCritMul[class-1]
	return dodge * 100
}

// PlayerCanParry reports whether any known spell grants the parry ability
// (SPELL_EFFECT_PARRY learned through Player::SetCanParry).
func (c *Catalog) PlayerCanParry(spells []uint32) bool { return c.playerHasSpellEffect(spells, 22) }

// PlayerCanBlock reports whether any known spell grants the block ability
// (SPELL_EFFECT_BLOCK learned through Player::SetCanBlock).
func (c *Catalog) PlayerCanBlock(spells []uint32) bool { return c.playerHasSpellEffect(spells, 23) }

func (c *Catalog) playerHasSpellEffect(spells []uint32, effect uint32) bool {
	for _, id := range spells {
		if info := c.Spell(id); info != nil {
			for _, e := range info.Effect {
				if e == effect {
					return true
				}
			}
		}
	}
	return false
}
