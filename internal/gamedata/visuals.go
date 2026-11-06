package gamedata

import (
	"fmt"

	"wowserver/internal/entity"
)

// CreatureTemplate returns the full creature_template entity
// (entity.CreatureInfo — every field the reference holds).
func (c *Catalog) CreatureTemplate(id uint32) (entity.CreatureInfo, bool) {
	v, ok := c.creatureTemplates[id]
	return v, ok
}

// GameObjectTemplate returns the full gameobject_template entity
// (entity.GameObjectInfo including the data0..data23 block).
func (c *Catalog) GameObjectTemplate(id uint32) (entity.GameObjectInfo, bool) {
	v, ok := c.goTemplates[id]
	return v, ok
}

// ItemPrototype returns the full item_template entity (entity.ItemPrototype,
// the complete row — header, stats, damage, spells, sockets).
func (c *Catalog) ItemPrototype(id uint32) (entity.ItemPrototype, bool) {
	v, ok := c.itemPrototypes[id]
	return v, ok
}

// Quest returns a loaded quest_template entity.
func (c *Catalog) Quest(id uint32) (*entity.Quest, bool) {
	v, ok := c.quests[id]
	return v, ok
}

// CreatureModelInfo returns the creature_model_info row for a display id.
func (c *Catalog) CreatureModelInfo(modelID uint32) (entity.CreatureModelInfo, bool) {
	v, ok := c.creatureModels[modelID]
	return v, ok
}

// CreatureClassLvlStats ports ObjectMgr::GetCreatureClassLvlStats: the
// per-expansion base stats for a creature level/class, nil when the row
// is missing or incomplete (BaseHealth 0 or BaseDamage <= 0.1).
func (c *Catalog) CreatureClassLvlStats(level, unitClass uint32, expansion int32) (entity.CreatureClassLvlStats, bool) {
	if expansion < 0 {
		return entity.CreatureClassLvlStats{}, false
	}
	v, ok := c.classLevelStats[entity.CreatureClassLevelKey{Class: unitClass, Level: level, Expansion: expansion}]
	if !ok || v.BaseHealth == 0 || v.BaseDamage <= 0.1 {
		return entity.CreatureClassLvlStats{}, false
	}
	return v, true
}

// Creature keeps the (ID, Family) pet lookup view.
func (c *Catalog) Creature(id uint32) (Creature, bool) { v, ok := c.creatures[id]; return v, ok }

