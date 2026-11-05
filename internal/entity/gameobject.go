package entity

// GameObjectInfo mirrors struct GameObjectInfo (GameObject.h) — one
// `gameobject_template` row. The 24 data columns map to a type-dependent
// union in the reference; the raw values are preserved here so every GO
// type can be interpreted later without touching the loader.
type GameObjectInfo struct {
	ID             uint32            // `entry`
	Type           uint32            // `type`
	DisplayID      uint32            // `displayId`
	Name           string            // `name`
	IconName       string            // `IconName`
	CastBarCaption string            // `castBarCaption`
	Faction        uint32            // `faction`
	Flags          uint32            // `flags`
	Size           float32           // `size`
	Data           [MaxGOData]uint32 // `data0`..`data23`
}

// GameObjectData mirrors struct GameObjectData (GameObject.h) — one spawn
// row of the `gameobject` table, keyed by GUID like
// ObjectMgr::mGameObjectDataMap.
type GameObjectData struct {
	ID            uint32  // `id` (gameobject_template entry)
	MapID         uint32  // `map`
	PosX          float32 // `position_x`
	PosY          float32 // `position_y`
	PosZ          float32 // `position_z`
	Orientation   float32 // `orientation`
	Rotation0     float32 // `rotation0` (quaternion i)
	Rotation1     float32 // `rotation1` (quaternion j)
	Rotation2     float32 // `rotation2` (quaternion k)
	Rotation3     float32 // `rotation3` (quaternion w)
	SpawnTimeSecs int32   // `spawntimesecs` (signed; <0 = despawn after loot)
	AnimProgress  uint32  // `animprogress`
	State         uint32  // `state` (GOState; 0..MAX_GO_STATE-1)
	SpawnMask     uint8   // `spawnMask`
}
