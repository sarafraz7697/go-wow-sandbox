package worldload

import (
	"context"

	"wowserver/internal/entity"
)

// itemTemplateLoader ports ObjectMgr::LoadItemPrototypes
// (sItemStorage, SQLStorage format "iiiisiiiis…"): the complete
// item_template row including the 10 stat pairs, 5 damage triples, 7
// resistances and 5 spell blocks — the full ItemPrototype struct, not a
// subset. Column order follows the physical schema order.
func init() {
	Register(Loader{
		Name:       "item_template",
		Order:      OrderItemTemplate,
		CountQuery: "SELECT COUNT(*) FROM `item_template`",
		Load:       loadItemPrototypes,
	})
}

func itemTemplateQuery() string {
	q := "SELECT `entry`,`class`,`subclass`,`unk0`,`name`,`displayid`,`Quality`,`Flags`,`BuyCount`,`BuyPrice`,`SellPrice`," +
		"`InventoryType`,`AllowableClass`,`AllowableRace`,`ItemLevel`,`RequiredLevel`," +
		"`RequiredSkill`,`RequiredSkillRank`,`requiredspell`,`requiredhonorrank`,`RequiredCityRank`,`RequiredReputationFaction`,`RequiredReputationRank`," +
		"`maxcount`,`stackable`,`ContainerSlots`"
	for i := 1; i <= entity.MaxItemProtoStats; i++ {
		q += ",`stat_type" + itoa(i) + "`,`stat_value" + itoa(i) + "`"
	}
	for i := 1; i <= entity.MaxItemProtoDamages; i++ {
		q += ",`dmg_min" + itoa(i) + "`,`dmg_max" + itoa(i) + "`,`dmg_type" + itoa(i) + "`"
	}
	q += ",`armor`,`holy_res`,`fire_res`,`nature_res`,`frost_res`,`shadow_res`,`arcane_res`," +
		"`delay`,`ammo_type`,`RangedModRange`"
	for i := 1; i <= entity.MaxItemProtoSpells; i++ {
		q += ",`spellid_" + itoa(i) + "`,`spelltrigger_" + itoa(i) + "`,`spellcharges_" + itoa(i) +
			"`,`spellppmRate_" + itoa(i) + "`,`spellcooldown_" + itoa(i) + "`,`spellcategory_" + itoa(i) +
			"`,`spellcategorycooldown_" + itoa(i) + "`"
	}
	q += ",`bonding`,`description`,`PageText`,`LanguageID`,`PageMaterial`,`startquest`,`lockid`,`Material`,`sheath`," +
		"`RandomProperty`,`RandomSuffix`,`block`,`itemset`,`MaxDurability`,`area`,`Map`,`BagFamily`,`TotemCategory`"
	for i := 1; i <= entity.MaxItemProtoSockets; i++ {
		q += ",`socketColor_" + itoa(i) + "`,`socketContent_" + itoa(i) + "`"
	}
	q += ",`socketBonus`,`GemProperties`,`RequiredDisenchantSkill`,`ArmorDamageModifier`,`duration`" +
		" FROM `item_template` ORDER BY `entry`"
	return q
}

func loadItemPrototypes(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t, itemTemplateQuery(), 5000, func(r scanner) error {
		var v entity.ItemPrototype
		dest2 := []any{
			u32(&v.ItemID), u32(&v.Class), u32(&v.SubClass), u32(&v.Unk0), str(&v.Name1), u32(&v.DisplayInfoID),
			u32(&v.Quality), u32(&v.Flags), u32(&v.BuyCount), u32(&v.BuyPrice), u32(&v.SellPrice),
			u32(&v.InventoryType), i32(&v.AllowableClass), i32(&v.AllowableRace), u32(&v.ItemLevel), u32(&v.RequiredLevel),
			u32(&v.RequiredSkill), u32(&v.RequiredSkillRank), u32(&v.RequiredSpell), u32(&v.RequiredHonorRank), u32(&v.RequiredCityRank),
			u32(&v.RequiredReputationFaction), u32(&v.RequiredReputationRank),
			u32(&v.MaxCount), i32(&v.Stackable), u32(&v.ContainerSlots),
		}
		for i := range v.ItemStat {
			dest2 = append(dest2, u32(&v.ItemStat[i].ItemType), i32(&v.ItemStat[i].Value))
		}
		for i := range v.Damage {
			dest2 = append(dest2, f32(&v.Damage[i].DamageMin), f32(&v.Damage[i].DamageMax), u32(&v.Damage[i].DamageType))
		}
		dest2 = append(dest2,
			u32(&v.Armor), u32(&v.HolyRes), u32(&v.FireRes), u32(&v.NatureRes), u32(&v.FrostRes), u32(&v.ShadowRes), u32(&v.ArcaneRes),
			u32(&v.Delay), u32(&v.AmmoType), f32(&v.RangedModRange))
		for i := range v.Spells {
			s := &v.Spells[i]
			dest2 = append(dest2, u32(&s.SpellID), u32(&s.SpellTrigger), i32(&s.SpellCharges), f32(&s.SpellPPMRate), i32(&s.SpellCooldown), u32(&s.SpellCategory), i32(&s.SpellCategoryCooldown))
		}
		dest2 = append(dest2,
			u32(&v.Bonding), str(&v.Description),
			u32(&v.PageText), u32(&v.LanguageID), u32(&v.PageMaterial), u32(&v.StartQuest), u32(&v.LockID), i32(&v.Material), u32(&v.Sheath),
			u32(&v.RandomProperty), u32(&v.RandomSuffix), u32(&v.Block), u32(&v.ItemSet), u32(&v.MaxDurability), u32(&v.Area), i32(&v.Map), u32(&v.BagFamily), u32(&v.TotemCategory))
		for i := range v.Socket {
			dest2 = append(dest2, u32(&v.Socket[i].SocketColor), u32(&v.Socket[i].SocketContent))
		}
		dest2 = append(dest2, u32(&v.SocketBonus), u32(&v.GemProperties), i32(&v.RequiredDisenchantSkill), f32(&v.ArmorDamageModifier), u32(&v.Duration))

		if err := r.Scan(dest2...); err != nil {
			return err
		}
		// Reference behavior: entry zero is not a usable item and is
		// skipped with a warning (the validation loop starts at 1).
		if v.ItemID == 0 {
			t.Skipf("item_template entry 0 is not a valid prototype")
			return nil
		}
		if _, dup := dest.ItemPrototypes[v.ItemID]; dup {
			t.Skipf("item_template has duplicate entry %d", v.ItemID)
			return nil
		}
		dest.ItemPrototypes[v.ItemID] = v
		dest.ItemPrototypeOrder = append(dest.ItemPrototypeOrder, v.ItemID)
		t.Stored(1)
		return nil
	})
}
