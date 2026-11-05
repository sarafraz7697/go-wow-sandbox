package entity

// ItemStat mirrors struct _ItemStat (ItemPrototype.h): one stat_type%d /
// stat_value%d pair.
type ItemStat struct {
	ItemType uint32 // `stat_typeN`
	Value    int32  // `stat_valueN`
}

// ItemDamage mirrors struct _Damage (ItemPrototype.h): dmg_minN/dmg_maxN
// plus the damage school.
type ItemDamage struct {
	DamageMin  float32 // `dmg_minN`
	DamageMax  float32 // `dmg_maxN`
	DamageType uint32  // `dmg_typeN`
}

// ItemSpell mirrors struct _Spell (ItemPrototype.h): the per-spell block
// spellid_N .. spellcategorycooldown_N.
type ItemSpell struct {
	SpellID               uint32  // `spellid_N`
	SpellTrigger          uint32  // `spelltrigger_N`
	SpellCharges          int32   // `spellcharges_N`
	SpellPPMRate          float32 // `spellppmRate_N`
	SpellCooldown         int32   // `spellcooldown_N`
	SpellCategory         uint32  // `spellcategory_N`
	SpellCategoryCooldown int32   // `spellcategorycooldown_N`
}

// ItemSocket mirrors struct _Socket (ItemPrototype.h).
type ItemSocket struct {
	SocketColor   uint32 // `socketColor_N`
	SocketContent uint32 // `socketContent_N`
}

// ItemPrototype mirrors struct ItemPrototype (ItemPrototype.h) — one
// `item_template` row, complete. Column order follows the SQLStorage
// source format "iiiisiiiii…fiiiiii" from SQLStorages.cpp, which is the
// physical column order of the mangos-one 2.4.3 schema.
type ItemPrototype struct {
	ItemID                    uint32 // `entry`
	Class                     uint32 // `class`
	SubClass                  uint32 // `subclass`
	Unk0                      uint32 // `unk0`
	Name1                     string // `name`
	DisplayInfoID             uint32 // `displayid`
	Quality                   uint32 // `Quality`
	Flags                     uint32 // `Flags`
	BuyCount                  uint32 // `BuyCount`
	BuyPrice                  uint32 // `BuyPrice`
	SellPrice                 uint32 // `SellPrice`
	InventoryType             uint32 // `InventoryType`
	AllowableClass            int32  // `AllowableClass`
	AllowableRace             int32  // `AllowableRace`
	ItemLevel                 uint32 // `ItemLevel`
	RequiredLevel             uint32 // `RequiredLevel`
	RequiredSkill             uint32 // `RequiredSkill`
	RequiredSkillRank         uint32 // `RequiredSkillRank`
	RequiredSpell             uint32 // `requiredspell`
	RequiredHonorRank         uint32 // `requiredhonorrank`
	RequiredCityRank          uint32 // `RequiredCityRank`
	RequiredReputationFaction uint32 // `RequiredReputationFaction`
	RequiredReputationRank    uint32 // `RequiredReputationRank`
	MaxCount                  uint32 // `maxcount`
	Stackable                 int32  // `stackable` (schema int(11); reference reads uint32, -1 means unlimited)
	ContainerSlots            uint32 // `ContainerSlots`

	ItemStat [MaxItemProtoStats]ItemStat     // stat_type1..10 / stat_value1..10
	Damage   [MaxItemProtoDamages]ItemDamage // dmg_min1..5, dmg_max1..5, dmg_type1..5

	Armor          uint32  // `armor`
	HolyRes        uint32  // `holy_res`
	FireRes        uint32  // `fire_res`
	NatureRes      uint32  // `nature_res`
	FrostRes       uint32  // `frost_res`
	ShadowRes      uint32  // `shadow_res`
	ArcaneRes      uint32  // `arcane_res`
	Delay          uint32  // `delay`
	AmmoType       uint32  // `ammo_type`
	RangedModRange float32 // `RangedModRange`

	Spells [MaxItemProtoSpells]ItemSpell // spellid_1..spellcategorycooldown_5

	Bonding     uint32 // `bonding`
	Description string // `description`

	PageText       uint32 // `PageText`
	LanguageID     uint32 // `LanguageID`
	PageMaterial   uint32 // `PageMaterial`
	StartQuest     uint32 // `startquest`
	LockID         uint32 // `lockid`
	Material       int32  // `Material` (signed smallint in schema)
	Sheath         uint32 // `sheath`
	RandomProperty uint32 // `RandomProperty`
	RandomSuffix   uint32 // `RandomSuffix`
	Block          uint32 // `block`
	ItemSet        uint32 // `itemset`
	MaxDurability  uint32 // `MaxDurability`
	Area           uint32 // `area`
	Map            int32  // `Map`
	BagFamily      uint32 // `BagFamily`
	TotemCategory  uint32 // `TotemCategory`

	Socket                  [MaxItemProtoSockets]ItemSocket // socketColor_1..socketContent_3
	SocketBonus             uint32                          // `socketBonus`
	GemProperties           uint32                          // `GemProperties`
	RequiredDisenchantSkill int32                           // `RequiredDisenchantSkill` (signed)
	ArmorDamageModifier     float32                         // `ArmorDamageModifier`
	DisenchantID            uint32                          // (not in the 2.4.3 base schema; stays zero)
	FoodType                uint32                          // (not in the 2.4.3 base schema; stays zero)
	MinMoneyLoot            uint32                          // (not in the 2.4.3 base schema; stays zero)
	MaxMoneyLoot            uint32                          // (not in the 2.4.3 base schema; stays zero)
	Duration                uint32                          // `duration`
	ExtraFlags              uint32                          // (not in the 2.4.3 base schema; stays zero)
}

// GetMaxStackSize mirrors ItemPrototype::GetMaxStackSize.
func (p *ItemPrototype) GetMaxStackSize() uint32 {
	return uint32(p.Stackable)
}
