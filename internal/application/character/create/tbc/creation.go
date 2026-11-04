// Package tbc builds the build-8606 creation profile for every race/class
// pair the playercreateinfo table ships.
package tbc

// DOMAIN_USECASES_PATCH: moved from internal/application/character/tbc
// to internal/application/character/create/tbc. The Human/Warrior-only
// guard was removed in the CREATION_ALL_PAIRS patch: any pair the
// playercreateinfo table ships and the DBC permits is now accepted.

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"wowserver/internal/dbc"
	"wowserver/internal/domain/character"
	"wowserver/internal/gamedata"
)

type section struct{ race, gender, kind, variation, color, flags uint32 }
type hair struct{ race, gender, variation uint32 }
type ability struct{ skill, spell, race, class, method uint32 }
type Profile struct {
	data        *gamedata.Catalog
	sections    []section
	hair        []hair
	abilities   []ability
	categories  map[uint32]uint32
	reputations []character.InitialReputation
	// taxiCapital is the playercreateinfo-independent capital taxi node MaNGOS
	// unlocks for each race in PlayerTaxi::InitTaxiNodesForLevel, indexed by race.
	taxiCapital map[uint32]uint32
	// taxiOutland is the cross-faction Outland hub: 100 (Alliance) / 99 (Horde).
	taxiOutland map[uint32]uint32
}

// Load reads only static files. These layouts belong to TBC, not shared domain code.
func Load(ctx context.Context, dir string, data *gamedata.Catalog) (*Profile, error) {
	p := &Profile{
		data:        data,
		categories:  map[uint32]uint32{},
		taxiCapital: map[uint32]uint32{},
		taxiOutland: map[uint32]uint32{},
	}
	read := func(name string, fields uint32, consume func([]uint32)) error {
		f, err := dbc.Open(filepath.Join(dir, name+".dbc"), fields, fields*4)
		if err != nil {
			return err
		}
		if f.Len() == 0 {
			return fmt.Errorf("%s.dbc is empty", name)
		}
		for i := 0; i < f.Len(); i++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			v := make([]uint32, fields)
			for j := range v {
				v[j], err = f.Uint32(i, j*4)
				if err != nil {
					return err
				}
			}
			consume(v)
		}
		return nil
	}
	if err := read("CharSections", 10, func(v []uint32) { p.sections = append(p.sections, section{v[1], v[2], v[3], v[4], v[5], v[9]}) }); err != nil {
		return nil, err
	}
	if err := read("CharacterFacialHairStyles", 11, func(v []uint32) { p.hair = append(p.hair, hair{v[0], v[1], v[2]}) }); err != nil {
		return nil, err
	}
	if err := read("SkillLine", 38, func(v []uint32) { p.categories[v[0]] = v[1] }); err != nil {
		return nil, err
	}
	if err := read("SkillLineAbility", 15, func(v []uint32) { p.abilities = append(p.abilities, ability{v[1], v[2], v[3], v[4], v[9]}) }); err != nil {
		return nil, err
	}
	if err := read("Faction", 53, func(v []uint32) {
		if int32(v[1]) < 0 {
			return
		}
		flags := uint32(0)
		for i := 0; i < 4; i++ {
			if (v[2+i] == 0 || v[2+i]&1 != 0) && (v[6+i] == 0 || v[6+i]&1 != 0) {
				flags = v[14+i]
				break
			}
		}
		p.reputations = append(p.reputations, character.InitialReputation{Faction: v[0], Flags: flags})
	}); err != nil {
		return nil, err
	}
	// Capital taxi node for each race. Source: PlayerTaxi::InitTaxiNodesForLevel.
	// RACE_* are stable enum values; we keep the literals so a future DBC change
	// cannot silently move what the MaNGOS reference calls the "race-specific
	// initial known node". Outland hub is keyed by team (7 Alliance, 1 Horde).
	for race, node := range map[uint32]uint32{
		1: 2, 2: 23, 3: 6, 4: 26, 5: 11, 6: 22, 7: 6, 8: 23, 10: 82, 11: 94,
	} {
		p.taxiCapital[race] = node
	}
	// LearnDefaultSpells covers every spell listed in playercreateinfo_spell,
	// including language skills. A modified starting-spell set that ships a
	// learn-spell/trade-skill effect triggers a missing pipeline and we must
	// refuse rather than write an incomplete character_skill row.
	starting := map[uint32]bool{}
	for _, race := range data.RaceIDs() {
		for _, class := range data.ClassIDs() {
			if _, ok := data.Start(race, class); !ok {
				continue
			}
			for _, id := range data.StartSpells(race, class) {
				starting[id] = true
			}
		}
	}
	var unsupported uint32
	if err := read("Spell", 216, func(v []uint32) {
		if starting[v[0]] {
			for _, effect := range v[65:68] {
				if effect == 36 || effect == 44 || effect == 118 {
					unsupported = v[0]
				}
			}
		}
	}); err != nil {
		return nil, err
	}
	if unsupported != 0 {
		return nil, fmt.Errorf("starting spell %d requires an unsupported creation effect", unsupported)
	}
	return p, nil
}

