package entity

// VendorItem mirrors struct VendorItem (ObjectMgr.h / VendorItemData) — one
// `npc_vendor` (or `npc_vendor_template`) row.
type VendorItem struct {
	Item         uint32 // `item`
	MaxCount     uint32 // `maxcount` (0 = unlimited)
	IncrTime     uint32 // `incrtime` (restock seconds)
	ExtendedCost uint32 // `ExtendedCost`
	ConditionID  uint32 // `condition_id`
}

// TrainerSpell mirrors struct TrainerSpell (ObjectMgr.h) — one `npc_trainer`
// (or `npc_trainer_template`) row. RequiredLevel is stored as signed in the
// reference; the meaning of the fields is documented in TrainerTemplate.
type TrainerSpell struct {
	Spell              uint32 // `spell`
	SpellCost          uint32 // `spellcost`
	RequiredSkill      uint32 // `reqskill`
	RequiredSkillValue uint32 // `reqskillvalue`
	ReqLevel           int32  // `reqlevel` (signed in reference)
}

// GossipMenu mirrors struct GossipMenus (GossipDef.h) — one `gossip_menu`
// row.
type GossipMenu struct {
	Entry       uint32 // `entry`
	TextID      uint32 // `text_id`
	ScriptID    uint32 // `script_id`
	ConditionID uint32 // `condition_id`
}

// GossipMenuOption mirrors struct GossipMenuOption (GossipDef.h) — one
// `gossip_menu_option` row.
type GossipMenuOption struct {
	MenuID           uint32 // `menu_id`
	ID               uint32 // `id`
	OptionIcon       uint32 // `option_icon`
	OptionText       string // `option_text`
	OptionID         uint32 // `option_id`
	NpcOptionNpcflag uint32 // `npc_option_npcflag`
	ActionMenuID     uint32 // `action_menu_id`
	ActionPoiID      uint32 // `action_poi_id`
	ActionScriptID   uint32 // `action_script_id`
	BoxCoded         bool   // `box_coded`
	BoxMoney         uint32 // `box_money`
	BoxText          string // `box_text`
	ConditionID      uint32 // `condition_id`
}

// NpcTextBlock is one of the 8 text groups of an `npc_text` row: two text
// variants, language, probability and three emote/delay pairs.
type NpcTextBlock struct {
	Text0      string    // `textN_0`
	Text1      string    // `textN_1`
	Lang       uint32    // `langN`
	Prob       float32   // `probN`
	EmoteDelay [3]uint32 // `emN_0_delay`..`emN_2_delay`
	Emote      [3]uint32 // `emN_0`..`emN_2`
}

// NpcText is one `npc_text` row (GossipText in the reference) with its 8
// broadcast text blocks.
type NpcText struct {
	ID     uint32 // `ID`
	Blocks [8]NpcTextBlock
}

// PageText is one `page_text` row.
type PageText struct {
	Entry    uint32 // `entry`
	Text     string // `text`
	NextPage uint32 // `next_page`
}

// MangosString is one `mangos_string` row: entry plus the default text and
// the 8 localized variants (empty when the column is NULL).
type MangosString struct {
	Entry          uint32
	ContentDefault string
	ContentLoc     [8]string
}

// AreaTriggerTeleport mirrors struct AreaTrigger (ObjectMgr.h) — one
// `areatrigger_teleport` row. Name comes from the schema (the reference
// does not keep it in memory, but the column exists).
type AreaTriggerTeleport struct {
	ID                      uint32  // `id`
	Name                    string  // `name`
	RequiredLevel           uint32  // `required_level`
	RequiredItem            uint32  // `required_item`
	RequiredItem2           uint32  // `required_item2`
	HeroicKey               uint32  // `heroic_key`
	HeroicKey2              uint32  // `heroic_key2`
	RequiredQuestDone       uint32  // `required_quest_done`
	RequiredQuestDoneHeroic uint32  // `required_quest_done_heroic`
	TargetMapID             uint32  // `target_map`
	TargetX                 float32 // `target_position_x`
	TargetY                 float32 // `target_position_y`
	TargetZ                 float32 // `target_position_z`
	TargetOrientation       float32 // `target_orientation`
}

