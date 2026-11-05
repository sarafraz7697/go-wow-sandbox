// Package entity holds the in-memory world entity model. Every struct in
// this package mirrors, field for field, the corresponding MaNGOS-one
// (2.4.3 / build 8606) C++ struct so that data loaded from the world
// database is an exact copy of what the reference server holds in memory.
//
// Reference mapping:
//
//	CreatureInfo          <- struct CreatureInfo        (src/game/Object/Creature.h)
//	CreatureData          <- struct CreatureData        (src/game/Object/Creature.h)
//	CreatureModelInfo     <- struct CreatureModelInfo   (src/game/Object/Creature.h)
//	CreatureDataAddon     <- struct CreatureDataAddon   (src/game/Object/Creature.h)
//	CreatureInfoAddon     <- struct CreatureDataAddon   (src/game/Object/Creature.h, table creature_template_addon)
//	EquipmentInfo         <- struct EquipmentInfo       (src/game/Object/ObjectMgr.h, table creature_equip_template)
//	EquipmentInfoRaw      <- struct EquipmentInfoRaw    (src/game/Object/Creature.h, table creature_equip_template_raw)
//	CreatureClassLvlStats <- struct CreatureClassLvlStats (src/game/Object/Creature.h)
//	CreatureModelRace     <- struct CreatureModelRace   (src/game/Object/Creature.h)
//	GameObjectInfo        <- struct GameObjectInfo      (src/game/Object/GameObject.h)
//	GameObjectData        <- struct GameObjectData      (src/game/Object/GameObject.h)
//	ItemPrototype         <- struct ItemPrototype       (src/game/Object/ItemPrototype.h)
//	Quest                 <- class Quest                (src/game/WorldHandlers/QuestDef.h)
//
// The package contains no SQL and no logic beyond small helpers that the
// reference implements as struct methods; loading lives in
// internal/worldload and lookups live in internal/gamedata.
package entity

// Constants shared with the reference. Sources are noted per constant.

const (
	// MaxCreatureModels is MAX_CREATURE_MODEL from Creature.h: only
	// modelid_1..modelid_4 exist on a creature_template row.
	MaxCreatureModels = 4
	// MaxKillCredit is MAX_KILL_CREDIT from Creature.h.
	MaxKillCredit = 2
	// MaxGOData is the size of the gameobject_template data0..data23 block
	// (GameObjectInfo data union in GameObject.h).
	MaxGOData = 24
	// MaxQuestObjectives is QUEST_OBJECTIVES_COUNT from QuestDef.h.
	MaxQuestObjectives = 4
	// MaxQuestSourceItemIDs is QUEST_SOURCE_ITEM_IDS_COUNT from QuestDef.h.
	MaxQuestSourceItemIDs = 4
	// MaxQuestRewardChoices is QUEST_REWARD_CHOICES_COUNT from QuestDef.h.
	MaxQuestRewardChoices = 6
	// MaxQuestRewards is QUEST_REWARDS_COUNT from QuestDef.h.
	MaxQuestRewards = 4
	// MaxQuestReputations is QUEST_REPUTATIONS_COUNT from QuestDef.h.
	MaxQuestReputations = 5
	// MaxQuestEmotes is QUEST_EMOTE_COUNT from QuestDef.h.
	MaxQuestEmotes = 4
	// MaxItemProtoSpells is MAX_ITEM_PROTO_SPELLS from ItemPrototype.h.
	MaxItemProtoSpells = 5
	// MaxItemProtoStats is MAX_ITEM_PROTO_STATS from ItemPrototype.h.
	MaxItemProtoStats = 10
	// MaxItemProtoDamages is MAX_ITEM_PROTO_DAMAGES from ItemPrototype.h.
	MaxItemProtoDamages = 5
	// MaxItemProtoSockets is MAX_ITEM_PROTO_SOCKETS from ItemPrototype.h.
	MaxItemProtoSockets = 3
	// MaxExpansion is MAX_EXPANSION from shared/Common/SharedDefines.h:
	// 0 = classic, 1 = TBC (creature_template_classlevelstats has one
	// BaseHealthExpN/BaseDamageExpN column pair per expansion).
	MaxExpansion = 1
	// MaxGOState is MAX_GO_STATE from GameObject.h: valid go state values.
	MaxGOState = 3
)
