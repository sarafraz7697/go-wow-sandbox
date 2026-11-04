package gamedata

import "fmt"

type Enchantment struct {
	ID, Visual                  uint32
	Effects, Amounts, Arguments [3]uint32
	Condition                   uint32
}

// CreatureTemplate is the static creature_template row. Only fields needed
// for the CREATE_OBJECT update block and CMSG_CREATURE_QUERY_RESPONSE are
// loaded; the full template has ~50 fields.
//
// Reference: CreatureInfo in Creature.h, creature_template table.
type CreatureTemplate struct {
	ID              uint32
	Name            string
	SubName         string
	MinLevel        uint32
	MaxLevel        uint32
	ModelID         uint32 // first model from ModelId[0]
	FactionAlliance uint32
	FactionHorde    uint32
	NPCFlags        uint32
	UnitFlags       uint32
	CreatureType    uint32
	Family          uint32
	Rank            uint32
	// Health/mana at min and max level (from creature_template).
	MinHealth, MaxHealth uint32
	MinMana, MaxMana     uint32
}

// GameObjectTemplate is the static gameobject_template row. Only fields
// needed for the CREATE_OBJECT update block and CMSG_GAMEOBJECT_QUERY_RESPONSE
// are loaded.
//
// Reference: GameObjectInfo in GameObject.h, gameobject_template table.
type GameObjectTemplate struct {
	ID        uint32
	Type      uint32 // GAMEOBJECT_TYPE_* from SharedDefines.h
	DisplayID uint32
	Name      string
	Faction   uint32
	Flags     uint32
	Size      float32
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
		if v.ID == 0 {
			continue
		}
		if _, ok := c.creatureTemplates[v.ID]; ok {
			return fmt.Errorf("duplicate creature_template %d", v.ID)
		}
		c.creatureTemplates[v.ID] = v
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
