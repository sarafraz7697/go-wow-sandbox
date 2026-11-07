package tbc

import (
	"sort"

	"wowserver/internal/domain/character"
)

// normalizeLoadedPlayer applies what the reference does while loading a
// character (Player::LoadFromDB) and the Go loader does not do on its own:
//   - learnDefaultSpells: the race/class start spells (playercreateinfo_spell,
//     which carry the language spells) are added on every login;
//   - _LoadHomeBind: a missing bind row falls back to the race/class start;
//   - _LoadInventory: items whose template is unknown are dropped (with their
//     contents) instead of rejecting the whole login.
func (h *Handler) normalizeLoadedPlayer(s *Session, p *character.Player) {
	for _, w := range p.LoadWarnings {
		s.log.Warn("character data skipped at login", "guid", p.GUID, "detail", w)
	}
	have := make(map[uint32]bool, len(p.Spells))
	for _, id := range p.Spells {
		have[id] = true
	}
	added := false
	for _, id := range h.Data.StartSpells(uint32(p.Race), uint32(p.Class)) {
		if !have[id] {
			p.Spells = append(p.Spells, id)
			have[id] = true
			added = true
		}
	}
	if added {
		sort.Slice(p.Spells, func(i, j int) bool { return p.Spells[i] < p.Spells[j] })
	}
	if p.HomeMissing {
		if st, ok := h.Data.Start(uint32(p.Race), uint32(p.Class)); ok {
			p.HomeMap, p.HomeZone, p.HomeX, p.HomeY, p.HomeZ = st.Map, st.Zone, st.X, st.Y, st.Z
		}
	}
	dropped := map[uint32]bool{}
	for _, it := range p.Items {
		if _, ok := h.Data.ItemPrototype(it.Entry); !ok {
			dropped[it.GUID] = true
			s.log.Warn("item with unknown template ignored at login", "guid", p.GUID, "item", it.GUID, "entry", it.Entry)
		}
	}
	if len(dropped) == 0 {
		return
	}
	kept := p.Items[:0]
	for _, it := range p.Items {
		if !dropped[it.GUID] && !(it.Bag != 0 && dropped[it.Bag]) {
			kept = append(kept, it)
		}
	}
	p.Items = kept
}