// GraveyardZoneLink is one `game_graveyard_zone` row (GraveyardEntry in the
// reference). The safe-loc position itself comes from WorldSafeLocs.dbc.
type GraveyardZoneLink struct {
	ID        uint32 // `id` (WorldSafeLocs.dbc id)
	GhostZone uint32 // `ghost_zone`
	Faction   uint32 // `faction` (0 both, 1 horde... TEAM mask)
}

// ExplorationBaseXP is one `exploration_basexp` row.
type ExplorationBaseXP struct {
	Level  uint32 // `level`
	BaseXP uint32 // `basexp`
}

// PetNameGeneration is one `pet_name_generation` row: a word half for
// generated pet names.
type PetNameGeneration struct {
	ID    uint32 // `id`
	Word  string // `word`
	Entry uint32 // `entry` (creature entry, 0 = all)
	Half  uint32 // `half` (0 front, 1 back)
}

// GameEvent is one `game_event` row.
type GameEvent struct {
	Entry       uint32 // `entry`
	StartTime   string // `start_time` (kept as DB string like the reference)
	EndTime     string // `end_time`
	Occurence   uint32 // `occurence`
	Length      uint32 // `length`
	Holiday     uint32 // `holiday`
	Description string // `description`
}

// InstanceTemplate is one `instance_template` row.
type InstanceTemplate struct {
	Map        uint32 // `map`
	Parent     uint32 // `parent`
	LevelMin   uint32 // `levelMin`
	LevelMax   uint32 // `levelMax`
	MaxPlayers uint32 // `maxPlayers`
	ResetDelay uint32 // `reset_delay`
}

// SpellTemplate is one `spell_template` row (the subset the reference
// loads from the DB to supplement Spell.dbc for spell_proc_event handling).
type SpellTemplate struct {
	ID                     uint32 // `id`
	ProcFlags              uint32 // `proc_flags`
	ProcChance             uint32 // `proc_chance`
	DurationIndex          uint32 // `duration_index`
	Effect0                uint32 // `effect0`
	Effect0ImplicitTargetA uint32 // `effect0_implicit_target_a`
	Effect0RadiusIdx       uint32 // `effect0_radius_idx`
	Effect0ApplyAuraName   uint32 // `effect0_apply_aura_name`
	Effect0MiscValue       uint32 // `effect0_misc_value`
	Effect0MiscValueB      uint32 // `effect0_misc_value_b`
	Effect0TriggerSpell    uint32 // `effect0_trigger_spell`
}

// PlayerCreateInfo is one `playercreateinfo` row (PlayerInfo in the
// reference).
type PlayerCreateInfo struct {
	Race        uint32
	Class       uint32
	Map         uint32
	Zone        uint32
	PositionX   float32
	PositionY   float32
	PositionZ   float32
	Orientation float32
}

// PlayerCreateInfoItem is one `playercreateinfo_item` row.
type PlayerCreateInfoItem struct {
	Race   uint32
	Class  uint32
	ItemID uint32 // `itemid`
	Amount uint32
}

// PlayerCreateInfoSpell is one `playercreateinfo_spell` row.
type PlayerCreateInfoSpell struct {
	Race  uint32
	Class uint32
	Spell uint32
}

// PlayerCreateInfoAction is one `playercreateinfo_action` row.
type PlayerCreateInfoAction struct {
	Race   uint32
	Class  uint32
	Button uint32
	Action uint32
	Type   uint32
}

// PlayerLevelStats is one `player_levelstats` row.
type PlayerLevelStats struct {
	Race  uint32
	Class uint32
	Level uint32
	Str   uint32
	Agi   uint32
	Sta   uint32
	Inte  uint32
	Spi   uint32
}

