package characters

import "wowserver/internal/domain/character"

// Language skill lines (SkillLine.dbc, category 10) a race starts with: the
// faction language plus the racial one. The reference gets these from
// Player::learnDefaultSpells (playercreateinfo_spell -> SkillLineAbility,
// AcquireMethod 2) on EVERY login, so a character always has them.
var raceLanguageSkills = map[uint8][]uint16{
	1:  {98},       // Human: Common
	2:  {109},      // Orc: Orcish
	3:  {98, 111},  // Dwarf: Common, Dwarvish
	4:  {98, 113},  // Night Elf: Common, Darnassian
	5:  {109, 673}, // Undead: Orcish, Gutterspeak
	6:  {109, 115}, // Tauren: Orcish, Taurahe
	7:  {98, 313},  // Gnome: Common, Gnomish
	8:  {109, 315}, // Troll: Orcish, Troll
	10: {109, 137}, // Blood Elf: Orcish, Thalassian
	11: {98, 759},  // Draenei: Common, Draenei
}

var languageSkillIDs = map[uint32]bool{98: true, 109: true, 111: true, 113: true, 115: true, 137: true, 168: true, 313: true, 315: true, 673: true, 759: true}

// normalizeLanguageSkills ports Player::_LoadSkills' fixed range for
// SKILL_RANGE_LANGUAGE (value = max = 300) and Player::learnDefaultSpells'
// re-granting of the race's language skills.
func normalizeLanguageSkills(race uint8, skills []character.InitialSkill) []character.InitialSkill {
	have := map[uint32]bool{}
	for i := range skills {
		have[skills[i].ID] = true
		if languageSkillIDs[skills[i].ID] {
			skills[i].Value, skills[i].Max = 300, 300
		}
	}
	for _, id := range raceLanguageSkills[race] {
		if !have[uint32(id)] && len(skills) < 128 {
			skills = append(skills, character.InitialSkill{ID: uint32(id), Value: 300, Max: 300})
		}
	}
	return skills
}
