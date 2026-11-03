// Package tbc builds the deliberately limited build-8606 creation profile.
package tbc

// PACKAGE_SKELETON_PATCH: profile lives under application/character/tbc

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"wowserver/internal/domain/character"
	"wowserver/internal/dbc"
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
}

// Load reads only static files. These layouts belong to TBC, not shared domain code.
func Load(ctx context.Context, dir string, data *gamedata.Catalog) (*Profile, error) {
	p := &Profile{data: data, categories: map[uint32]uint32{}}
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
	// Learned-spell and skill-step effects need another initializer. Refuse
	// modified starting spell sets that would silently produce incomplete rows.
	starting := map[uint32]bool{}
	for _, id := range data.StartSpells(1, 1) {
		starting[id] = true
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
			if s.race == 1 && s.gender == uint32(r.Gender) && s.kind == kind && s.variation == variation && s.color == color && s.flags == 0 {
				return true
			}
		}
		return false
	}
	faceHair := false
	for _, h := range p.hair {
		if h.race == 1 && h.gender == uint32(r.Gender) && h.variation == uint32(r.FacialHair) {
			faceHair = true
			break
		}
	}
	return faceHair && match(0, 0, uint32(r.Skin)) && match(1, uint32(r.Face), uint32(r.Skin)) && match(3, uint32(r.HairStyle), uint32(r.HairColor))
}

func (p *Profile) Build(r character.CreateRequest) (character.Creation, error) {
	var out character.Creation
	if r.Race != 1 || r.Class != 1 {
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
	// Human capital (2) and Alliance Outland hub (100), as InitTaxiNodesForLevel.
	out.TaxiMask[0], out.TaxiMask[3] = 2, 8
	start, ok := p.data.Start(1, 1)
	if !ok {
		return out, fmt.Errorf("missing Human Warrior start")
	}
	out.Start = start
	level, ok := p.data.Level(1, 1, 1)
	if !ok {
		return out, fmt.Errorf("missing level stats")
	}
	base, ok := p.data.ClassLevel(1, 1)
	if !ok {
		return out, fmt.Errorf("missing class stats")
	}
	stamina := level.Stats[2]
	bonus := stamina
	if stamina > 20 {
		bonus = 20 + (stamina-20)*10
	}
	out.Health = base.Health + bonus
	out.Spells = p.data.StartSpells(1, 1)
	out.Actions = p.data.Actions(1, 1)
	out.Reputations = append([]character.InitialReputation(nil), p.reputations...)
	known := map[uint32]bool{}
	for _, s := range out.Spells {
		known[s] = true
	}
	skills := map[uint32]character.InitialSkill{}
	for _, a := range p.abilities {
		if !known[a.spell] || a.method != 2 || (a.race != 0 && a.race&1 == 0) || (a.class != 0 && a.class&1 == 0) {
			continue
		}
		s := character.InitialSkill{ID: a.skill, Value: 1, Max: 1}
		switch p.categories[a.skill] {
		case 10:
			s.Value = 300
			s.Max = 300
		case 6:
			if a.skill != 473 {
				s.Max = 5
			}
		case 7, 8:
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
	outfit, ok := p.data.Outfit(1, 1, r.Gender, 0)
	if !ok {
		return out, fmt.Errorf("missing Human Warrior outfit")
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
	for _, item := range p.data.StartItems(1, 1) {
		if err := add(item.Item, item.Amount); err != nil {
			return out, err
		}
	}
	return out, nil
}