// loadWorldEntities copies the DB entity collections from the dataset into
// the catalog maps. Duplicate detection is the loader's job; the catalog
// re-checks cheaply and fails loudly on programming errors.
func (c *Catalog) loadWorldEntities(db *entity.Dataset) error {
	c.creatures = make(map[uint32]Creature, len(db.CreatureTemplates))
	c.creatureTemplates = make(map[uint32]entity.CreatureInfo, len(db.CreatureTemplates))
	c.goTemplates = make(map[uint32]entity.GameObjectInfo, len(db.GameObjectTemplates))
	c.quests = make(map[uint32]*entity.Quest, len(db.Quests))
	c.creatureModels = make(map[uint32]entity.CreatureModelInfo, len(db.CreatureModelInfos))
	c.classLevelStats = make(map[entity.CreatureClassLevelKey]entity.CreatureClassLvlStats, len(db.CreatureClassLvlStats))
	c.itemPrototypes = make(map[uint32]entity.ItemPrototype, len(db.ItemPrototypes))
	c.conditions = make(map[uint32]entity.PlayerCondition, len(db.Conditions))
	for id, v := range db.Conditions {
		c.conditions[id] = v
	}
	c.vendorItems = make(map[uint32][]entity.VendorItem, len(db.NpcVendors))
	for entry, items := range db.NpcVendors {
		c.vendorItems[entry] = append([]entity.VendorItem(nil), items...)
	}
	c.vendorTemplates = make(map[uint32][]entity.VendorItem, len(db.NpcVendorTemplates))
	for id, items := range db.NpcVendorTemplates {
		c.vendorTemplates[id] = append([]entity.VendorItem(nil), items...)
	}

	for id, v := range db.CreatureTemplates {
		if id == 0 {
			continue
		}
		c.creatureTemplates[id] = v
		// Pet-family view: family comes from the template like the
		// reference's GetPetFamily lookup chain.
		c.creatures[id] = Creature{ID: id, Family: v.Family}
	}
	for id, v := range db.GameObjectTemplates {
		if id == 0 {
			continue
		}
		c.goTemplates[id] = v
	}
	for id, v := range db.Quests {
		c.quests[id] = v
	}
	for id, v := range db.CreatureModelInfos {
		c.creatureModels[id] = v
	}
	for k, v := range db.CreatureClassLvlStats {
		c.classLevelStats[k] = v
	}

	// Spawns: keep every spawn the loaders accepted (missing templates
	// were already skipped there, mirroring LoadCreatures/LoadGameObjects).
	//
	// Only spawns the reference puts on the map grid at load time are kept:
	// ObjectMgr::LoadCreatures / LoadGameObjects call AddCreatureToGrid /
	// AddGameobjectToGrid only when `gameEvent == 0 && GuidPoolId == 0 &&
	// EntryPoolId == 0` — everything else (holiday event decorations, pooled
	// spawns) is owned by the GameEvent / Pool systems and appears only when
	// they activate it. Sending all of them is what produced the stray
	// platforms and NPCs. spawnMask must also include the regular (0)
	// difficulty bit, as AddCreatureToGrid iterates the mask bits.
	eventCreature := make(map[uint32]struct{})
	for _, guids := range db.GameEventCreatureGUIDs {
		for _, g := range guids {
			eventCreature[g] = struct{}{}
		}
	}
	eventGO := make(map[uint32]struct{})
	for _, guids := range db.GameEventGameObjectGUIDs {
		for _, g := range guids {
			eventGO[g] = struct{}{}
		}
	}
	c.creatureSpawns = make([]CreatureSpawnRef, 0, len(db.Creatures))
	for _, guid := range db.CreatureOrder {
		data, ok := db.Creatures[guid]
		if !ok {
			return fmt.Errorf("creature order references unknown guid %d", guid)
		}
		if _, ev := eventCreature[guid]; ev {
			continue
		}
		if _, pooled := db.PoolCreature[guid]; pooled {
			continue
		}
		if _, pooled := db.PoolCreatureTemplate[data.ID]; pooled {
			continue
		}
		if data.SpawnMask&1 == 0 {
			continue
		}
		c.creatureSpawns = append(c.creatureSpawns, CreatureSpawnRef{GUID: guid, Data: data})
	}
	c.creatureByGUID = make(map[uint32]int, len(c.creatureSpawns))
	for i, s := range c.creatureSpawns {
		c.creatureByGUID[s.GUID] = i
	}
	c.goSpawns = make([]GameObjectSpawnRef, 0, len(db.GameObjects))
	for _, guid := range db.GameObjectOrder {
		data, ok := db.GameObjects[guid]
		if !ok {
			return fmt.Errorf("gameobject order references unknown guid %d", guid)
		}
		if _, ev := eventGO[guid]; ev {
			continue
		}
		if _, pooled := db.PoolGameObject[guid]; pooled {
			continue
		}
		if _, pooled := db.PoolGameObjectTemplate[data.ID]; pooled {
			continue
		}
		if data.SpawnMask&1 == 0 {
			continue
		}
		// Transports (GAMEOBJECT_TYPE_MO_TRANSPORT) are driven by the
		// transport manager in the reference, not by static spawns.
		if tmpl, ok := db.GameObjectTemplates[data.ID]; ok && tmpl.Type == goTypeMOTransport {
			continue
		}
		c.goSpawns = append(c.goSpawns, GameObjectSpawnRef{GUID: guid, Data: data})
	}
	c.buildSpatial()
	return nil
}

// goTypeMOTransport is GAMEOBJECT_TYPE_MO_TRANSPORT (15).
const goTypeMOTransport = 15
