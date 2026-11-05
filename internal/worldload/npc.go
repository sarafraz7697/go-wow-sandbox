package worldload

import (
	"context"

	"wowserver/internal/entity"
)

// The vendor, trainer and gossip loaders port
// ObjectMgrTrainerVendor.cpp (LoadVendors/LoadVendorTemplates/
// LoadTrainers/LoadTrainerTemplates) and ObjectMgrGossipMenu.cpp
// (LoadGossipMenu/LoadGossipMenuItems/LoadGossipText).
//
// The reference validates every vendor item against item_template and
// every trainer spell against Spell.dbc before storing it. The item
// check runs here against the already-loaded item prototypes (item
// loads earlier in the sequence); the spell check happens later against
// the DBC-loaded spell set (the gamedata catalog owns that join), so
// this loader keeps unknown spells and the catalog reports them.

func init() {
	Register(Loader{
		Name:       "npc_vendor",
		Order:      OrderVendors,
		CountQuery: "SELECT COUNT(*) FROM `npc_vendor`",
		Load: func(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
			return scanVendors(ctx, q, t, dest, "npc_vendor", dest.NpcVendors)
		},
	})
	Register(Loader{
		Name:       "npc_vendor_template",
		Order:      OrderVendorTemplates,
		CountQuery: "SELECT COUNT(*) FROM `npc_vendor_template`",
		Load: func(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
			return scanVendors(ctx, q, t, dest, "npc_vendor_template", dest.NpcVendorTemplates)
		},
	})
}

// scanVendors reads a vendor table (entry,item,maxcount,incrtime,
// ExtendedCost,condition_id) into a multimap keyed by the npc entry.
func scanVendors(ctx context.Context, q Queryer, t *Table, dest *Dataset, table string, into map[uint32][]entity.VendorItem) error {
	return scanTable(ctx, q, t,
		"SELECT `entry`,`item`,`maxcount`,`incrtime`,`ExtendedCost`,`condition_id` FROM `"+table+"`",
		2000, func(r scanner) error {
			var entry uint32
			var v entity.VendorItem
			if err := r.Scan(u32(&entry), u32(&v.Item), u32(&v.MaxCount), u32(&v.IncrTime), u32(&v.ExtendedCost), u32(&v.ConditionID)); err != nil {
				return err
			}
			// Reference validation (IsVendorItemValid): the item must
			// exist in item_template.
			if _, ok := dest.ItemPrototypes[v.Item]; !ok {
				t.Skipf("Table `npc_vendor` has item %d that does not exist in item_template, ignore", v.Item)
				return nil
			}
			into[entry] = append(into[entry], v)
			t.Stored(1)
			return nil
		})
}

func init() {
	Register(Loader{
		Name:       "npc_trainer",
		Order:      OrderTrainers,
		CountQuery: "SELECT COUNT(*) FROM `npc_trainer`",
		Load: func(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
			return scanTrainers(ctx, q, t, dest, "npc_trainer", dest.NpcTrainers)
		},
	})
	Register(Loader{
		Name:       "npc_trainer_template",
		Order:      OrderTrainerTemplates,
		CountQuery: "SELECT COUNT(*) FROM `npc_trainer_template`",
		Load: func(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
			return scanTrainers(ctx, q, t, dest, "npc_trainer_template", dest.NpcTrainerTemplates)
		},
	})
}

func scanTrainers(ctx context.Context, q Queryer, t *Table, dest *Dataset, table string, into map[uint32][]entity.TrainerSpell) error {
	return scanTable(ctx, q, t,
		"SELECT `entry`,`spell`,`spellcost`,`reqskill`,`reqskillvalue`,`reqlevel` FROM `"+table+"`",
		2000, func(r scanner) error {
			var entry uint32
			var v entity.TrainerSpell
			if err := r.Scan(u32(&entry), u32(&v.Spell), u32(&v.SpellCost), u32(&v.RequiredSkill), u32(&v.RequiredSkillValue), i32(&v.ReqLevel)); err != nil {
				return err
			}
			into[entry] = append(into[entry], v)
			t.Stored(1)
			return nil
		})
}

func init() {
	Register(Loader{
		Name:       "gossip_menu",
		Order:      OrderGossipMenus,
		CountQuery: "SELECT COUNT(*) FROM `gossip_menu`",
		Load:       loadGossipMenus,
	})
	Register(Loader{
		Name:       "gossip_menu_option",
		Order:      OrderGossipMenus + 1,
		CountQuery: "SELECT COUNT(*) FROM `gossip_menu_option`",
		Load:       loadGossipMenuOptions,
	})
}

