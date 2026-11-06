package inventory

import (
	"testing"

	"wowserver/internal/domain/character"
	"wowserver/internal/entity"
)

func protos() ProtoFunc {
	m := map[uint32]entity.ItemPrototype{
		1: {ItemID: 1, Class: classArmor, SubClass: 2, InventoryType: invChest, Stackable: 1, MaxCount: 0, AllowableClass: -1, AllowableRace: -1},
		2: {ItemID: 2, Class: classWeapon, SubClass: 8, InventoryType: inv2HWeapon, Stackable: 1, AllowableClass: -1, AllowableRace: -1},
		3: {ItemID: 3, Class: classArmor, SubClass: 6, InventoryType: invShield, Stackable: 1, AllowableClass: -1, AllowableRace: -1},
		4: {ItemID: 4, Class: 0, InventoryType: 0, Stackable: 20, AllowableClass: -1, AllowableRace: -1}, // potion stack
		5: {ItemID: 5, Class: classContainer, InventoryType: invBag, Stackable: 1, ContainerSlots: 6, AllowableClass: -1, AllowableRace: -1},
		6: {ItemID: 6, Class: classArmor, SubClass: 2, InventoryType: invChest, Stackable: 1, RequiredLevel: 40, AllowableClass: -1, AllowableRace: -1},
		7: {ItemID: 7, Class: 0, Stackable: 1, Flags: flagIndestructible, AllowableClass: -1, AllowableRace: -1},
	}
	return func(e uint32) (entity.ItemPrototype, bool) { p, ok := m[e]; return p, ok }
}

func newInv(items ...character.PlayerItem) (*Inventory, *uint32) {
	next := uint32(1000)
	st := PlayerState{Level: 10, Race: 1, Class: 1, Alive: true}
	st.SkillValue = func(uint32) uint16 { return 300 }
	return New(items, 1, protos(), st, func() uint32 { next++; return next }), &next
}

func it(guid, entry uint32, bag uint32, slot uint8, count uint32) character.PlayerItem {
	p := character.PlayerItem{GUID: guid, Entry: entry, Bag: bag, Slot: slot}
	p.Fields[0], p.Fields[1], p.Fields[3], p.Fields[14] = guid, guidHighItem, entry, count
	return p
}

func kinds(ev []Event) []EventKind {
	var k []EventKind
	for _, e := range ev {
		k = append(k, e.Kind)
	}
	return k
}

func TestAutoEquipFromBackpack(t *testing.T) {
	inv, _ := newInv(it(10, 1, 0, 23, 1))
	f, ev := inv.AutoEquipItem(Bag0, 23)
	if f.Code != ErrOK {
		t.Fatalf("code %d", f.Code)
	}
	if inv.ItemByPos(Bag0, 4) == nil || inv.ItemByPos(Bag0, 23) != nil {
		t.Fatal("item not moved to chest slot")
	}
	// reference order: remove(src) -> create(update), then equip: InvSlot, Visible, create
	want := []EventKind{EvInvSlot, EvItemCreate, EvInvSlot, EvVisible, EvItemCreate}
	got := kinds(ev)
	if len(got) != len(want) {
		t.Fatalf("events %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d: %v want %v", i, got, want)
		}
	}
}

func TestEquipLevelTooLow(t *testing.T) {
	inv, _ := newInv(it(10, 6, 0, 23, 1))
	f, ev := inv.AutoEquipItem(Bag0, 23)
	if f.Code != ErrCantEquipLevel || f.Item1 != 10 || f.RequiredLevel != 40 || len(ev) != 0 {
		t.Fatalf("%+v", f)
	}
}

func TestTwoHandedMovesOffhand(t *testing.T) {
	inv, _ := newInv(it(10, 3, 0, 16, 1), it(11, 2, 0, 23, 1))
	inv.st.CanDualWield = false
	f, _ := inv.AutoEquipItem(Bag0, 23)
	if f.Code != ErrOK {
		t.Fatalf("code %d", f.Code)
	}
	if m := inv.ItemByPos(Bag0, EquipMainhand); m == nil || m.GUID != 11 {
		t.Fatal("2h not equipped")
	}
	if inv.ItemByPos(Bag0, EquipOffhand) != nil {
		t.Fatal("offhand not unequipped")
	}
	if s := inv.ItemByGUID(10); s == nil || s.Bag != Bag0 || s.Slot < ItemStart {
		t.Fatalf("shield should be in the backpack: %+v", s)
	}
}

