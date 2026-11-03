package gamedata

// ItemDetails contains template data used for the client's item cache. All
// numeric values come from item_template; it is not an encoded network packet.
type ItemDetails struct {
	ID                                                         uint32
	Name, Description                                          string
	Quality, Flags, BuyPrice, SellPrice                        uint32
	AllowableClass, AllowableRace                              int32
	ItemLevel, RequiredLevel                                   uint32
	RequiredSkill, RequiredSkillRank, RequiredSpell            uint32
	RequiredHonorRank, RequiredCityRank                        uint32
	RequiredReputationFaction, RequiredReputationRank          uint32
	MaxCount, ContainerSlots                                   uint32
	Stats                                                      [10]ItemStat
	Damage                                                     [5]ItemDamage
	Resistances                                                [7]uint32
	Delay, AmmoType                                            uint32
	RangedModRange                                             float32
	Spells                                                     [5]ItemSpell
	PageText, Language, PageMaterial, StartQuest, Lock         uint32
	Material                                                   int32
	Sheath, RandomProperty, RandomSuffix, Block, ItemSet, Area uint32
	Map                                                        int32
	BagFamily, TotemCategory                                   uint32
	Sockets                                                    [3]ItemSocket
	SocketBonus, GemProperties                                 uint32
	DisenchantSkill                                            int32
	ArmorDamageModifier                                        float32
}
type ItemStat struct {
	Type  uint32
	Value int32
}
type ItemDamage struct {
	Min, Max float32
	Type     uint32
}
type ItemSpell struct {
	ID, Trigger       uint32
	Charges, Cooldown int32
	Category          uint32
	CategoryCooldown  int32
}
type ItemSocket struct{ Color, Content uint32 }
type Faction struct {
	ID    uint32
	Index int32
}
type SpellCooldown struct{ ID, Category, Recovery, CategoryRecovery uint32 }

func (c *Catalog) SpellCooldown(id uint32) (SpellCooldown, bool) {
	v, ok := c.cooldowns[id]
	return v, ok
}
func (c *Catalog) ItemDetails(id uint32) (ItemDetails, bool) {
	v, ok := c.itemDetails[id]
	return v, ok
}
func (c *Catalog) FactionIndex(id uint32) (int32, bool) { v, ok := c.factionIndices[id]; return v, ok }
