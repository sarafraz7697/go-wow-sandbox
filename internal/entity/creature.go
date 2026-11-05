package entity

// CreatureInfo mirrors struct CreatureInfo (Creature.h) — one row of the
// `creature_template` table. Column order matches the MaNGOS SQLStorage
// source format "isssiiiiiiiiifiiiiliiiiiffiiiffffffiiiiffff…" from
// src/game/Server/SQLStorages.cpp, i.e. the physical column order of the
// mangos-one 2.4.3 world schema. Field names are the MaNGOS names.
type CreatureInfo struct {
	Entry    uint32 // `Entry`
	Name     string // `Name`
	SubName  string // `SubName` (nullable, COALESCE '')
	IconName string // `IconName` (nullable, COALESCE '')

	MinLevel    uint32 // `MinLevel`
	MaxLevel    uint32 // `MaxLevel`
	HeroicEntry uint32 // `HeroicEntry`

	ModelId [MaxCreatureModels]uint32 // `ModelId1`..`ModelId4`

	FactionAlliance uint32  // `FactionAlliance` (faction_A)
	FactionHorde    uint32  // `FactionHorde` (faction_H)
	Scale           float32 // `Scale`

	Family          uint32 // `Family`
	CreatureType    uint32 // `CreatureType` (type)
	InhabitType     uint32 // `InhabitType`
	RegenerateStats uint32 // `RegenerateStats`
	RacialLeader    bool   // `RacialLeader` (tinyint -> bool)

	NpcFlags          uint32 // `NpcFlags`
	UnitFlags         uint32 // `UnitFlags`
	DynamicFlags      uint32 // `DynamicFlags`
	ExtraFlags        uint32 // `ExtraFlags`
	CreatureTypeFlags uint32 // `CreatureTypeFlags`

	SpeedWalk float32 // `SpeedWalk`
	SpeedRun  float32 // `SpeedRun`

	UnitClass uint32 // `UnitClass`
	Rank      uint32 // `Rank`
	Expansion int32  // `Expansion` (signed; -1 marks special cases)

	HealthMultiplier     float32 // `HealthMultiplier`
	PowerMultiplier      float32 // `PowerMultiplier`
	DamageMultiplier     float32 // `DamageMultiplier`
	DamageVariance       float32 // `DamageVariance`
	ArmorMultiplier      float32 // `ArmorMultiplier`
	ExperienceMultiplier float32 // `ExperienceMultiplier`

	MinLevelHealth uint32 // `MinLevelHealth`
	MaxLevelHealth uint32 // `MaxLevelHealth`
	MinLevelMana   uint32 // `MinLevelMana`
	MaxLevelMana   uint32 // `MaxLevelMana`

	MinMeleeDmg  float32 // `MinMeleeDmg`
	MaxMeleeDmg  float32 // `MaxMeleeDmg`
	MinRangedDmg float32 // `MinRangedDmg`
	MaxRangedDmg float32 // `MaxRangedDmg`

	Armor                uint32 // `Armor`
	MeleeAttackPower     uint32 // `MeleeAttackPower`
	RangedAttackPower    uint32 // `RangedAttackPower`
	MeleeBaseAttackTime  uint32 // `MeleeBaseAttackTime`
	RangedBaseAttackTime uint32 // `RangedBaseAttackTime`
	DamageSchool         uint32 // `DamageSchool`

	MinLootGold      uint32 // `MinLootGold`
	MaxLootGold      uint32 // `MaxLootGold`
	LootId           uint32 // `LootId`
	PickpocketLootId uint32 // `PickpocketLootId`
	SkinningLootId   uint32 // `SkinningLootId`

	KillCredit [MaxKillCredit]uint32 // `KillCredit1`, `KillCredit2`

	MechanicImmuneMask uint32 // `MechanicImmuneMask`
	SchoolImmuneMask   uint32 // `SchoolImmuneMask`

	ResistanceHoly   int32 // `ResistanceHoly` (signed smallint)
	ResistanceFire   int32 // `ResistanceFire`
	ResistanceNature int32 // `ResistanceNature`
	ResistanceFrost  int32 // `ResistanceFrost`
	ResistanceShadow int32 // `ResistanceShadow`
	ResistanceArcane int32 // `ResistanceArcane`

	PetSpellDataId uint32 // `PetSpellDataId`
	MovementType   uint32 // `MovementType`

	TrainerType         uint32 // `TrainerType`
	TrainerSpell        uint32 // `TrainerSpell`
	TrainerClass        uint32 // `TrainerClass`
	TrainerRace         uint32 // `TrainerRace`
	TrainerTemplateId   uint32 // `TrainerTemplateId`
	VendorTemplateId    uint32 // `VendorTemplateId`
	EquipmentTemplateId uint32 // `EquipmentTemplateId`
	GossipMenuId        uint32 // `GossipMenuId`

	AIName string // `AIName`
}