func (p *Profile) appearance(r character.CreateRequest) bool {
	match := func(kind, variation, color uint32) bool {
		for _, s := range p.sections {
			if s.race == uint32(r.Race) && s.gender == uint32(r.Gender) && s.kind == kind && s.variation == variation && s.color == color && s.flags == 0 {
				return true
			}
		}
		return false
	}
	faceHair := false
	for _, h := range p.hair {
		if h.race == uint32(r.Race) && h.gender == uint32(r.Gender) && h.variation == uint32(r.FacialHair) {
			faceHair = true
			break
		}
	}
	return faceHair && match(0, 0, uint32(r.Skin)) && match(1, uint32(r.Face), uint32(r.Skin)) && match(3, uint32(r.HairStyle), uint32(r.HairColor))
}

// team returns 7 (Alliance) or 1 (Horde) for a race, matching ChrRaces'
// BaseLanguage field as read by Player::TeamForRace. 0 means unknown team.
func (p *Profile) team(race uint32) uint32 {
	r, ok := p.data.Race(race)
	if !ok {
		return 0
	}
	// Race.Team is BaseLanguage: 7=Alliance, 1=Horde.
	return r.Team
}

func (p *Profile) Build(r character.CreateRequest) (character.Creation, error) {
	var out character.Creation
	// Accept any pair the catalog ships; the domain guards against garbage.
	start, ok := p.data.Start(uint32(r.Race), uint32(r.Class))
	if !ok {
		return out, character.ErrDisabled
	}
	if r.Gender > 1 || !p.appearance(r) {
		return out, character.ErrAppearance
	}
	name, err := p.data.NormalizeName(r.Name, false)
	if err != nil {
		return out, err
	}
	r.Name = name
	for i := 2; i < len(name); i++ {
		a, b, c := name[i-2]|32, name[i-1]|32, name[i]|32
		if a == b && b == c {
			return out, character.ErrNameRepeated
		}
	}
	out.Request = r
	// Initial taxi mask mirrors PlayerTaxi::InitTaxiNodesForLevel exactly:
	// the race-specific capital, plus the Outland hub keyed by team.
	if node, ok := p.taxiCapital[uint32(r.Race)]; ok {
		out.TaxiMask[node/32] |= 1 << (node % 32)
	}
	switch p.team(uint32(r.Race)) {
	case 7: // Alliance
		out.TaxiMask[100/32] |= 1 << (100 % 32)
	case 1: // Horde
		out.TaxiMask[99/32] |= 1 << (99 % 32)
	}
	out.Start = start
	level, ok := p.data.Level(uint32(r.Race), uint32(r.Class), 1)
	if !ok {
		return out, fmt.Errorf("missing level stats race=%d class=%d", r.Race, r.Class)
	}
	base, ok := p.data.ClassLevel(uint32(r.Class), 1)
	if !ok {
		return out, fmt.Errorf("missing class stats class=%d", r.Class)
	}
	// Health at level 1 = base class health + the same stamina bonus the
	// reference applies via InitStatsForLevel + UpdateMaxHealth before the
	// first heartbeat. The 20-then-×10 curve is the classic TBC rule.
	stamina := level.Stats[2]
	bonus := stamina
	if stamina > 20 {
		bonus = 20 + (stamina-20)*10
	}
	out.Health = base.Health + bonus
	out.Spells = p.data.StartSpells(uint32(r.Race), uint32(r.Class))
	out.Actions = p.data.Actions(uint32(r.Race), uint32(r.Class))
	out.Reputations = append([]character.InitialReputation(nil), p.reputations...)
	known := map[uint32]bool{}
	for _, s := range out.Spells {
		known[s] = true
	}
	// SkillLineAbility maps starting spells to skills; a spell without an
	// ability row simply does not have a skill (e.g. racial passive). We
	// follow the MaNGOS order: weapon/armor/class skills get Max=5, language
	// skills get 300. Anything not in a tracked category is skipped.
	skills := map[uint32]character.InitialSkill{}
	for _, a := range p.abilities {
		if !known[a.spell] || a.method != 2 {
			continue
		}
		// Race mask: bit 0 set means Human, bit k set means race k+1. A zero
		// mask applies to all races; otherwise the spell is gated.
		if a.race != 0 && a.race&(1<<(uint32(r.Race)-1)) == 0 {
			continue
		}
		// Class mask uses the same bit-per-class layout.
		if a.class != 0 && a.class&(1<<(uint32(r.Class)-1)) == 0 {
			continue
		}
		s := character.InitialSkill{ID: a.skill, Value: 1, Max: 1}
		switch p.categories[a.skill] {
		case 10: // SKILL_CATEGORY_LANGUAGES — fluent at creation.
			s.Value = 300
			s.Max = 300
		case 6: // SKILL_CATEGORY_WEAPON — start at 1/5 unless it is Riding (473).
			if a.skill != 473 {
				s.Max = 5
			}
		case 7, 8: // SKILL_CATEGORY_CLASS, SKILL_CATEGORY_ARMOR — 1/5, except class skills.
			if a.skill == 40 || a.skill == 633 {
				s.Max = 5
			}
		default:
			continue
		}
		skills[s.ID] = s
	}
	for _, s := range skills {
		out.Skills = append(out.Skills, s)
	}
	sort.Slice(out.Skills, func(i, j int) bool { return out.Skills[i].ID < out.Skills[j].ID })
	// Outfit lookup is by race/class/gender/outfitId. The reference ignores
	// outfitId ("outfitId not used in player creation") and always picks the
	// first matching CharStartOutfit row, so we pass 0 and let the catalog
	// return the lowest id.
	outfit, ok := p.data.Outfit(uint32(r.Race), uint32(r.Class), r.Gender, 0)
	if !ok {
		return out, fmt.Errorf("missing starting outfit race=%d class=%d gender=%d", r.Race, r.Class, r.Gender)
	}
	occupied := map[uint8]bool{}
	backpack := uint8(23)
	add := func(id, count uint32) error {
		item, ok := p.data.Item(id)
		if !ok || count == 0 || item.Stackable < 1 || item.Duration != 0 {
			return fmt.Errorf("unsupported initial item %d", id)
		}
		// This profile supports ordinary armor, one-handed weapons, shields and backpack consumables.
		if item.Class == 1 || item.InventoryType == 18 || item.InventoryType == 24 {
			return fmt.Errorf("unsupported container/ammo item %d", id)
		}
		slot := uint8(255)
		switch item.InventoryType {
		case 4:
			slot = 3
		case 7:
			slot = 6
		case 8:
			slot = 7
		case 13, 21:
			slot = 15
		case 14:
			slot = 16
		case 0:
		default:
			return fmt.Errorf("unsupported starting inventory type %d", item.InventoryType)
		}
		for count > 0 {
			n := min(count, uint32(item.Stackable))
			dest := slot
			if dest == 255 || occupied[dest] {
				if backpack >= 39 {
					return fmt.Errorf("initial backpack full")
				}
				dest = backpack
				backpack++
			} else {
				n = 1
			}
			occupied[dest] = true
			out.Items = append(out.Items, character.InitialItem{Template: item, Slot: dest, Count: n})
			count -= n
		}
		return nil
	}
	for _, id := range outfit.Items {
		if id <= 0 {
			continue
		}
		item, ok := p.data.Item(uint32(id))
		if !ok {
			return out, fmt.Errorf("missing item %d", id)
		}
		count := item.BuyCount
		if item.Class == 0 && item.Subclass == 5 {
			if item.FoodCategory == 11 && item.Stackable > 4 {
				count = 4
			}
			if item.FoodCategory == 59 && item.Stackable > 2 {
				count = 2
			}
		}
		if err := add(uint32(id), count); err != nil {
			return out, err
		}
	}
	for _, item := range p.data.StartItems(uint32(r.Race), uint32(r.Class)) {
		if err := add(item.Item, item.Amount); err != nil {
			return out, err
		}
	}
	return out, nil
}
