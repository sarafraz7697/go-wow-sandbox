package gamedata

import "fmt"

type Enchantment struct {
	ID, Visual                  uint32
	Effects, Amounts, Arguments [3]uint32
	Condition                   uint32
}

// MAX_CREATURE_MODEL matches MAX_CREATURE_MODEL in Creature.h: four
// alternative display models per template. SMSG_CREATURE_QUERY_RESPONSE
// sends all four; CREATE_OBJECT only needs the first.
const MAX_CREATURE_MODEL = 4

// MAX_GAMEOBJECT_TEMPLATE_DATA matches the 24 type-specific uint32 data
// slots (data0..data23) of GameObjectInfo in the mangos-one reference.
// SMSG_GAMEOBJECT_QUERY_RESPONSE ships the first 6 (24 bytes) raw as
// info->raw.data.
const MAX_GAMEOBJECT_TEMPLATE_DATA = 24

// CreatureTemplate mirrors the CreatureInfo struct in the mangos-one 2.4.3
// reference (Object/Creature.h). Fields the project does not yet consume
// (combat/loot/trainer data) are omitted; everything needed for CREATE_OBJECT,
// CMSG_CREATURE_QUERY_RESPONSE and creature-spawn validation is present.
//
// Field names and ordering follow the C++ struct so a side-by-side read
// against the reference is direct. The DB column names used by the loader
// match the project schema (CamelCase: ModelId1..4, FactionAlliance,
// FactionHorde, NpcFlags, UnitFlags, CreatureType, MinLevelHealth, ...).
//
// Reference: CreatureInfo in Creature.h, creature_template table; source
// format string CreatureInfosrcfmt in SQLStorages.cpp.
type CreatureTemplate struct {
	Entry             uint32
	Name              string
	SubName           string
	IconName          string
	MinLevel          uint32
	MaxLevel          uint32
	HeroicEntry       uint32
	ModelId           [MAX_CREATURE_MODEL]uint32
	FactionAlliance   uint32
	FactionHorde      uint32
	Scale             float32
	Family            uint32
	CreatureType      uint32
	InhabitType       uint32
	RegenerateStats   uint32
	RacialLeader      bool
	NpcFlags          uint32
	UnitFlags         uint32
	DynamicFlags      uint32
	ExtraFlags        uint32
	CreatureTypeFlags uint32
	SpeedWalk         float32
	SpeedRun          float32
	UnitClass         uint32
	Rank              uint32
	Expansion         int32
	HealthMultiplier  float32
	PowerMultiplier   float32
	MinLevelHealth    uint32
	MaxLevelHealth    uint32
	MinLevelMana      uint32
	MaxLevelMana      uint32
	PetSpellDataId    uint32
	MovementType      uint32
}

// GameObjectTemplate mirrors the GameObjectInfo struct in the mangos-one
// 2.4.3 reference (Object/GameObject.h). Data carries the type-specific
// slots (data0..data23) that SMSG_GAMEOBJECT_QUERY_RESPONSE ships raw as
// 24 bytes (info->raw.data). IconName and CastBarCaption are needed for
// the same response packet.
//
// Reference: GameObjectInfo in GameObject.h, gameobject_template table;
// source format string GameObjectInfosrcfmt in SQLStorages.cpp.
type GameObjectTemplate struct {
	ID             uint32
	Type           uint32 // GAMEOBJECT_TYPE_* from SharedDefines.h
	DisplayID      uint32
	Name           string
	IconName       string
	CastBarCaption string
	Faction        uint32
	Flags          uint32
	Size           float32
	Data           [MAX_GAMEOBJECT_TEMPLATE_DATA]uint32 // data0..data23
}

// CreatureSpawn is one row from the `creature` table (a spawn point).
// Reference: CreatureData in Creature.h.
type CreatureSpawn struct {
	GUID        uint32 // spawn GUID (low part)
	Entry       uint32 // creature_template entry
	Map         uint32
	X, Y, Z     float32
	Orientation float32
	Health      uint32
	Mana        uint32
}

// GameObjectSpawn is one row from the `gameobject` table (a spawn point).
// Reference: GameObjectData in GameObject.h.
type GameObjectSpawn struct {
	GUID                 uint32
	Entry                uint32
	Map                  uint32
	X, Y, Z              float32
	Orientation          float32
	Rotation0, Rotation1 float32
	Rotation2, Rotation3 float32
	State                uint32 // GO_STATE_ACTIVE etc.
	AnimProgress         uint32
}

// Creature keeps the old (ID, Family) for backward compat with pet lookups.
type Creature struct{ ID, Family uint32 }

func (c *Catalog) Enchantment(id uint32) (Enchantment, bool) {
	v, ok := c.enchantments[id]
	return v, ok
}
func (c *Catalog) Creature(id uint32) (Creature, bool) { v, ok := c.creatures[id]; return v, ok }
func (c *Catalog) CreatureTemplate(id uint32) (CreatureTemplate, bool) {
	v, ok := c.creatureTemplates[id]
	return v, ok
}
func (c *Catalog) GameObjectTemplate(id uint32) (GameObjectTemplate, bool) {
	v, ok := c.goTemplates[id]
	return v, ok
}
func (c *Catalog) loadVisuals(client ClientData, db Rows) error {
	c.enchantments = make(map[uint32]Enchantment, len(client.Enchantments))
	c.creatures = make(map[uint32]Creature, len(db.Creatures))
	c.creatureTemplates = make(map[uint32]CreatureTemplate, len(db.CreatureTemplates))
	c.goTemplates = make(map[uint32]GameObjectTemplate, len(db.GameObjectTemplates))
	for _, v := range client.Enchantments {
		if _, ok := c.enchantments[v.ID]; ok {
			return fmt.Errorf("duplicate enchantment %d", v.ID)
		}
		c.enchantments[v.ID] = v
	}
	for _, v := range db.Creatures {
		if v.ID == 0 {
			continue
		}
		if _, ok := c.creatures[v.ID]; ok {
			return fmt.Errorf("duplicate creature %d", v.ID)
		}
		c.creatures[v.ID] = v
	}
	for _, v := range db.CreatureTemplates {
		if v.Entry == 0 {
			continue
		}
		if _, ok := c.creatureTemplates[v.Entry]; ok {
			return fmt.Errorf("duplicate creature_template %d", v.Entry)
		}
		c.creatureTemplates[v.Entry] = v
	}
	for _, v := range db.GameObjectTemplates {
		if v.ID == 0 {
			continue
		}
		if _, ok := c.goTemplates[v.ID]; ok {
			return fmt.Errorf("duplicate gameobject_template %d", v.ID)
		}
		c.goTemplates[v.ID] = v
	}
	return nil
}
