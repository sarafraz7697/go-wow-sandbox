package worldload

import "wowserver/internal/entity"

// datasetAlias keeps the registry's public name short while the entity
// package stays the single source of truth.
type datasetAlias = entity.Dataset

// Load order constants matching World::SetInitialWorldSettings in the
// reference. New loaders should take the slot that matches their position
// in the reference sequence; the 100-step spacing allows inserting between
// two stages without renumbering everything.
const (
	OrderMangosString           = 100  // LoadMangosStrings
	OrderSpellTemplate          = 200  // LoadSpellTemplate
	OrderInstanceTemplate       = 300  // LoadInstanceTemplate
	OrderPageText               = 400  // LoadPageTexts
	OrderGameObjectTemplate     = 500  // LoadGameobjectInfo
	OrderNpcText                = 600  // LoadGossipText
	OrderItemTemplate           = 700  // LoadItemPrototypes
	OrderCreatureModelInfo      = 800  // LoadCreatureModelInfo
	OrderCreatureEquipTemplate  = 900  // LoadEquipmentTemplates
	OrderCreatureClassLvlStats  = 1000 // LoadCreatureClassLvlStats
	OrderCreatureTemplate       = 1100 // LoadCreatureTemplates
	OrderCreatureTemplateSpells = 1200 // LoadCreatureTemplateSpells
	OrderCreatureModelRace      = 1300 // LoadCreatureModelRace
	OrderItemRequiredTarget     = 1400 // LoadItemRequiredTarget
	OrderReputationRewardRate   = 1500 // LoadReputationRewardRate
	OrderReputationSpillover    = 1600 // LoadReputationSpilloverTemplate
	OrderPointsOfInterest       = 1700 // LoadPointsOfInterest
	OrderPetCreateSpells        = 1800 // LoadPetCreateSpells
	OrderCreatures              = 2000 // LoadCreatures
	OrderCreatureAddons         = 2100 // LoadCreatureAddons
	OrderGameObjects            = 2200 // LoadGameObjects
	OrderQuests                 = 2300 // LoadQuests
	OrderQuestRelations         = 2400 // LoadQuestRelations
	OrderConditions             = 2500 // LoadConditions
	OrderAreaTriggerTeleport    = 2600 // LoadAreaTriggerTeleports
	OrderAreaTriggerQuestEnd    = 2700 // LoadQuestAreaTriggers
	OrderAreaTriggerTavern      = 2800 // LoadTavernAreaTriggers
	OrderGraveyardZones         = 2900 // LoadGraveyardZones
	OrderPlayerInfo             = 3000 // LoadPlayerInfo
	OrderExplorationBaseXP      = 3100 // LoadExplorationBaseXP
	OrderPetNames               = 3200 // LoadPetNames
	OrderPetLevelStats          = 3300 // LoadPetLevelInfo
	OrderMailLevelRewards       = 3400 // LoadMailLevelRewards
	OrderFishingBaseSkill       = 3500 // LoadFishingBaseSkillLevel
	OrderGameTele               = 3600 // LoadGameTele
	OrderGameEvent              = 3650 // GameEventMgr::LoadHolidays (world DB part)
	OrderVendorTemplates        = 3800 // LoadVendorTemplates
	OrderVendors                = 3900 // LoadVendors
	OrderTrainerTemplates       = 4000 // LoadTrainerTemplates
	OrderTrainers               = 4100 // LoadTrainers
	OrderGossipMenus            = 4200 // LoadGossipMenus
	OrderReservedNames          = 4300 // LoadReservedPlayersNames
)
