package worldload

import (
	"context"
	"database/sql/driver"
	"testing"
)

func TestNpcLoaders(t *testing.T) {
	ResetForTest()
	db, srv := newFakeDB(t)

	// item_template for vendor validation.
	srv.put("SELECT COUNT(*) FROM `item_template`", rows1(1))
	itTotal := 135
	itCols := make([]string, itTotal)
	itRow := make([]driver.Value, itTotal)
	for i := range itCols {
		itCols[i] = "c"
	}
	itRow[0] = int64(6948)
	srv.put(itemTemplateQuery(), &fakeRows{cols: itCols, data: [][]driver.Value{itRow}})

	srv.put("SELECT COUNT(*) FROM `npc_vendor`", rows1(2))
	srv.put("SELECT `entry`,`item`,`maxcount`,`incrtime`,`ExtendedCost`,`condition_id` FROM `npc_vendor`",
		&fakeRows{cols: []string{"entry", "item", "maxcount", "incrtime", "ExtendedCost", "condition_id"},
			data: [][]driver.Value{
				{int64(358), int64(6948), int64(0), int64(0), int64(0), int64(0)},
				{int64(358), int64(999), int64(1), int64(7200), int64(0), int64(0)}, // item missing -> skipped
			}})
	srv.put("SELECT COUNT(*) FROM `npc_vendor_template`", rows1(1))
	srv.put("SELECT `entry`,`item`,`maxcount`,`incrtime`,`ExtendedCost`,`condition_id` FROM `npc_vendor_template`",
		&fakeRows{cols: []string{"entry", "item", "maxcount", "incrtime", "ExtendedCost", "condition_id"},
			data: [][]driver.Value{{int64(4), int64(6948), int64(0), int64(0), int64(2), int64(0)}}})

	srv.put("SELECT COUNT(*) FROM `npc_trainer`", rows1(1))
	srv.put("SELECT `entry`,`spell`,`spellcost`,`reqskill`,`reqskillvalue`,`reqlevel` FROM `npc_trainer`",
		&fakeRows{cols: []string{"entry", "spell", "spellcost", "reqskill", "reqskillvalue", "reqlevel"},
			data: [][]driver.Value{{int64(5), int64(688), int64(10), int64(0), int64(0), int64(-1)}}})
	srv.put("SELECT COUNT(*) FROM `npc_trainer_template`", rows1(1))
	srv.put("SELECT `entry`,`spell`,`spellcost`,`reqskill`,`reqskillvalue`,`reqlevel` FROM `npc_trainer_template`",
		&fakeRows{cols: []string{"entry", "spell", "spellcost", "reqskill", "reqskillvalue", "reqlevel"},
			data: [][]driver.Value{{int64(8), int64(689), int64(100), int64(164), int64(50), int64(20)}}})

	srv.put("SELECT COUNT(*) FROM `gossip_menu`", rows1(1))
	srv.put("SELECT `entry`,`text_id`,`script_id`,`condition_id` FROM `gossip_menu`",
		&fakeRows{cols: []string{"entry", "text_id", "script_id", "condition_id"},
			data: [][]driver.Value{{int64(75), int64(300), int64(0), int64(0)}}})
	srv.put("SELECT COUNT(*) FROM `gossip_menu_option`", rows1(1))
	srv.put("SELECT `menu_id`,`id`,`option_icon`,`option_text`,`option_id`,`npc_option_npcflag`,`action_menu_id`,`action_poi_id`,`action_script_id`,`box_coded`,`box_money`,`box_text`,`condition_id` FROM `gossip_menu_option` ORDER BY `menu_id`,`id`",
		&fakeRows{cols: []string{"menu_id", "id", "option_icon", "option_text", "option_id", "npc_option_npcflag", "action_menu_id", "action_poi_id", "action_script_id", "box_coded", "box_money", "box_text", "condition_id"},
			data: [][]driver.Value{{int64(75), int64(0), int64(0), "I want to browse your goods.", int64(3), int64(128), int64(0), int64(0), int64(0), int64(0), int64(0), nil, int64(0)}}})

	// npc_text: 1 + 8*(4+6) = 81 columns.
	npcCols := make([]string, 81)
	npcRow := make([]driver.Value, 81)
	for i := range npcCols {
		npcCols[i] = "c"
	}
	npcRow[0] = int64(300)
	npcRow[1] = "Welcome to the Lion's Pride Inn."
	npcRow[4] = 1.0 // prob0 (1..4 = text0_0,text0_1,lang0,prob0)
	srv.put("SELECT COUNT(*) FROM `npc_text`", rows1(1))
	srv.put(npcTextQuery(), &fakeRows{cols: npcCols, data: [][]driver.Value{npcRow}})

	srv.put("SELECT COUNT(*) FROM `page_text`", rows1(1))
	srv.put("SELECT `entry`,`text`,`next_page` FROM `page_text` ORDER BY `entry`",
		&fakeRows{cols: []string{"entry", "text", "next_page"},
			data: [][]driver.Value{{int64(1), "A well-worn journal.", int64(2)}}})

	Register(Loader{Name: "item_template", Order: OrderItemTemplate, CountQuery: "SELECT COUNT(*) FROM `item_template`", Load: loadItemPrototypes})
	Register(Loader{Name: "npc_vendor", Order: OrderVendors, CountQuery: "SELECT COUNT(*) FROM `npc_vendor`", Load: func(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
		return scanVendors(ctx, q, t, dest, "npc_vendor", dest.NpcVendors)
	}})
	Register(Loader{Name: "npc_vendor_template", Order: OrderVendorTemplates, CountQuery: "SELECT COUNT(*) FROM `npc_vendor_template`", Load: func(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
		return scanVendors(ctx, q, t, dest, "npc_vendor_template", dest.NpcVendorTemplates)
	}})
	Register(Loader{Name: "npc_trainer", Order: OrderTrainers, CountQuery: "SELECT COUNT(*) FROM `npc_trainer`", Load: func(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
		return scanTrainers(ctx, q, t, dest, "npc_trainer", dest.NpcTrainers)
	}})
	Register(Loader{Name: "npc_trainer_template", Order: OrderTrainerTemplates, CountQuery: "SELECT COUNT(*) FROM `npc_trainer_template`", Load: func(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
		return scanTrainers(ctx, q, t, dest, "npc_trainer_template", dest.NpcTrainerTemplates)
	}})
	Register(Loader{Name: "gossip_menu", Order: OrderGossipMenus, CountQuery: "SELECT COUNT(*) FROM `gossip_menu`", Load: loadGossipMenus})
	Register(Loader{Name: "gossip_menu_option", Order: OrderGossipMenus + 1, CountQuery: "SELECT COUNT(*) FROM `gossip_menu_option`", Load: loadGossipMenuOptions})
	Register(Loader{Name: "npc_text", Order: OrderNpcText, CountQuery: "SELECT COUNT(*) FROM `npc_text`", Load: loadNpcTexts})
	Register(Loader{Name: "page_text", Order: OrderPageText, CountQuery: "SELECT COUNT(*) FROM `page_text`", Load: loadPageTexts})

	dest := newDatasetForTest()
	rep, err := Run(context.Background(), db, dest, RunOptions{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if vs := dest.NpcVendors[358]; len(vs) != 1 || vs[0].Item != 6948 {
		t.Fatalf("vendor items wrong: %+v", vs)
	}
	if vs := dest.NpcVendorTemplates[4]; len(vs) != 1 || vs[0].ExtendedCost != 2 {
		t.Fatalf("vendor template wrong: %+v", vs)
	}
	if ts := dest.NpcTrainers[5]; len(ts) != 1 || ts[0].Spell != 688 || ts[0].ReqLevel != -1 {
		t.Fatalf("trainer wrong: %+v", ts)
	}
	if ts := dest.NpcTrainerTemplates[8]; len(ts) != 1 || ts[0].RequiredSkillValue != 50 {
		t.Fatalf("trainer template wrong: %+v", ts)
	}
	if gm := dest.GossipMenus[75]; len(gm) != 1 || gm[0].TextID != 300 {
		t.Fatalf("gossip menu wrong: %+v", gm)
	}
	if gopt := dest.GossipMenuOptions[75]; len(gopt) != 1 || gopt[0].OptionText != "I want to browse your goods." || gopt[0].NpcOptionNpcflag != 128 {
		t.Fatalf("gossip option wrong: %+v", gopt)
	}
	if nt := dest.NpcTexts[300]; nt.Blocks[0].Text0 != "Welcome to the Lion's Pride Inn." || nt.Blocks[0].Prob != 1.0 {
		t.Fatalf("npc text wrong: %+v", nt.Blocks[0])
	}
	if pt := dest.PageTexts[1]; pt.Text != "A well-worn journal." || pt.NextPage != 2 {
		t.Fatalf("page text wrong: %+v", pt)
	}
	if rep.TotalSkipped() != 1 {
		t.Fatalf("skipped = %d, want 1 (missing vendor item)", rep.TotalSkipped())
	}
}
