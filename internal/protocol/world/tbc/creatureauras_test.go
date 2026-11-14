package tbc

import (
	"testing"

	"wowserver/internal/domain/aura"
	"wowserver/internal/entity"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world/tbc/codec"
)

func foeFixture(t *testing.T) (*Handler, creatureView) {
	t.Helper()
	h := &Handler{Data: &gamedata.Catalog{}}
	ref := gamedata.CreatureSpawnRef{GUID: 42, Data: entity.CreatureData{ID: 30, MapID: 1, PosX: 1, PosY: 2, PosZ: 3}}
	v := creatureView{ref: ref}
	return h, v
}

func TestSharedCreatureAurasMerge(t *testing.T) {
	h, v := foeFixture(t)
	// Two attackers put different debuffs on the creature.
	a := aura.NewHolder(100, 7, 10, 5000, false, [3]*aura.Effect{})
	b := aura.NewHolder(200, 9, 10, 5000, false, [3]*aura.Effect{})
	h.foeAuraAdd(v, a)
	h.foeAuraAdd(v, b)
	h.creatureAuras.mu.Lock()
	st := h.creatureAuras.m[42]
	if st == nil {
		t.Fatal("registry entry missing")
	}
	holders := st.set.Holders()
	words := h.auraWords(&st.set)
	h.creatureAuras.mu.Unlock()
	if len(holders) != 2 {
		t.Fatalf("both auras must merge, got %d", len(holders))
	}
	// Debuffs are negative: their slots live in the 40+ range.
	for _, hd := range holders {
		if words[48+int(hd.Slot)] != hd.SpellID {
			t.Fatalf("word of spell %d at slot %d is %d", hd.SpellID, hd.Slot, words[48+int(hd.Slot)])
		}
	}
	// The same spell of the same caster replaces its holder, not adds.
	h.foeAuraAdd(v, aura.NewHolder(100, 7, 10, 9000, false, [3]*aura.Effect{}))
	h.creatureAuras.mu.Lock()
	if got := len(h.creatureAuras.m[42].set.Holders()); got != 2 {
		t.Fatalf("replace must not add: %d", got)
	}
	h.creatureAuras.mu.Unlock()
	// Removing one holder keeps the other.
	h.foeAuraRemove(v, 100, 7)
	h.creatureAuras.mu.Lock()
	if got := len(h.creatureAuras.m[42].set.Holders()); got != 1 {
		t.Fatalf("single removal: %d", got)
	}
	if h.creatureAuras.m[42].sent[48+40] != 0 { // slot of the removed spell 100
		t.Fatalf("removed spell word must clear, got %d", h.creatureAuras.m[42].sent[88])
	}
	h.creatureAuras.mu.Unlock()
}

func TestSharedCreatureAurasClearOnDeath(t *testing.T) {
	h, v := foeFixture(t)
	h.foeAuraAdd(v, aura.NewHolder(100, 7, 10, 5000, false, [3]*aura.Effect{}))
	h.foeAuraAdd(v, aura.NewHolder(200, 9, 10, 5000, false, [3]*aura.Effect{}))
	h.foeAuraClearAll(v)
	h.creatureAuras.mu.Lock()
	if len(h.creatureAuras.m) != 0 {
		t.Fatal("death must clear the registry entry")
	}
	h.creatureAuras.mu.Unlock()
	if ch := h.creatureAuraWords(codec.CreatureGUID(30, 42)); len(ch) != 0 {
		t.Fatal("no words after death")
	}
}

func TestSharedCreatureAurasPurgeCaster(t *testing.T) {
	h, v := foeFixture(t)
	h.foeAuraAdd(v, aura.NewHolder(100, 7, 10, 5000, false, [3]*aura.Effect{}))
	h.foeAuraAdd(v, aura.NewHolder(200, 9, 10, 5000, false, [3]*aura.Effect{}))
	h.foeAuraPurgeCaster(7)
	h.creatureAuras.mu.Lock()
	st := h.creatureAuras.m[42]
	if st == nil || len(st.set.Holders()) != 1 || st.set.Holders()[0].Caster != 9 {
		t.Fatalf("purge must keep the other caster: %+v", st)
	}
	h.creatureAuras.mu.Unlock()
}