// CreatureData mirrors struct CreatureData (Creature.h) — one spawn row of
// the `creature` table. The spawn GUID is the map key of Dataset.Creatures,
// exactly like ObjectMgr::mCreatureDataMap.
type CreatureData struct {
	ID              uint32  // `id` (creature_template entry)
	MapID           uint32  // `map`
	ModelIDOverride uint32  // `modelid` (overrides the template model)
	EquipmentID     int32   // `equipment_id` (0 default, -1 no equipment)
	PosX            float32 // `position_x`
	PosY            float32 // `position_y`
	PosZ            float32 // `position_z`
	Orientation     float32 // `orientation`
	SpawnTimeSecs   uint32  // `spawntimesecs`
	SpawnDist       float32 // `spawndist`
	CurrentWaypoint uint32  // `currentwaypoint`
	CurHealth       uint32  // `curhealth`
	CurMana         uint32  // `curmana`
	IsDead          bool    // `DeathState`
	MovementType    uint8   // `MovementType`
	SpawnMask       uint8   // `spawnMask`
}

// CreatureModelInfo mirrors struct CreatureModelInfo (Creature.h) — one row
// of `creature_model_info`, keyed by modelid.
type CreatureModelInfo struct {
	ModelID            uint32  // `modelid`
	BoundingRadius     float32 // `bounding_radius`
	CombatReach        float32 // `combat_reach`
	Gender             uint8   // `gender`
	ModelIDOtherGender uint32  // `modelid_other_gender`
	ModelIDAlternative uint32  // `modelid_alternative`
}

// CreatureDataAddon mirrors struct CreatureDataAddon (Creature.h) for the
// `creature_addon` table (keyed by spawn GUID). Auras is the parsed
// "spell1 spell2 ..." list from the `auras` column (empty slice when the
// column is empty; MaNGOS stores a NULL-terminated uint32 array).
type CreatureDataAddon struct {
	GUIDOrEntry uint32   // `guid`
	Mount       uint32   // `mount`
	Bytes1      uint32   // `bytes1`
	SheathState uint8    // `b2_0_sheath`
	Flags       uint8    // `b2_1_flags`
	Emote       uint32   // `emote`
	MoveFlags   uint32   // `moveflags`
	Auras       []uint32 // `auras` parsed
}

// CreatureInfoAddon is the `creature_template_addon` flavor of
// CreatureDataAddon (keyed by creature entry).
type CreatureInfoAddon CreatureDataAddon

// EquipmentInfo is one `creature_equip_template` row: the entry plus the
// three equipped item entries (SQLStorage format "iiii").
type EquipmentInfo struct {
	Entry      uint32    // `entry`
	EquipEntry [3]uint32 // `equipentry1`..`equipentry3`
}

// EquipmentInfoRaw is the deprecated `creature_equip_template_raw` layout
// (SQLStorage format "iiiiiiiiii").
type EquipmentInfoRaw struct {
	Entry      uint32    // `entry`
	EquipModel [3]uint32 // `equipmodel1`..`equipmodel3`
	EquipInfo  [3]uint32 // `equipinfo1`..`equipinfo3`
	EquipSlot  [3]uint32 // `equipslot1`..`equipslot3`
}

// CreatureClassLvlStats mirrors struct CreatureClassLvlStats (Creature.h) —
// one `creature_template_classlevelstats` row for a (class, level,
// expansion) tuple. The table stores one BaseHealthExpN/BaseDamageExpN pair
// per expansion on the same row; the dataset stores them split per
// expansion like m_creatureClassLvlStats in the reference.
type CreatureClassLvlStats struct {
	Level                 uint32  // `Level`
	Class                 uint32  // `Class`
	Expansion             int32   // derived: which BaseHealthExpN column
	BaseHealth            uint32  // `BaseHealthExp0`/`BaseHealthExp1`
	BaseMana              uint32  // `BaseMana`
	BaseDamage            float32 // `BaseDamageExp0`/`BaseDamageExp1`
	BaseMeleeAttackPower  float32 // `BaseMeleeAttackPower`
	BaseRangedAttackPower float32 // `BaseRangedAttackPower`
	BaseArmor             uint32  // `BaseArmor`
}

// CreatureModelRace is one `creature_model_race` row (used to pick model by
// race for shapeshift forms).
type CreatureModelRace struct {
	ModelID       uint32 // `modelid`
	RaceMask      uint32 // `racemask`
	CreatureEntry uint32 // `creature_entry`
	ModelIDRacial uint32 // `modelid_racial`
}

// CreatureTemplateSpells is one `creature_template_spells` row.
type CreatureTemplateSpells struct {
	Entry  uint32    // `entry`
	Spells [4]uint32 // `spell1`..`spell4`
}