func loadGossipMenus(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `entry`,`text_id`,`script_id`,`condition_id` FROM `gossip_menu`",
		2000, func(r scanner) error {
			var v entity.GossipMenu
			if err := r.Scan(u32(&v.Entry), u32(&v.TextID), u32(&v.ScriptID), u32(&v.ConditionID)); err != nil {
				return err
			}
			if v.Entry == 0 {
				t.Skipf("gossip_menu entry 0 is not a valid menu")
				return nil
			}
			dest.GossipMenus[v.Entry] = append(dest.GossipMenus[v.Entry], v)
			t.Stored(1)
			return nil
		})
}

func loadGossipMenuOptions(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `menu_id`,`id`,`option_icon`,`option_text`,`option_id`,`npc_option_npcflag`,`action_menu_id`,`action_poi_id`,`action_script_id`,`box_coded`,`box_money`,`box_text`,`condition_id` FROM `gossip_menu_option` ORDER BY `menu_id`,`id`",
		2000, func(r scanner) error {
			var v entity.GossipMenuOption
			if err := r.Scan(u32(&v.MenuID), u32(&v.ID), u32(&v.OptionIcon), str(&v.OptionText), u32(&v.OptionID), u32(&v.NpcOptionNpcflag),
				u32(&v.ActionMenuID), u32(&v.ActionPoiID), u32(&v.ActionScriptID), boolean(&v.BoxCoded), u32(&v.BoxMoney), str(&v.BoxText), u32(&v.ConditionID)); err != nil {
				return err
			}
			if v.MenuID == 0 {
				t.Skipf("gossip_menu_option menu_id 0 is not a valid option")
				return nil
			}
			dest.GossipMenuOptions[v.MenuID] = append(dest.GossipMenuOptions[v.MenuID], v)
			t.Stored(1)
			return nil
		})
}

// npcTextLoader ports ObjectMgr::LoadGossipText: the npc_text table with
// its 8 broadcast-text blocks (two text variants, language, probability
// and three emote/delay pairs each).
func init() {
	Register(Loader{
		Name:       "npc_text",
		Order:      OrderNpcText,
		CountQuery: "SELECT COUNT(*) FROM `npc_text`",
		Load:       loadNpcTexts,
	})
}

func npcTextQuery() string {
	q := "SELECT `ID`"
	for i := 0; i < 8; i++ {
		q += ",`text" + itoa(i) + "_0`,`text" + itoa(i) + "_1`,`lang" + itoa(i) + "`,`prob" + itoa(i) + "`"
		for e := 0; e < 3; e++ {
			q += ",`em" + itoa(i) + "_" + itoa(e) + "_delay`,`em" + itoa(i) + "_" + itoa(e) + "`"
		}
	}
	q += " FROM `npc_text` ORDER BY `ID`"
	return q
}

func loadNpcTexts(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t, npcTextQuery(), 1000, func(r scanner) error {
		var v entity.NpcText
		dest2 := []any{u32(&v.ID)}
		for i := range v.Blocks {
			b := &v.Blocks[i]
			dest2 = append(dest2, str(&b.Text0), str(&b.Text1), u32(&b.Lang), f32(&b.Prob))
			for e := 0; e < 3; e++ {
				dest2 = append(dest2, u32(&b.EmoteDelay[e]), u32(&b.Emote[e]))
			}
		}
		if err := r.Scan(dest2...); err != nil {
			return err
		}
		if v.ID == 0 {
			t.Skipf("npc_text id 0 is not a valid gossip text")
			return nil
		}
		if _, dup := dest.NpcTexts[v.ID]; dup {
			t.Skipf("npc_text has duplicate id %d", v.ID)
			return nil
		}
		dest.NpcTexts[v.ID] = v
		t.Stored(1)
		return nil
	})
}

// pageTextLoader ports ObjectMgr::LoadPageTexts (sPageTextStore).
func init() {
	Register(Loader{
		Name:       "page_text",
		Order:      OrderPageText,
		CountQuery: "SELECT COUNT(*) FROM `page_text`",
		Load:       loadPageTexts,
	})
}

func loadPageTexts(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	return scanTable(ctx, q, t,
		"SELECT `entry`,`text`,`next_page` FROM `page_text` ORDER BY `entry`",
		2000, func(r scanner) error {
			var v entity.PageText
			if err := r.Scan(u32(&v.Entry), str(&v.Text), u32(&v.NextPage)); err != nil {
				return err
			}
			if v.Entry == 0 {
				t.Skipf("page_text entry 0 is not a valid page")
				return nil
			}
			if _, dup := dest.PageTexts[v.Entry]; dup {
				t.Skipf("page_text has duplicate entry %d", v.Entry)
				return nil
			}
			dest.PageTexts[v.Entry] = v
			t.Stored(1)
			return nil
		})
}