// PlayerClassLevelStats is one `player_classlevelstats` row.
type PlayerClassLevelStats struct {
	Class    uint32
	Level    uint32
	BaseHP   uint32 // `basehp`
	BaseMana uint32 // `basemana`
}

// PlayerXPForLevel is one `player_xp_for_level` row.
type PlayerXPForLevel struct {
	Level          uint32 // `lvl`
	XPForNextLevel uint32 // `xp_for_next_level`
}

// AreaTriggerTavern is one `areatrigger_tavern` row (rest zone trigger id).
type AreaTriggerTavern struct {
	ID   uint32 // `id`
	Name string // `name`
}

// PlayerCondition is one `conditions` row (sConditionStorage).
type PlayerCondition struct {
	ConditionEntry uint32 // `condition_entry`
	Type           uint32 // `type`
	Value1         uint32 // `value1`
	Value2         uint32 // `value2`
}

// GameTele is one `game_tele` row (mGameTeleMap).
type GameTele struct {
	ID          uint32  // `id`
	PositionX   float32 // `position_x`
	PositionY   float32 // `position_y`
	PositionZ   float32 // `position_z`
	Orientation float32 // `orientation`
	Map         uint32  // `map`
	Name        string  // `name`
}

// PointOfInterest is one `points_of_interest` row (mPointsOfInterest).
type PointOfInterest struct {
	Entry    uint32  // `entry`
	X        float32 // `x`
	Y        float32 // `y`
	Icon     uint32  // `icon`
	Flags    uint32  // `flags`
	Data     uint32  // `data`
	IconName string  // `icon_name`
}

// ItemRequiredTarget is one `item_required_target` row.
type ItemRequiredTarget struct {
	Entry       uint32 // `entry` (item id)
	Type        uint32 // `type`
	TargetEntry uint32 // `targetEntry`
}

// ReputationSpilloverTemplate is one `reputation_spillover_template` row.
type ReputationSpilloverTemplate struct {
	Faction uint32 // `faction`
	// Four (faction, rate, rank) triples.
	SpilloverFaction [4]uint32  // `faction1`..`faction4`
	Rate             [4]float32 // `rate_1`..`rate_4`
	Rank             [4]uint32  // `rank_1`..`rank_4`
}

// ReputationRewardRate is one `reputation_reward_rate` row.
type ReputationRewardRate struct {
	Faction      uint32  // `faction`
	QuestRate    float32 // `quest_rate`
	CreatureRate float32 // `creature_rate`
	SpellRate    float32 // `spell_rate`
}

// MailLevelReward is one `mail_level_reward` row.
type MailLevelReward struct {
	Level          uint32 // `level`
	RaceMask       uint32 // `raceMask`
	MailTemplateID uint32 // `mailTemplateId`
	SenderEntry    uint32 // `senderEntry`
}

// SkillFishingBaseLevel is one `skill_fishing_base_level` row.
type SkillFishingBaseLevel struct {
	Entry uint32 // `entry` (area id)
	Skill uint32 // `skill`
}

// PetCreateInfoSpell is one `petcreateinfo_spell` row (default pet spells).
type PetCreateInfoSpell struct {
	Entry  uint32    // `entry` (creature entry)
	Spells [4]uint32 // `Spell1`..`Spell4`
}

// PetLevelStats is one `pet_levelstats` row.
type PetLevelStats struct {
	CreatureEntry uint32 // `creature_entry`
	Level         uint32 // `level`
	HP            uint32 // `hp`
	Mana          uint32 // `mana`
	Armor         uint32 // `armor`
	Str           uint32 // `str`
	Agi           uint32 // `agi`
	Sta           uint32 // `sta`
	Inte          uint32 // `inte`
	Spi           uint32 // `spi`
}
