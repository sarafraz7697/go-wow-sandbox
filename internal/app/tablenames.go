package app

import "strings"

// tableLabels gives the sections whose reference wording differs from the
// table name; everything else is derived from the table name.
var tableLabels = map[string][2]string{
	"creature_model_info":               {"Creature Model Based Info Data", "creature model based info"},
	"creature_template":                 {"Creature Templates", "creature definitions"},
	"creature_template_classlevelstats": {"Creature Class Level Stats", "creature class level stats"},
	"creature_equip_template":           {"Equipment Templates", "equipment templates"},
	"creature_addons":                   {"Creature Addon Data", "creature addons"},
	"creature":                          {"Creature Spawns", "creature spawns"},
	"gameobject_template":               {"Gameobject Templates", "gameobject templates"},
	"gameobject":                        {"Gameobject Spawns", "gameobject spawns"},
	"item_template":                     {"Item Prototypes", "item prototypes"},
	"reputation_reward_rate":            {"Reputation Reward Rates", "reputation_reward_rate"},
	"reputation_spillover_template":     {"Reputation Spillover Data", "reputation_spillover_template"},
	"quest_template":                    {"Quests", "quests"},
	"quest_relations":                   {"Quest Relations", "quest relations"},
	"player_levelstats":                 {"Player Level Stats", "player level stat rows"},
	"player_classlevelstats":            {"Player Class Level Stats", "class level stat rows"},
	"player_xp_for_level":               {"Player XP Table", "xp levels"},
	"mangos_string":                     {"MaNGOS Strings", "mangos strings"},
	"spell_template":                    {"Spell Templates", "spell templates"},
	"npc_text":                          {"Gossip Texts", "gossip texts"},
	"page_text":                         {"Page Texts", "page texts"},
}

// tableLabel returns the "Loading ..." title and the noun printed after the
// row count for a table.
func tableLabel(table string) (title, noun string) {
	if l, ok := tableLabels[table]; ok {
		return l[0], l[1]
	}
	noun = strings.ReplaceAll(table, "_", " ")
	words := strings.Fields(noun)
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " "), noun
}
