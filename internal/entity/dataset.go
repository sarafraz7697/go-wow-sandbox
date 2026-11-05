package entity

// RaceClassKey indexes player-side rows by (race, class).
type RaceClassKey struct{ Race, Class uint32 }

// RaceClassLevelKey indexes player_levelstats by (race, class, level).
type RaceClassLevelKey struct {
	Race  uint32
	Class uint32
	Level uint32
}

// ClassLevelKey indexes player_classlevelstats by (class, level).
type ClassLevelKey struct{ Class, Level uint32 }

// CreatureClassLevelKey indexes creature_template_classlevelstats the way
// the reference indexes m_creatureClassLvlStats[level][classIndex][expansion].
type CreatureClassLevelKey struct {
	Class     uint32
	Level     uint32
	Expansion int32
}

// Dataset is the complete set of world-database entities held in memory
// after startup. Member names follow the ObjectMgr storage they mirror
// (see src/game/Object/ObjectMgr.h). The dataset is populated once by the
// worldload runners and is read-only afterwards; loaders append through
// the methods at the bottom of this file so a partially loaded dataset is
// never observable.
type Dataset struct {
	// sCreatureStorage — creature_template keyed by Entry.
	CreatureTemplates map[uint32]CreatureInfo
	// CreatureTemplateOrder is the ascending entry order (queries order by
	// Entry) for deterministic iteration over templates.
	CreatureTemplateOrder []uint32

	// sCreatureModelStorage — creature_model_info keyed by modelid.
	CreatureModelInfos map[uint32]CreatureModelInfo
	// sEquipmentStorage — creature_equip_template keyed by entry.
	EquipmentTemplates map[uint32]EquipmentInfo
	// sEquipmentStorageRaw — creature_equip_template_raw keyed by entry.
	EquipmentTemplatesRaw map[uint32]EquipmentInfoRaw
	// sCreatureInfoAddonStorage — creature_template_addon keyed by entry.
	CreatureTemplateAddons map[uint32]CreatureInfoAddon
	// sCreatureDataAddonStorage — creature_addon keyed by spawn guid.
	CreatureAddons map[uint32]CreatureDataAddon
	// sCreatureTemplateSpellsStorage — creature_template_spells keyed by entry.
	CreatureTemplateSpells map[uint32]CreatureTemplateSpells
	// m_mCreatureModelRaceMap — creature_model_race rows.
	CreatureModelRaces []CreatureModelRace
	// m_creatureClassLvlStats — creature_template_classlevelstats split by
	// expansion like the reference's three-dimensional array.
	CreatureClassLvlStats map[CreatureClassLevelKey]CreatureClassLvlStats

	// mCreatureDataMap — `creature` spawn rows keyed by guid.
	Creatures map[uint32]CreatureData
	// CreatureOrder is the ascending guid order of Creatures for
	// deterministic iteration (mangos reads the table ORDER-less; we keep
	// the query order).
	CreatureOrder []uint32

	// mGameObjectDataMap — `gameobject` spawn rows keyed by guid.
	GameObjects map[uint32]GameObjectData
	// GameObjectOrder is the ascending guid order of GameObjects.
	GameObjectOrder []uint32

	// sGOStorage — gameobject_template keyed by entry.
	GameObjectTemplates map[uint32]GameObjectInfo
	// GameObjectTemplateOrder is the ascending entry order.
	GameObjectTemplateOrder []uint32

	// sItemStorage — item_template keyed by entry.
	ItemPrototypes map[uint32]ItemPrototype
	// ItemPrototypeOrder is the ascending entry order.
	ItemPrototypeOrder []uint32

	// sPageTextStore — page_text keyed by entry.
	PageTexts map[uint32]PageText

	// mQuestTemplates — quest_template keyed by entry.
	Quests map[uint32]*Quest
	// QuestOrder is the ascending entry order.
	QuestOrder []uint32

	// m_CreatureQuestRelations / m_CreatureQuestInvolvedRelations /
	// m_GOQuestRelations / m_GOQuestInvolvedRelations — from the unified
	// `quest_relations` table, split by (actor, role).
	CreatureQuestStarters   map[uint32][]uint32 // creature entry -> quest ids
	CreatureQuestEnders     map[uint32][]uint32
	GameObjectQuestStarters map[uint32][]uint32
	GameObjectQuestEnders   map[uint32][]uint32

	// mAreaTriggers — areatrigger_teleport keyed by trigger id.
	AreaTriggerTeleports map[uint32]AreaTriggerTeleport
	// mTavernAreaTrigger — areatrigger_tavern ids.
	TavernTriggers map[uint32]AreaTriggerTavern
	// mGraveYardMap — game_graveyard_zone links.
	GraveyardLinks []GraveyardZoneLink
	// mExplorationBaseXPTable — exploration_basexp keyed by level.
	ExplorationBaseXP map[uint32]uint32
	// PetNames — pet_name_generation rows.
	PetNames []PetNameGeneration
	// mMangosStrings — mangos_string keyed by entry.
	MangosStrings map[uint32]MangosString
	// mGameEvent — game_event keyed by entry.
	GameEvents map[uint32]GameEvent

	// GameEvent system side indexes filled by the spawn loaders (the LEFT
	// OUTER JOIN columns in LoadCreatures/LoadGameObjects):
	// guid/event pairs from game_event_creature / game_event_gameobject.
	GameEventCreatureGUIDs   map[int16][]uint32 // event -> creature guids
	GameEventGameObjectGUIDs map[int16][]uint32 // event -> gameobject guids
	// Pool side indexes from pool_creature(_template) /
	// pool_gameobject(_template).
	PoolCreature           map[uint32]uint32 // creature guid -> pool entry
	PoolCreatureTemplate   map[uint32]uint32 // creature entry -> pool entry
	PoolGameObject         map[uint32]uint32 // gameobject guid -> pool entry
	PoolGameObjectTemplate map[uint32]uint32 // gameobject entry -> pool entry

	// m_mCacheVendorItemMap / m_mCacheVendorTemplateItemMap.
	NpcVendors         map[uint32][]VendorItem // creature entry -> items
	NpcVendorTemplates map[uint32][]VendorItem
	// m_mCacheTrainerSpellMap / m_mCacheTrainerTemplateSpellMap.
	NpcTrainers         map[uint32][]TrainerSpell // creature entry -> spells
	NpcTrainerTemplates map[uint32][]TrainerSpell
	// m_mGossipMenusMap — gossip_menu keyed by entry (multimap semantics).
	GossipMenus map[uint32][]GossipMenu
	// m_mGossipMenuItemsMap — gossip_menu_option keyed by menu id.
	GossipMenuOptions map[uint32][]GossipMenuOption
	// npc_text (GossipText) keyed by id.
	NpcTexts map[uint32]NpcText

	// sSpellTemplate — spell_template keyed by id.
	SpellTemplates map[uint32]SpellTemplate
	// sInstanceTemplate — instance_template keyed by map.
	InstanceTemplates map[uint32]InstanceTemplate

	// Player creation data (ObjectMgr::mPlayerInfo and friends).
	PlayerCreateInfos       map[RaceClassKey]PlayerCreateInfo
	PlayerCreateInfoItems   []PlayerCreateInfoItem
	PlayerCreateInfoSpells  []PlayerCreateInfoSpell
	PlayerCreateInfoActions []PlayerCreateInfoAction
	PlayerLevelStats        map[RaceClassLevelKey]PlayerLevelStats
	PlayerClassLevelStats   map[ClassLevelKey]PlayerClassLevelStats
	PlayerXPForLevel        map[uint32]uint32
	ReservedNames           map[string]struct{}

	// sConditionStorage — conditions keyed by condition_entry.
	Conditions map[uint32]PlayerCondition
	// mGameTeleMap — game_tele keyed by id.
	GameTeles map[uint32]GameTele
	// mPointsOfInterest — points_of_interest keyed by entry.
	PointsOfInterest map[uint32]PointOfInterest
	// mItemRequiredTarget — item_required_target rows.
	ItemRequiredTargets []ItemRequiredTarget
	// mRepSpilloverTemplate — reputation_spillover_template keyed by faction.
	ReputationSpilloverTemplates map[uint32]ReputationSpilloverTemplate
	// mRepRewardRate — reputation_reward_rate keyed by faction.
	ReputationRewardRates map[uint32]ReputationRewardRate
	// mMailLevelRewards — mail_level_reward rows.
	MailLevelRewards []MailLevelReward
	// mFishingBaseSkillLevel — skill_fishing_base_level keyed by area.
	FishingBaseSkillLevels map[uint32]uint32
	// mPetCreateSpells — petcreateinfo_spell keyed by creature entry.
	PetCreateSpells map[uint32]PetCreateInfoSpell
	// mPetLevelStats — pet_levelstats keyed by (creature, level).
	PetLevelStats map[PetLevelStatsKey]PetLevelStats
}