func TestSwapAndStackMerge(t *testing.T) {
	inv, _ := newInv(it(10, 4, 0, 23, 15), it(11, 4, 0, 24, 10), it(12, 1, 0, 25, 1))
	f, _ := inv.SwapInvItem(24, 23)
	if f.Code != ErrOK {
		t.Fatal(f.Code)
	}
	if a, b := inv.ItemByPos(Bag0, 23), inv.ItemByPos(Bag0, 24); a.Count() != 20 || b.Count() != 5 {
		t.Fatalf("merge: %d %d", a.Count(), b.Count())
	}
	f, _ = inv.SwapInvItem(23, 25)
	if f.Code != ErrOK || inv.ItemByPos(Bag0, 25).Entry != 4 || inv.ItemByPos(Bag0, 23).Entry != 1 {
		t.Fatalf("plain swap %+v", f)
	}
}

func TestSplitAndDestroy(t *testing.T) {
	inv, _ := newInv(it(10, 4, 0, 23, 10))
	f, ev := inv.SplitItem(Bag0, 23, Bag0, 24, 4)
	if f.Code != ErrOK || inv.ItemByPos(Bag0, 23).Count() != 6 || inv.ItemByPos(Bag0, 24).Count() != 4 {
		t.Fatalf("split %+v", f)
	}
	if len(ev) == 0 {
		t.Fatal("no events")
	}
	if f, _ = inv.SplitItem(Bag0, 23, Bag0, 25, 6); f.Code != ErrCouldntSplitItems {
		t.Fatalf("%d", f.Code)
	}
	if f, _ = inv.SplitItem(Bag0, 23, Bag0, 25, 7); f.Code != ErrTriedToSplitMoreThanCount {
		t.Fatalf("%d", f.Code)
	}
	f, _ = inv.DestroyItem(Bag0, 23, 2)
	if f.Code != ErrOK || inv.ItemByPos(Bag0, 23).Count() != 4 {
		t.Fatal("partial destroy")
	}
	_, ev = inv.DestroyItem(Bag0, 23, 0)
	if inv.ItemByPos(Bag0, 23) != nil || len(ev) == 0 {
		t.Fatal("full destroy")
	}
}

func TestDestroyIndestructible(t *testing.T) {
	inv, _ := newInv(it(10, 7, 0, 23, 1))
	if f, _ := inv.DestroyItem(Bag0, 23, 0); f.Code != ErrCantDropSoulbound {
		t.Fatalf("%d", f.Code)
	}
}

func TestStoreIntoBagAndBagSwapKeepsContents(t *testing.T) {
	inv, _ := newInv(it(20, 5, 0, 19, 1), it(21, 5, 0, 20, 1), it(30, 4, 20, 0, 3))
	// the potion lives in the bag equipped at slot 19 (guid 20)
	if p := inv.ItemByPos(19, 0); p == nil || p.GUID != 30 {
		t.Fatal("bag content not indexed by bag slot")
	}
	f, _ := inv.SwapInvItem(19, 20) // swap the two equipped bags
	if f.Code != ErrOK {
		t.Fatalf("code %d", f.Code)
	}
	if p := inv.ItemByPos(20, 0); p == nil || p.GUID != 30 {
		t.Fatalf("contents must follow the bag: %+v", inv.pos)
	}
	var dest []PosCount
	if r := inv.CanStoreNewItem(NullBag, NullSlot, &dest, 1, 1); r != ErrOK || len(dest) != 1 {
		t.Fatal("store new")
	}
	got := inv.Items()
	for _, x := range got {
		if x.GUID == 30 && x.Bag != 20 {
			t.Fatalf("persist form lost bag link: %+v", x)
		}
	}
}

func TestInventoryFull(t *testing.T) {
	var items []character.PlayerItem
	for s := uint8(23); s < 39; s++ {
		items = append(items, it(uint32(s), 1, 0, s, 1))
	}
	inv, _ := newInv(items...)
	var dest []PosCount
	if r := inv.CanStoreNewItem(NullBag, NullSlot, &dest, 1, 1); r != ErrInventoryFull {
		t.Fatalf("%d", r)
	}
}
