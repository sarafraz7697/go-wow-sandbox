package gamedata

import (
	"wowserver/internal/entity"
)

// ResolvedCreatureSpawn is the outcome of resolving one creature spawn
// against its template at "spawn time", the way the reference does inside
// Creature::LoadFromDB + Creature::SelectLevel before the object becomes
// visible. The codec turns this directly into the CREATE_OBJECT block.
type ResolvedCreatureSpawn struct {
	GUID           uint32
	Entry          uint32
	X, Y, Z        float32 // spawn position (creature.position_x/y/z)
	Orientation    float32
	DisplayID      uint32 // modelid_override when set, else template ModelId[0]
	Level          uint32 // template range resolved per spawn
	Health         uint32 // SelectLevel result
	Mana           uint32 // SelectLevel result
	MaxHealth      uint32
	MaxMana        uint32
	Faction        uint32 // template FactionAlliance (faction_A)
	UnitFlags      uint32
	NpcFlags       uint32
	Scale          float32 // template Scale, min 1 like DEFAULT_OBJECT_SCALE fallback
	BoundingRadius float32 // creature_model_info of the display, fallback 0.5
	CombatReach    float32 // creature_model_info of the display, fallback 1.5
	SpeedWalk      float32 // 2.5 * template SpeedWalk (baseMoveSpeed[MOVE_WALK])
	SpeedRun       float32 // 7.0 * template SpeedRun (baseMoveSpeed[MOVE_RUN])
}

// ResolveCreatureSpawn ports Creature::SelectLevel (CreatureLevel.cpp) for
// a spawn: the level is picked from the template's MinLevel..MaxLevel
// range, health/mana come from creature_template_classlevelstats scaled by
// the template multipliers, and when the class-level row is missing the
// reference's legacy interpolation between MinLevelHealth/MaxLevelHealth
// applies.
//
// One deliberate deviation: the reference draws the level with urand()
// inside the range; this port derives it deterministically from the spawn
// GUID so every startup shows the same world and tests are reproducible.
func (c *Catalog) ResolveCreatureSpawn(ref CreatureSpawnRef) (ResolvedCreatureSpawn, bool) {
	info, ok := c.creatureTemplates[ref.Data.ID]
	if !ok {
		return ResolvedCreatureSpawn{}, false
	}

	minLevel, maxLevel := info.MinLevel, info.MaxLevel
	level := minLevel
	if maxLevel > minLevel {
		// Deterministic stand-in for urand(minlevel, maxlevel).
		level = minLevel + ref.GUID%(maxLevel-minLevel+1)
	}

	var health, mana uint32
	if cls, ok := c.CreatureClassLvlStats(level, info.UnitClass, info.Expansion); ok {
		// Use Creature Stats to calculate stat values.
		health = uint32(float32(cls.BaseHealth) * info.HealthMultiplier)
		mana = uint32(float32(cls.BaseMana) * info.PowerMultiplier)
	} else {
		// Use old style to calculate stat values.
		relLevel := float32(0)
		if maxLevel != minLevel {
			relLevel = float32(level-minLevel) / float32(maxLevel-minLevel)
		}
		minHealth, maxHealth := minOf(info.MaxLevelHealth, info.MinLevelHealth), maxOf(info.MaxLevelHealth, info.MinLevelHealth)
		health = minHealth + uint32(relLevel*float32(maxHealth-minHealth))
		minMana, maxMana := minOf(info.MaxLevelMana, info.MinLevelMana), maxOf(info.MaxLevelMana, info.MinLevelMana)
		mana = minMana + uint32(relLevel*float32(maxMana-minMana))
	}
	if health < 1 {
		health = 1
	}

	displayID := info.ModelId[0]
	if ref.Data.ModelIDOverride > 0 {
		displayID = ref.Data.ModelIDOverride
	}

	bounding, reach := float32(0.5), float32(1.5)
	if model, ok := c.creatureModels[displayID]; ok {
		if model.BoundingRadius > 0 {
			bounding = model.BoundingRadius
		}
		if model.CombatReach > 0 {
			reach = model.CombatReach
		}
	}

	scale := info.Scale
	if scale <= 0 {
		scale = 1
	}
	// Unit::UpdateModelData: "we expect values in database to be relative to
	// scale = 1.0", so both are multiplied by the object scale.
	bounding *= scale
	reach *= scale
	// Base movement speeds from Unit.cpp baseMoveSpeed: walk 2.5, run 7.0.
	const baseWalk, baseRun = 2.5, 7.0
	walk := float32(baseWalk) * info.SpeedWalk
	run := float32(baseRun) * info.SpeedRun

	curHealth := ref.Data.CurHealth
	if curHealth == 0 || curHealth > health {
		curHealth = health
	}
	curMana := ref.Data.CurMana
	if curMana > mana {
		curMana = mana
	}

	return ResolvedCreatureSpawn{
		GUID:           ref.GUID,
		Entry:          ref.Data.ID,
		X:              ref.Data.PosX,
		Y:              ref.Data.PosY,
		Z:              ref.Data.PosZ,
		Orientation:    ref.Data.Orientation,
		DisplayID:      displayID,
		Level:          level,
		Health:         curHealth,
		Mana:           curMana,
		MaxHealth:      health,
		MaxMana:        mana,
		Faction:        info.FactionAlliance,
		UnitFlags:      info.UnitFlags,
		NpcFlags:       info.NpcFlags,
		Scale:          scale,
		BoundingRadius: bounding,
		CombatReach:    reach,
		SpeedWalk:      walk,
		SpeedRun:       run,
	}, true
}

func minOf(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}
func maxOf(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}

// GameObjectTemplateForSpawn returns the template plus the resolved
// display fields for a gameobject spawn; kept separate because gameobject
// resolution is a plain template lookup (no SelectLevel equivalent).
func (c *Catalog) ResolveGameObjectSpawn(ref GameObjectSpawnRef) (entity.GameObjectInfo, bool) {
	info, ok := c.goTemplates[ref.Data.ID]
	if !ok {
		return entity.GameObjectInfo{}, false
	}
	return info, true
}
