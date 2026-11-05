package worldload

import (
	"context"

	"wowserver/internal/entity"
)

// creatureSpawnLoader ports ObjectMgr::LoadCreatures (ObjectMgrCreatures.cpp):
// the `creature` table LEFT JOINed with game_event_creature,
// pool_creature and pool_creature_template, so every spawn carries its
// event and pool memberships like the reference does before it decides
// initial spawn state. The dataset keeps the raw CreatureData (keyed by
// guid, mirroring mCreatureDataMap) plus the event/pool side indexes.
func init() {
	Register(Loader{
		Name:       "creature",
		Order:      OrderCreatures,
		CountQuery: "SELECT COUNT(*) FROM `creature`",
		Load:       loadCreatures,
	})
}

func loadCreatures(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	const query = "SELECT `creature`.`guid`, `creature`.`id`, `map`, `modelid`," +
		"`equipment_id`, `position_x`, `position_y`, `position_z`, `orientation`, `spawntimesecs`, `spawndist`, `currentwaypoint`," +
		"`curhealth`, `curmana`, `DeathState`, `MovementType`, `spawnMask`, `event`," +
		"`pool_creature`.`pool_entry`, `pool_creature_template`.`pool_entry` " +
		"FROM `creature` " +
		"LEFT OUTER JOIN `game_event_creature` ON `creature`.`guid` = `game_event_creature`.`guid` " +
		"LEFT OUTER JOIN `pool_creature` ON `creature`.`guid` = `pool_creature`.`guid` " +
		"LEFT OUTER JOIN `pool_creature_template` ON `creature`.`id` = `pool_creature_template`.`id` " +
		"ORDER BY `creature`.`guid`"

	return scanTable(ctx, q, t, query, 5000, func(r scanner) error {
		var guid, event, guidPool, entryPool nullI32Raw
		var v entity.CreatureData
		var isDead nullBoolRaw
		if err := r.Scan(
			&guid, u32(&v.ID),
			u32(&v.MapID), u32(&v.ModelIDOverride),
			i32(&v.EquipmentID), f32(&v.PosX), f32(&v.PosY), f32(&v.PosZ), f32(&v.Orientation),
			u32(&v.SpawnTimeSecs), f32(&v.SpawnDist), u32(&v.CurrentWaypoint),
			u32(&v.CurHealth), u32(&v.CurMana), &isDead, u8(&v.MovementType), u8(&v.SpawnMask),
			&event, &guidPool, &entryPool,
		); err != nil {
			return err
		}
		v.IsDead = isDead.v

		// Reference validation, in order:
		// 1. spawn guid 0 is unusable;
		if guid.v <= 0 {
			t.Skipf("Table `creature` has creature with invalid GUID %d", guid.v)
			return nil
		}
		// 2. the creature entry must exist in creature_template (heroic
		//    side entries are skipped there, the same lookup applies);
		info, ok := dest.CreatureTemplates[v.ID]
		if !ok {
			t.Skipf("Table `creature` has creature (GUID: %d) with non existing creature entry %d", guid.v, v.ID)
			return nil
		}
		// 3. curhealth/curmana below the template floor are raised, like
		//    the reference's RegenerateStats/low-mana clamps;
		if info.RegenerateStats&0x1 != 0 && v.CurHealth < info.MinLevelHealth {
			t.Skipf("Table `creature` have creature (GUID: %d Entry: %d) with low current health (%d), MinLevelHealth=%d", guid.v, v.ID, v.CurHealth, info.MinLevelHealth)
			v.CurHealth = info.MinLevelHealth
		}
		if v.CurMana < info.MinLevelMana {
			t.Skipf("Table `creature` have creature (GUID: %d Entry: %d) with low current mana (%d), MinLevelMana=%d", guid.v, v.ID, v.CurMana, info.MinLevelMana)
			v.CurMana = info.MinLevelMana
		}
		if _, dup := dest.Creatures[uint32(guid.v)]; dup {
			t.Skipf("Table `creature` has duplicate GUID %d", guid.v)
			return nil
		}
		dest.Creatures[uint32(guid.v)] = v
		dest.CreatureOrder = append(dest.CreatureOrder, uint32(guid.v))
		// Event and pool side indexes (the reference feeds these into the
		// GameEventMgr and PoolSystem during grid loading).
		if event.v != 0 {
			dest.GameEventCreatureGUIDs[int16(event.v)] = append(dest.GameEventCreatureGUIDs[int16(event.v)], uint32(guid.v))
		}
		if guidPool.v > 0 {
			dest.PoolCreature[uint32(guid.v)] = uint32(guidPool.v)
		}
		if entryPool.v > 0 {
			dest.PoolCreatureTemplate[v.ID] = uint32(entryPool.v)
		}
		t.Stored(1)
		return nil
	})
}

