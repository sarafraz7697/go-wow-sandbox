package stats

import (
	"testing"

	"wowserver/internal/gamedata"
)

func TestAddItemStatRating(t *testing.T) {
	var r [24]int32
	// The specific ratings map one to one.
	if !AddItemStatRating(13, 12, &r) || r[gamedata.CRDodge] != 12 {
		t.Fatalf("dodge rating %+v", r)
	}
	// The combined ratings feed several slots (Player::_ApplyItemBonuses).
	if !AddItemStatRating(31, 10, &r) {
		t.Fatal("hit rating is a known kind")
	}
	if r[gamedata.CRHitMelee] != 10 || r[gamedata.CRHitRanged] != 10 {
		t.Fatalf("hit rating feeds melee+ranged: %+v", r)
	}
	if !AddItemStatRating(35, 20, &r) {
		t.Fatal("resilience is a known kind")
	}
	for _, cr := range []uint32{gamedata.CRCritTakenMelee, gamedata.CRCritTakenRanged, gamedata.CRCritTakenSpell} {
		if r[cr] != 20 {
			t.Fatalf("resilience feeds crit taken %d: %d", cr, r[cr])
		}
	}
	// Non-rating kinds stay unknown.
	var none [24]int32
	if AddItemStatRating(5, 10, &none) {
		t.Fatal("intellect is not a rating kind")
	}
	if AddItemStatRating(38, 10, &none) {
		t.Fatal("38 is not a known kind")
	}
}
