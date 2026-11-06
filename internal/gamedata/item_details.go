package gamedata

// Client-side DBC types used by the catalog. The world-DB entity structs
// live in internal/entity.

// ClientItem is one Item.dbc row (display correction source).
type ClientItem struct {
	ID            uint32
	Display       uint32
	InventoryType uint32
}

// Enchantment is one SpellItemEnchantment.dbc row: three
// (effect, amount, argument) triplets plus the visual and condition.
type Enchantment struct {
	ID                          uint32
	Visual                      uint32
	Condition                   uint32
	Effects, Amounts, Arguments [3]uint32
}
type Faction struct {
	ID    uint32
	Index int32
	// RaceMask/ClassMask/Base are ReputationRaceMask[4], ReputationClassMask[4]
	// and ReputationBase[4] of FactionEntry (FactionEntry::GetIndexFitTo).
	RaceMask, ClassMask [4]uint32
	Base                [4]int32
}
type SpellCooldown struct{ ID, Category, Recovery, CategoryRecovery uint32 }

func (c *Catalog) SpellCooldown(id uint32) (SpellCooldown, bool) {
	v, ok := c.cooldowns[id]
	return v, ok
}
func (c *Catalog) Enchantment(id uint32) (Enchantment, bool) {
	v, ok := c.enchantments[id]
	return v, ok
}
func (c *Catalog) FactionIndex(id uint32) (int32, bool) { v, ok := c.factionIndices[id]; return v, ok }
