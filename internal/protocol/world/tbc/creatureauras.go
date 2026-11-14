package tbc

import (
	"sync"
	"time"

	"wowserver/internal/domain/aura"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world/tbc/codec"
)

// creatureAuraState is the merged aura display of one creature spawn: every
// attacker's holders in one set plus the field words the clients were last
// sent. The reference keeps one aura list per unit (Unit::m_Auras); merging
// the per-attacker sets here is what makes the UNIT_FIELD_AURA* words the
// same for every observer.
type creatureAuraState struct {
	ref  gamedata.CreatureSpawnRef
	set  aura.Set
	sent map[int]uint32
}

// creatureAuraRegistry maps spawn guids to their merged aura display. The
// mutex is held while holders and words change; broadcasts happen after it is
// released.
type creatureAuraRegistry struct {
	mu sync.Mutex
	m  map[uint32]*creatureAuraState
}

// diff returns the field changes between want and the words the clients were
// last sent, recording the new state.
func (st *creatureAuraState) diff(want map[int]uint32) []codec.FieldChange {
	var ch []codec.FieldChange
	for idx, val := range want {
		if st.sent[idx] != val {
			st.sent[idx] = val
			ch = append(ch, codec.FieldChange{Index: idx, Value: val})
		}
	}
	return ch
}

// cloneHolder copies a holder so the merged set can assign its own slots
// without touching the attacker's logic set.
func cloneHolder(h *aura.Holder) *aura.Holder {
	c := *h
	for i, e := range h.Effects {
		if e != nil {
			ec := *e
			c.Effects[i] = &ec
		}
	}
	return &c
}

// foeAuraAdd merges one holder into the creature's shared aura display and
// broadcasts the changed words to every nearby client.
func (h *Handler) foeAuraAdd(v creatureView, hd *aura.Holder) {
	h.creatureAuras.mu.Lock()
	if h.creatureAuras.m == nil {
		h.creatureAuras.m = map[uint32]*creatureAuraState{}
	}
	st := h.creatureAuras.m[v.ref.GUID]
	if st == nil {
		st = &creatureAuraState{ref: v.ref, sent: map[int]uint32{}}
		h.creatureAuras.m[v.ref.GUID] = st
	}
	var ch []codec.FieldChange
	if _, ok := st.set.Add(cloneHolder(hd)); ok {
		ch = st.diff(h.auraWords(&st.set))
	}
	h.creatureAuras.mu.Unlock()
	if len(ch) > 0 {
		h.broadcastCreature(v.ref, codec.ValuesUpdate(codec.CreatureGUID(v.ref.Data.ID, v.ref.GUID), ch), time.Now())
	}
}

// foeAuraRemove drops one holder (spell and caster) from the shared display.
func (h *Handler) foeAuraRemove(v creatureView, spellID uint32, caster uint64) {
	h.creatureAuras.mu.Lock()
	st := h.creatureAuras.m[v.ref.GUID]
	var ch []codec.FieldChange
	if st != nil {
		for _, hd := range st.set.Holders() {
			if hd.SpellID == spellID && hd.Caster == caster {
				st.set.Remove(hd.Slot)
			}
		}
		ch = st.diff(h.auraWords(&st.set))
		if len(st.set.Holders()) == 0 {
			delete(h.creatureAuras.m, v.ref.GUID)
		}
	}
	h.creatureAuras.mu.Unlock()
	if len(ch) > 0 {
		h.broadcastCreature(v.ref, codec.ValuesUpdate(codec.CreatureGUID(v.ref.Data.ID, v.ref.GUID), ch), time.Now())
	}
}

// foeAuraClearAll removes every holder of the creature (death) and clears the
// words on all clients.
func (h *Handler) foeAuraClearAll(v creatureView) {
	h.creatureAuras.mu.Lock()
	st := h.creatureAuras.m[v.ref.GUID]
	var ch []codec.FieldChange
	if st != nil {
		st.set = aura.Set{}
		ch = st.diff(h.auraWords(&st.set))
		delete(h.creatureAuras.m, v.ref.GUID)
	}
	h.creatureAuras.mu.Unlock()
	if len(ch) > 0 {
		h.broadcastCreature(v.ref, codec.ValuesUpdate(codec.CreatureGUID(v.ref.Data.ID, v.ref.GUID), ch), time.Now())
	}
}

// foeAuraPurgeCaster removes every holder of one caster (logout).
func (h *Handler) foeAuraPurgeCaster(caster uint64) {
	type change struct {
		ref gamedata.CreatureSpawnRef
		ch  []codec.FieldChange
	}
	var out []change
	h.creatureAuras.mu.Lock()
	for guid, st := range h.creatureAuras.m {
		removed := false
		for _, hd := range st.set.Holders() {
			if hd.Caster == caster {
				st.set.Remove(hd.Slot)
				removed = true
			}
		}
		if removed {
			out = append(out, change{ref: st.ref, ch: st.diff(h.auraWords(&st.set))})
			if len(st.set.Holders()) == 0 {
				delete(h.creatureAuras.m, guid)
			}
		}
	}
	h.creatureAuras.mu.Unlock()
	for _, c := range out {
		if len(c.ch) > 0 {
			h.broadcastCreature(c.ref, codec.ValuesUpdate(codec.CreatureGUID(c.ref.Data.ID, c.ref.GUID), c.ch), time.Now())
		}
	}
}

// creatureAuraWords is the full aura word list of a creature for a client that
// just created it (every non-zero word, no diff).
func (h *Handler) creatureAuraWords(guid uint64) []codec.FieldChange {
	entry, counter, ok := codec.DecodeCreatureGUID(guid)
	if !ok {
		return nil
	}
	h.creatureAuras.mu.Lock()
	st := h.creatureAuras.m[counter]
	var ch []codec.FieldChange
	if st != nil {
		for idx, val := range h.auraWords(&st.set) {
			if val != 0 {
				ch = append(ch, codec.FieldChange{Index: idx, Value: val})
			}
		}
	}
	h.creatureAuras.mu.Unlock()
	_ = entry
	return ch
}