// gameObjectSpawnLoader ports ObjectMgr::LoadGameObjects
// (ObjectMgrGameObjects.cpp): the `gameobject` table LEFT JOINed with
// game_event_gameobject, pool_gameobject and pool_gameobject_template.
func init() {
	Register(Loader{
		Name:       "gameobject",
		Order:      OrderGameObjects,
		CountQuery: "SELECT COUNT(*) FROM `gameobject`",
		Load:       loadGameObjects,
	})
}

func loadGameObjects(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	const query = "SELECT `gameobject`.`guid`, `gameobject`.`id`, `gameobject`.`map`, `gameobject`.`position_x`, `gameobject`.`position_y`, `gameobject`.`position_z`, `gameobject`.`orientation`, " +
		"`gameobject`.`rotation0`, `gameobject`.`rotation1`, `gameobject`.`rotation2`, `gameobject`.`rotation3`, `gameobject`.`spawntimesecs`, `gameobject`.`animprogress`, `gameobject`.`state`, `gameobject`.`spawnMask`," +
		"`game_event_gameobject`.`event`, `pool_gameobject`.`pool_entry`, `pool_gameobject_template`.`pool_entry` " +
		"FROM `gameobject` " +
		"LEFT OUTER JOIN `game_event_gameobject` ON `gameobject`.`guid` = `game_event_gameobject`.`guid` " +
		"LEFT OUTER JOIN `pool_gameobject` ON `gameobject`.`guid` = `pool_gameobject`.`guid` " +
		"LEFT OUTER JOIN `pool_gameobject_template` ON `gameobject`.`id` = `pool_gameobject_template`.`id` " +
		"ORDER BY `gameobject`.`guid`"

	return scanTable(ctx, q, t, query, 5000, func(r scanner) error {
		var guid, event, guidPool, entryPool nullI32Raw
		var v entity.GameObjectData
		var state uint32
		if err := r.Scan(
			&guid, u32(&v.ID), u32(&v.MapID),
			f32(&v.PosX), f32(&v.PosY), f32(&v.PosZ), f32(&v.Orientation),
			f32(&v.Rotation0), f32(&v.Rotation1), f32(&v.Rotation2), f32(&v.Rotation3),
			i32(&v.SpawnTimeSecs), u32(&v.AnimProgress), u32(&state), u8(&v.SpawnMask),
			&event, &guidPool, &entryPool,
		); err != nil {
			return err
		}
		if guid.v <= 0 {
			t.Skipf("Table `gameobject` has gameobject with invalid GUID %d", guid.v)
			return nil
		}
		if _, ok := dest.GameObjectTemplates[v.ID]; !ok {
			t.Skipf("Table `gameobject` has gameobject (GUID: %d) with non existing gameobject entry %d", guid.v, v.ID)
			return nil
		}
		// Reference validation: state must be a valid GOState (0..2),
		// rotation components must stay within [-1, 1].
		if state >= entity.MaxGOState {
			t.Skipf("Table `gameobject` have gameobject (GUID: %d Entry: %d) with invalid `state` (%d) value, skip", guid.v, v.ID, state)
			return nil
		}
		v.State = state
		if badRotation(v.Rotation0) || badRotation(v.Rotation1) || badRotation(v.Rotation2) || badRotation(v.Rotation3) {
			t.Skipf("Table `gameobject` have gameobject (GUID: %d Entry: %d) with invalid rotation value, skip", guid.v, v.ID)
			return nil
		}
		if _, dup := dest.GameObjects[uint32(guid.v)]; dup {
			t.Skipf("Table `gameobject` has duplicate GUID %d", guid.v)
			return nil
		}
		dest.GameObjects[uint32(guid.v)] = v
		dest.GameObjectOrder = append(dest.GameObjectOrder, uint32(guid.v))
		if event.v != 0 {
			dest.GameEventGameObjectGUIDs[int16(event.v)] = append(dest.GameEventGameObjectGUIDs[int16(event.v)], uint32(guid.v))
		}
		if guidPool.v > 0 {
			dest.PoolGameObject[uint32(guid.v)] = uint32(guidPool.v)
		}
		if entryPool.v > 0 {
			dest.PoolGameObjectTemplate[v.ID] = uint32(entryPool.v)
		}
		t.Stored(1)
		return nil
	})
}

func badRotation(f float32) bool { return f < -1.0 || f > 1.0 }
