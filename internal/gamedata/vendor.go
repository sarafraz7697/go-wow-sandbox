package gamedata

import "wowserver/internal/entity"

// FactionTemplate is one FactionTemplate.dbc row (FactionTemplateEntry).
type FactionTemplate struct {
	ID, Faction, Flags                 uint32
	OurMask, FriendlyMask, HostileMask uint32
	EnemyFaction, FriendFaction        [4]uint32
}

// IsHostileTo ports FactionTemplateEntry::IsHostileTo.
func (t FactionTemplate) IsHostileTo(o FactionTemplate) bool {
	if o.Faction != 0 {
		for _, f := range t.EnemyFaction {
			if f == o.Faction {
				return true
			}
		}
		for _, f := range t.FriendFaction {
			if f == o.Faction {
				return false
			}
		}
	}
	return t.HostileMask&o.OurMask != 0
}

// Reputation ranks (ReputationRank, SharedDefines.h).
const (
	RepHated    = 0
	RepHostile  = 1
	RepNeutral  = 3
	RepFriendly = 4
	RepExalted  = 7
)

// ReputationToRank ports ReputationMgr::ReputationToRank.
func ReputationToRank(standing int32) int {
	pointsInRank := [8]int32{36000, 3000, 3000, 3000, 6000, 12000, 21000, 1000}
	limit := int32(42999) + 1 // Reputation_Cap + 1
	for i := 7; i >= 0; i-- {
		limit -= pointsInRank[i]
		if standing >= limit {
			return i
		}
	}
	return RepHated
}

// PlayerReputation is the view of a player's reputation state the rules
// below need: race/class masks (Player::getRaceMask/getClassMask) and the
// stored FactionState standing per faction id.
type PlayerReputation struct {
	RaceMask, ClassMask uint32
	Standing            func(faction uint32) (standing int32, ok bool)
}

// FactionTemplate returns a FactionTemplate.dbc row.
func (c *Catalog) FactionTemplate(id uint32) (FactionTemplate, bool) {
	v, ok := c.factionTemplates[id]
	return v, ok
}

// ReputationRank ports ReputationMgr::GetRank: base reputation of the
// race/class (FactionEntry::GetIndexFitTo) plus the stored standing; a
// faction without a FactionState counts as 0.
func (c *Catalog) ReputationRank(faction uint32, p PlayerReputation) int {
	raw, ok := c.factions[faction]
	if !ok {
		return ReputationToRank(0)
	}
	if p.Standing == nil {
		return ReputationToRank(0)
	}
	standing, has := p.Standing(faction)
	if !has {
		return ReputationToRank(0)
	}
	base := int32(0)
	for i := 0; i < 4; i++ {
		if (raw.RaceMask[i] == 0 || raw.RaceMask[i]&p.RaceMask != 0) && (raw.ClassMask[i] == 0 || raw.ClassMask[i]&p.ClassMask != 0) {
			base = raw.Base[i]
			break
		}
	}
	return ReputationToRank(base + standing)
}

// CreatureHostileToPlayer ports the creature-vs-player branch of
// Unit::IsHostileTo (UnitHostility.cpp): a creature faction with a
// reputation list index is hostile at rank <= REP_HOSTILE, anything else
// falls back to FactionTemplateEntry::IsHostileTo. Forced reactions
// (aura-driven) and combat/duel state do not exist in this server yet.
func (c *Catalog) CreatureHostileToPlayer(creatureTemplate, playerTemplate uint32, p PlayerReputation) bool {
	tester, ok1 := c.factionTemplates[creatureTemplate]
	target, ok2 := c.factionTemplates[playerTemplate]
	if !ok1 || !ok2 {
		return false
	}
	if tester.Faction != 0 {
		if raw, ok := c.factions[tester.Faction]; ok && raw.Index >= 0 {
			return c.ReputationRank(tester.Faction, p) <= RepHostile
		}
	}
	return tester.IsHostileTo(target)
}

// ReputationPriceDiscount ports Player::GetReputationPriceDiscount.
func (c *Catalog) ReputationPriceDiscount(creatureTemplate uint32, p PlayerReputation) float32 {
	vendor, ok := c.factionTemplates[creatureTemplate]
	if !ok || vendor.Faction == 0 {
		return 1.0
	}
	rank := c.ReputationRank(vendor.Faction, p)
	if rank <= RepNeutral {
		return 1.0
	}
	return 1.0 - 0.05*float32(rank-RepNeutral)
}

// HasFaction reports whether Faction.dbc has the faction id.
func (c *Catalog) HasFaction(id uint32) bool { _, ok := c.factions[id]; return ok }

// PlayerCondition returns a `conditions` row (sConditionStorage).
func (c *Catalog) PlayerCondition(id uint32) (entity.PlayerCondition, bool) {
	v, ok := c.conditions[id]
	return v, ok
}

// VendorItems returns the npc_vendor rows of a creature entry in load order
// (Creature::GetVendorItems); nil when the entry has none.
func (c *Catalog) VendorItems(entry uint32) []entity.VendorItem { return c.vendorItems[entry] }

// VendorTemplateItems returns the npc_vendor_template rows of a vendor
// template id (Creature::GetVendorTemplateItems).
func (c *Catalog) VendorTemplateItems(id uint32) []entity.VendorItem { return c.vendorTemplates[id] }

// CreatureSpawnByGUID returns the static creature spawn with the given
// `creature`.guid.
func (c *Catalog) CreatureSpawnByGUID(guid uint32) (CreatureSpawnRef, bool) {
	i, ok := c.creatureByGUID[guid]
	if !ok {
		return CreatureSpawnRef{}, false
	}
	return c.creatureSpawns[i], true
}

// EachCreatureSpawn calls fn for every creature spawn the reference puts on
// the grid at load time (the same set CreatureSpawnsNear scans).
func (c *Catalog) EachCreatureSpawn(fn func(CreatureSpawnRef)) {
	for _, s := range c.creatureSpawns {
		fn(s)
	}
}
