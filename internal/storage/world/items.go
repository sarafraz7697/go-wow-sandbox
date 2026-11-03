package world

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"wowserver/internal/gamedata"
)

func (s *Store) loadItemDetails(ctx context.Context) ([]gamedata.ItemDetails, error) {
	columns := []string{"entry", "name", "description", "Quality", "Flags", "BuyPrice", "SellPrice", "AllowableClass", "AllowableRace", "ItemLevel", "RequiredLevel", "RequiredSkill", "RequiredSkillRank", "requiredspell", "requiredhonorrank", "RequiredCityRank", "RequiredReputationFaction", "RequiredReputationRank", "maxcount", "ContainerSlots"}
	for i := 1; i <= 10; i++ {
		columns = append(columns, fmt.Sprintf("stat_type%d", i), fmt.Sprintf("stat_value%d", i))
	}
	for i := 1; i <= 5; i++ {
		columns = append(columns, fmt.Sprintf("dmg_min%d", i), fmt.Sprintf("dmg_max%d", i), fmt.Sprintf("dmg_type%d", i))
	}
	columns = append(columns, "armor", "holy_res", "fire_res", "nature_res", "frost_res", "shadow_res", "arcane_res", "delay", "ammo_type", "RangedModRange")
	for i := 1; i <= 5; i++ {
		for _, name := range []string{"spellid", "spelltrigger", "spellcharges", "spellcooldown", "spellcategory", "spellcategorycooldown"} {
			columns = append(columns, fmt.Sprintf("%s_%d", name, i))
		}
	}
	columns = append(columns, "PageText", "LanguageID", "PageMaterial", "startquest", "lockid", "Material", "sheath", "RandomProperty", "RandomSuffix", "block", "itemset", "area", "Map", "BagFamily", "TotemCategory")
	for i := 1; i <= 3; i++ {
		columns = append(columns, fmt.Sprintf("socketColor_%d", i), fmt.Sprintf("socketContent_%d", i))
	}
	columns = append(columns, "socketBonus", "GemProperties", "RequiredDisenchantSkill", "ArmorDamageModifier")
	var out []gamedata.ItemDetails
	err := s.readStatic(ctx, "SELECT "+strings.Join(columns, ",")+" FROM item_template WHERE entry<>0 ORDER BY entry", func(r *sql.Rows) error {
		var v gamedata.ItemDetails
		dest := []any{&v.ID, &v.Name, &v.Description, &v.Quality, &v.Flags, &v.BuyPrice, &v.SellPrice, &v.AllowableClass, &v.AllowableRace, &v.ItemLevel, &v.RequiredLevel, &v.RequiredSkill, &v.RequiredSkillRank, &v.RequiredSpell, &v.RequiredHonorRank, &v.RequiredCityRank, &v.RequiredReputationFaction, &v.RequiredReputationRank, &v.MaxCount, &v.ContainerSlots}
		for i := range v.Stats {
			dest = append(dest, &v.Stats[i].Type, &v.Stats[i].Value)
		}
		for i := range v.Damage {
			dest = append(dest, &v.Damage[i].Min, &v.Damage[i].Max, &v.Damage[i].Type)
		}
		for i := range v.Resistances {
			dest = append(dest, &v.Resistances[i])
		}
		dest = append(dest, &v.Delay, &v.AmmoType, &v.RangedModRange)
		for i := range v.Spells {
			x := &v.Spells[i]
			dest = append(dest, &x.ID, &x.Trigger, &x.Charges, &x.Cooldown, &x.Category, &x.CategoryCooldown)
		}
		dest = append(dest, &v.PageText, &v.Language, &v.PageMaterial, &v.StartQuest, &v.Lock, &v.Material, &v.Sheath, &v.RandomProperty, &v.RandomSuffix, &v.Block, &v.ItemSet, &v.Area, &v.Map, &v.BagFamily, &v.TotemCategory)
		for i := range v.Sockets {
			dest = append(dest, &v.Sockets[i].Color, &v.Sockets[i].Content)
		}
		dest = append(dest, &v.SocketBonus, &v.GemProperties, &v.DisenchantSkill, &v.ArmorDamageModifier)
		if err := r.Scan(dest...); err != nil {
			return err
		}
		out = append(out, v)
		return nil
	})
	return out, err
}
