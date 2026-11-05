package entity

// Quest mirrors class Quest (QuestDef.h) — one `quest_template` row. Field
// order and semantics follow the Quest(Field*) constructor in
// QuestDef.cpp: column 0 is entry, the block layout matches the physical
// column order of the mangos-one 2.4.3 schema (127 columns).
type Quest struct {
	QuestID               uint32 // `entry`
	QuestMethod           uint32 // `Method`
	ZoneOrSort            int32  // `ZoneOrSort` (signed: negative = sort)
	MinLevel              uint32 // `MinLevel`
	QuestLevel            uint32 // `QuestLevel`
	Type                  uint32 // `Type`
	RequiredClasses       uint32 // `RequiredClasses`
	RequiredRaces         uint32 // `RequiredRaces`
	RequiredSkill         uint32 // `RequiredSkill`
	RequiredSkillValue    uint32 // `RequiredSkillValue`
	RepObjectiveFaction   uint32 // `RepObjectiveFaction`
	RepObjectiveValue     int32  // `RepObjectiveValue` (signed)
	RequiredMinRepFaction uint32 // `RequiredMinRepFaction`
	RequiredMinRepValue   int32  // `RequiredMinRepValue` (signed)
	RequiredMaxRepFaction uint32 // `RequiredMaxRepFaction`
	RequiredMaxRepValue   int32  // `RequiredMaxRepValue` (signed)
	SuggestedPlayers      uint32 // `SuggestedPlayers`
	LimitTime             uint32 // `LimitTime`
	QuestFlags            uint32 // `Flags` (m_QuestFlags)
	SpecialFlags          uint32 // `SpecialFlags` (m_SpecialFlags)
	CharTitleID           uint32 // `CharTitleId`
	PrevQuestID           int32  // `PrevQuestId` (signed)
	NextQuestID           int32  // `NextQuestId` (signed)
	ExclusiveGroup        int32  // `ExclusiveGroup` (signed)
	NextQuestInChain      uint32 // `NextQuestInChain`
	SrcItemID             uint32 // `SrcItemId`
	SrcItemCount          uint32 // `SrcItemCount`
	SrcSpell              uint32 // `SrcSpell`

	Title            string // `Title`
	Details          string // `Details`
	Objectives       string // `Objectives`
	OfferRewardText  string // `OfferRewardText`
	RequestItemsText string // `RequestItemsText`
	EndText          string // `EndText`

	ObjectiveText [MaxQuestObjectives]string // `ObjectiveText1`..`ObjectiveText4`

	ReqItemID            [MaxQuestObjectives]uint32    // `ReqItemId1`..`ReqItemId4`
	ReqItemCount         [MaxQuestObjectives]uint32    // `ReqItemCount1`..`ReqItemCount4`
	ReqSourceID          [MaxQuestSourceItemIDs]uint32 // `ReqSourceId1`..`ReqSourceId4`
	ReqSourceCount       [MaxQuestSourceItemIDs]uint32 // `ReqSourceCount1`..`ReqSourceCount4`
	ReqCreatureOrGOID    [MaxQuestObjectives]int32     // `ReqCreatureOrGOId1`..`4` (>0 creature, <0 gameobject)
	ReqCreatureOrGOCount [MaxQuestObjectives]uint32
	ReqSpell             [MaxQuestObjectives]uint32 // `ReqSpellCast1`..`ReqSpellCast4`

	RewChoiceItemID    [MaxQuestRewardChoices]uint32 // `RewChoiceItemId1`..`6`
	RewChoiceItemCount [MaxQuestRewardChoices]uint32 // `RewChoiceItemCount1`..`6`
	RewItemID          [MaxQuestRewards]uint32       // `RewItemId1`..`RewItemId4`
	RewItemCount       [MaxQuestRewards]uint32       // `RewItemCount1`..`RewItemCount4`
	RewRepFaction      [MaxQuestReputations]uint32   // `RewRepFaction1`..`5`
	RewRepValue        [MaxQuestReputations]int32    // `RewRepValue1`..`5` (signed)

	RewHonorableKills uint32  // `RewHonorableKills`
	RewOrReqMoney     int32   // `RewOrReqMoney` (signed)
	RewMoneyMaxLevel  uint32  // `RewMoneyMaxLevel`
	RewSpell          uint32  // `RewSpell`
	RewSpellCast      uint32  // `RewSpellCast`
	RewMailTemplateID uint32  // `RewMailTemplateId`
	RewMailDelaySecs  uint32  // `RewMailDelaySecs`
	PointMapID        uint32  // `PointMapId`
	PointX            float32 // `PointX`
	PointY            float32 // `PointY`
	PointOpt          uint32  // `PointOpt`

	DetailsEmote          [MaxQuestEmotes]uint32 // `DetailsEmote1`..`DetailsEmote4`
	DetailsEmoteDelay     [MaxQuestEmotes]uint32 // `DetailsEmoteDelay1`..`4`
	IncompleteEmote       uint32                 // `IncompleteEmote`
	CompleteEmote         uint32                 // `CompleteEmote`
	OfferRewardEmote      [MaxQuestEmotes]uint32 // `OfferRewardEmote1`..`4`
	OfferRewardEmoteDelay [MaxQuestEmotes]uint32 // `OfferRewardEmoteDelay1`..`4`
	QuestStartScript      uint32                 // `StartScript`
	QuestCompleteScript   uint32                 // `CompleteScript`

	// Computed by the loader exactly like the Quest constructor: the
	// number of non-zero objective slots.
	ReqItemsCount         int // m_reqitemscount
	ReqCreatureOrGOCount2 int // m_reqCreatureOrGOcount (number of non-zero creature/GO objectives)
	RewChoiceItemsCount   int // m_rewchoiceitemscount
	RewItemsCount         int // m_rewitemscount
}

// HasQuestFlag mirrors Quest::HasQuestFlag.
func (q *Quest) HasQuestFlag(flag uint32) bool { return q.QuestFlags&flag != 0 }

// HasSpecialFlag mirrors Quest::HasSpecialFlag.
func (q *Quest) HasSpecialFlag(flag uint32) bool { return q.SpecialFlags&flag != 0 }

// IsRepeatable mirrors Quest::IsRepeatable.
func (q *Quest) IsRepeatable() bool { return q.SpecialFlags&0x001 != 0 }

// IsAutoComplete mirrors Quest::IsAutoComplete.
func (q *Quest) IsAutoComplete() bool { return q.QuestMethod == 0 }

// IsDaily mirrors Quest::IsDaily.
func (q *Quest) IsDaily() bool { return q.QuestFlags&0x1000 != 0 }

// QuestActor/QuestRole mirror the quest_relations table discriminators
// (ObjectMgrQuestRelations.cpp): which entity type starts/ends a quest.
const (
	QuestActorCreature    = 0 // QA_CREATURE
	QuestActorGameObject  = 1 // QA_GAMEOBJECT
	QuestActorAreaTrigger = 2 // QA_AREATRIGGER
	QuestRoleStart        = 0 // QR_START
	QuestRoleEnd          = 1 // QR_END
)

// QuestRelation is one `quest_relations` row.
type QuestRelation struct {
	Actor uint8  // `actor` (QuestActor*)
	Entry uint32 // `entry` (creature/GO entry)
	Quest uint32 // `quest`
	Role  uint8  // `role` (QuestRole*)
}