// PetLevelStatsKey indexes pet_levelstats rows.
type PetLevelStatsKey struct{ Entry, Level uint32 }

// NewDataset allocates every collection. Loaders assume the maps are
// non-nil; appending to a nil map panics.
func NewDataset() *Dataset {
	d := &Dataset{}
	d.CreatureTemplates = make(map[uint32]CreatureInfo)
	d.CreatureModelInfos = make(map[uint32]CreatureModelInfo)
	d.EquipmentTemplates = make(map[uint32]EquipmentInfo)
	d.EquipmentTemplatesRaw = make(map[uint32]EquipmentInfoRaw)
	d.CreatureTemplateAddons = make(map[uint32]CreatureInfoAddon)
	d.CreatureAddons = make(map[uint32]CreatureDataAddon)
	d.CreatureTemplateSpells = make(map[uint32]CreatureTemplateSpells)
	d.CreatureClassLvlStats = make(map[CreatureClassLevelKey]CreatureClassLvlStats)
	d.Creatures = make(map[uint32]CreatureData)
	d.GameObjects = make(map[uint32]GameObjectData)
	d.GameObjectTemplates = make(map[uint32]GameObjectInfo)
	d.ItemPrototypes = make(map[uint32]ItemPrototype)
	d.PageTexts = make(map[uint32]PageText)
	d.Quests = make(map[uint32]*Quest)
	d.CreatureQuestStarters = make(map[uint32][]uint32)
	d.CreatureQuestEnders = make(map[uint32][]uint32)
	d.GameObjectQuestStarters = make(map[uint32][]uint32)
	d.GameObjectQuestEnders = make(map[uint32][]uint32)
	d.AreaTriggerTeleports = make(map[uint32]AreaTriggerTeleport)
	d.TavernTriggers = make(map[uint32]AreaTriggerTavern)
	d.ExplorationBaseXP = make(map[uint32]uint32)
	d.MangosStrings = make(map[uint32]MangosString)
	d.GameEvents = make(map[uint32]GameEvent)
	d.GameEventCreatureGUIDs = make(map[int16][]uint32)
	d.GameEventGameObjectGUIDs = make(map[int16][]uint32)
	d.PoolCreature = make(map[uint32]uint32)
	d.PoolCreatureTemplate = make(map[uint32]uint32)
	d.PoolGameObject = make(map[uint32]uint32)
	d.PoolGameObjectTemplate = make(map[uint32]uint32)
	d.NpcVendors = make(map[uint32][]VendorItem)
	d.NpcVendorTemplates = make(map[uint32][]VendorItem)
	d.NpcTrainers = make(map[uint32][]TrainerSpell)
	d.NpcTrainerTemplates = make(map[uint32][]TrainerSpell)
	d.GossipMenus = make(map[uint32][]GossipMenu)
	d.GossipMenuOptions = make(map[uint32][]GossipMenuOption)
	d.NpcTexts = make(map[uint32]NpcText)
	d.SpellTemplates = make(map[uint32]SpellTemplate)
	d.InstanceTemplates = make(map[uint32]InstanceTemplate)
	d.PlayerCreateInfos = make(map[RaceClassKey]PlayerCreateInfo)
	d.PlayerLevelStats = make(map[RaceClassLevelKey]PlayerLevelStats)
	d.PlayerClassLevelStats = make(map[ClassLevelKey]PlayerClassLevelStats)
	d.PlayerXPForLevel = make(map[uint32]uint32)
	d.ReservedNames = make(map[string]struct{})
	d.Conditions = make(map[uint32]PlayerCondition)
	d.GameTeles = make(map[uint32]GameTele)
	d.PointsOfInterest = make(map[uint32]PointOfInterest)
	d.ReputationSpilloverTemplates = make(map[uint32]ReputationSpilloverTemplate)
	d.ReputationRewardRates = make(map[uint32]ReputationRewardRate)
	d.FishingBaseSkillLevels = make(map[uint32]uint32)
	d.PetCreateSpells = make(map[uint32]PetCreateInfoSpell)
	d.PetLevelStats = make(map[PetLevelStatsKey]PetLevelStats)
	return d
}
