package gamedata

import "testing"

func TestReputationToRankBoundaries(t *testing.T) {
	cases := map[int32]int{-42000: RepHated, -6001: RepHated, -6000: RepHostile, -3000: 2, 0: RepNeutral, 2999: RepNeutral, 3000: RepFriendly, 9000: 5, 21000: 6, 42000: RepExalted, 42999: RepExalted}
	for standing, want := range cases {
		if got := ReputationToRank(standing); got != want {
			t.Fatalf("standing %d => rank %d, want %d", standing, got, want)
		}
	}
}

func testCatalogWithFactions() *Catalog {
	return &Catalog{
		factions: map[uint32]Faction{
			72: {ID: 72, Index: 5, RaceMask: [4]uint32{1, 0, 0, 0}, Base: [4]int32{3000, 0, 0, 0}}, // Stormwind, human base friendly
		},
		factionTemplates: map[uint32]FactionTemplate{
			1:  {ID: 1, Faction: 1, OurMask: 3, FriendlyMask: 2, HostileMask: 12},   // human player
			12: {ID: 12, Faction: 72, OurMask: 2, FriendlyMask: 3, HostileMask: 12}, // Stormwind guard
			85: {ID: 85, Faction: 0, OurMask: 8, FriendlyMask: 8, HostileMask: 1},   // hostile bandit group, no reputation
			29: {ID: 29, Faction: 0, OurMask: 4, FriendlyMask: 4, HostileMask: 0},   // orc-like group, not hostile to humans (mask 1)
		},
	}
}

func TestVendorDiscountAndHostility(t *testing.T) {
	c := testCatalogWithFactions()
	standing := map[uint32]int32{72: 0}
	p := PlayerReputation{RaceMask: 1, ClassMask: 1, Standing: func(f uint32) (int32, bool) { v, ok := standing[f]; return v, ok }}
	// base 3000 + 0 standing = Friendly (4): 5% discount.
	if d := c.ReputationPriceDiscount(12, p); d != 0.95 {
		t.Fatalf("discount = %v, want 0.95", d)
	}
	standing[72] = 9000 // 12000 total = Honored(5): 10%
	if d := c.ReputationPriceDiscount(12, p); d != 0.90 {
		t.Fatalf("discount = %v, want 0.90", d)
	}
	// Races that do not fit any base index fall back to base 0 (neutral).
	other := p
	other.RaceMask = 2
	standing[72] = 0
	if d := c.ReputationPriceDiscount(12, other); d != 1.0 {
		t.Fatalf("neutral discount = %v", d)
	}
	// Faction without reputation list index uses template masks.
	if !c.CreatureHostileToPlayer(85, 1, p) {
		t.Fatal("bandit template must be hostile to player template 1")
	}
	if c.CreatureHostileToPlayer(29, 1, p) {
		t.Fatal("friendly template reported hostile")
	}
	// Reputation-listed faction: hostile only at rank <= HOSTILE.
	standing[72] = -9000 // base 0 for non-human => -9000 => hated
	if !c.CreatureHostileToPlayer(12, 1, other) {
		t.Fatal("hated reputation must be hostile")
	}
	standing[72] = -3001 // human: 3000-3001 = -1 => unfriendly, still not hostile
	if c.CreatureHostileToPlayer(12, 1, p) {
		t.Fatal("unfriendly rank must not be hostile")
	}
	standing[72] = -9001 // human: -6001 => hated
	if !c.CreatureHostileToPlayer(12, 1, p) {
		t.Fatal("hated rank must be hostile")
	}
}
