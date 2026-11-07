package inventory

import "wowserver/internal/entity"

const (
	bindWhenPickedUp = 1
	bindWhenEquipped = 2
	bindQuestItem    = 4
	guidHighBag      = 0x40000000
)

func (inv *Inventory) takeEvents() []Event {
	e := inv.ev
	inv.ev = nil
	return e
}

func (inv *Inventory) fail(code Result, a, b *Item) Failure {
	f := Failure{Code: code}
	if a != nil {
		f.Item1 = a.GUID
		f.RequiredLevel = a.proto.RequiredLevel
	}
	if b != nil {
		f.Item2 = b.GUID
	}
	return f
}

// FailureOf builds a failure for a bare code (no items).
func FailureOf(code Result) Failure { return Failure{Code: code} }

func (it *Item) setBinding() { it.Fields[fFlags] |= dynBinded }

func (inv *Inventory) setContained(it *Item, bag *Item) {
	if bag != nil {
		it.Fields[fContained], it.Fields[fContained+1] = bag.GUID, guidHighBag
	} else {
		it.Fields[fContained], it.Fields[fContained+1] = inv.playerGUID, 0
	}
	it.Fields[fOwner], it.Fields[fOwner+1] = inv.playerGUID, 0
}

// cloneItem ports Item::CloneItem(count).
func (inv *Inventory) cloneItem(src *Item, count uint32) *Item {
	n := &Item{GUID: inv.newGUID(), Entry: src.Entry, proto: src.proto, Fields: src.Fields, Bag: Bag0, Slot: NullSlot}
	n.Fields[0], n.Fields[1] = n.GUID, guidHighItem
	n.setCount(count)
	return n
}

// RemoveItem ports Player::RemoveItem(bag, slot, update).
func (inv *Inventory) removeItem(bag, slot uint8, update bool) {
	it := inv.ItemByPos(bag, slot)
	if it == nil {
		return
	}
	if bag == Bag0 {
		if slot < BagEnd {
			inv.hooks.ApplyItemMods(it, slot, false)
			if slot < EquipmentEnd {
				inv.hooks.RemoveItemDependentAuras(it)
			}
		}
		inv.detachBag(it)
		delete(inv.pos, Pos(bag, slot))
		inv.emit(Event{Kind: EvInvSlot, Slot: slot, Value: 0})
		if slot < EquipmentEnd {
			inv.emit(Event{Kind: EvVisible, Slot: slot, Value: 0})
		}
	} else if b := inv.ItemByPos(Bag0, bag); b != nil {
		delete(inv.pos, Pos(bag, slot))
		inv.emit(Event{Kind: EvBagSlot, GUID: b.GUID, Slot: slot, Value: 0})
	}
	it.Fields[fContained], it.Fields[fContained+1] = 0, 0
	it.Slot = NullSlot
	it.Bag = Bag0
	if update {
		inv.snapCreate(it)
	}
}

func bindsOnStore(p entity.ItemPrototype, pos uint16) bool {
	return p.Bonding == bindWhenPickedUp || p.Bonding == bindQuestItem ||
		(p.Bonding == bindWhenEquipped && IsBagPos(pos))
}

// StoreItem ports Player::StoreItem(dest, pItem, update).
func (inv *Inventory) storeItem(dest []PosCount, it *Item, update bool) *Item {
	if it == nil {
		return nil
	}
	last := it
	for i := 0; i < len(dest); i++ {
		d := dest[i]
		if i == len(dest)-1 {
			last = inv.storeItemAt(d.Pos, it, d.Count, false, update)
			break
		}
		last = inv.storeItemAt(d.Pos, it, d.Count, true, update)
	}
	return last
}

// _StoreItem ports Player::_StoreItem.
func (inv *Inventory) storeItemAt(pos uint16, it *Item, count uint32, clone, update bool) *Item {
	bag, slot := posParts(pos)
	it2 := inv.ItemByPos(bag, slot)
	if it2 == nil {
		if clone {
			it = inv.cloneItem(it, count)
			inv.guid[it.GUID] = it
		} else {
			it.setCount(count)
		}
		if bindsOnStore(it.proto, pos) {
			it.setBinding()
		}
		if bag == Bag0 {
			it.Bag, it.Slot = Bag0, slot
			inv.pos[pos] = it
			inv.guid[it.GUID] = it
			inv.attachBag(it)
			inv.emit(Event{Kind: EvInvSlot, Slot: slot, Value: it.GUID})
			inv.setContained(it, nil)
			if update {
				inv.snapCreate(it)
			}
		} else if b := inv.ItemByPos(Bag0, bag); b != nil {
			it.Bag, it.Slot = bag, slot
			inv.pos[pos] = it
			inv.guid[it.GUID] = it
			inv.setContained(it, b)
			inv.emit(Event{Kind: EvBagSlot, GUID: b.GUID, Slot: slot, Value: it.GUID})
			if update {
				inv.snapCreate(it)
			}
		}
		return it
	}
	if bindsOnStore(it2.proto, pos) {
		it2.setBinding()
	}
	it2.setCount(it2.Count() + count)
	if update {
		inv.snapCreate(it2)
	}
	if !clone {
		if update {
			inv.emit(Event{Kind: EvItemDestroy, GUID: it.GUID})
		}
		delete(inv.guid, it.GUID)
	}
	return it2
}

// visualizeItem ports Player::VisualizeItem.
func (inv *Inventory) visualizeItem(slot uint8, it *Item) {
	p := it.proto
	if p.Bonding == bindWhenEquipped || p.Bonding == bindWhenPickedUp || p.Bonding == bindQuestItem {
		it.setBinding()
	}
	it.Bag, it.Slot = Bag0, slot
	inv.pos[Pos(Bag0, slot)] = it
	inv.guid[it.GUID] = it
	inv.attachBag(it)
	inv.emit(Event{Kind: EvInvSlot, Slot: slot, Value: it.GUID})
	inv.setContained(it, nil)
	if slot < EquipmentEnd {
		inv.emit(Event{Kind: EvVisible, Slot: slot, Value: it.Entry, GUID: it.GUID})
	}
}

// EquipItem ports Player::EquipItem(pos, pItem, update).
func (inv *Inventory) equipItem(pos uint16, it *Item, update bool) *Item {
	bag, slot := posParts(pos)
	it2 := inv.ItemByPos(bag, slot)
	if it2 == nil {
		inv.visualizeItem(slot, it)
		if inv.st.Alive {
			inv.hooks.ApplyItemMods(it, slot, true)
		}
		if update {
			inv.snapCreate(it)
		}
		return it
	}
	it2.setCount(it2.Count() + it.Count())
	if update {
		inv.snapCreate(it2)
		inv.emit(Event{Kind: EvItemDestroy, GUID: it.GUID})
	}
	delete(inv.guid, it.GUID)
	return it2
}

// DestroyItem ports Player::DestroyItem(bag, slot, update).
func (inv *Inventory) destroyItem(bag, slot uint8, update bool) {
	it := inv.ItemByPos(bag, slot)
	if it == nil {
		return
	}
	if it.IsBag() && bag == Bag0 && slot >= BagStart && slot < BagEnd {
		for i := 0; i < maxBagSize; i++ {
			inv.destroyItem(slot, uint8(i), update)
		}
	}
	if bag == Bag0 {
		inv.emit(Event{Kind: EvInvSlot, Slot: slot, Value: 0})
		if slot < BagEnd {
			inv.hooks.ApplyItemMods(it, slot, false)
		}
		if slot < EquipmentEnd {
			inv.hooks.RemoveItemDependentAuras(it)
			inv.emit(Event{Kind: EvVisible, Slot: slot, Value: 0})
		}
		delete(inv.pos, Pos(bag, slot))
	} else if b := inv.ItemByPos(Bag0, bag); b != nil {
		delete(inv.pos, Pos(bag, slot))
		inv.emit(Event{Kind: EvBagSlot, GUID: b.GUID, Slot: slot, Value: 0})
	}
	if update {
		inv.emit(Event{Kind: EvItemDestroy, GUID: it.GUID})
	}
	delete(inv.guid, it.GUID)
	it.Fields[fContained], it.Fields[fContained+1] = 0, 0
	it.Slot = NullSlot
}

// DestroyItemCount ports Player::DestroyItemCount(Item*, uint32& count, update).
func (inv *Inventory) destroyItemCount(it *Item, count *uint32, update bool) {
	if it == nil {
		return
	}
	if it.Count() <= *count {
		*count -= it.Count()
		inv.destroyItem(it.Bag, it.Slot, update)
		return
	}
	it.setCount(it.Count() - *count)
	*count = 0
	if update {
		inv.snapCreate(it)
	}
}

// autoUnequipOffhandIfNeed ports Player::AutoUnequipOffhandIfNeed.
func (inv *Inventory) autoUnequipOffhandIfNeed() {
	off := inv.ItemByPos(Bag0, EquipOffhand)
	if off == nil {
		return
	}
	if (inv.st.CanDualWield || off.proto.InventoryType == invShield || off.proto.InventoryType == invHoldable) && !inv.isTwoHandUsed() {
		return
	}
	var dest []PosCount
	if inv.CanStoreItem(NullBag, NullSlot, &dest, off, false) == ErrOK {
		inv.removeItem(Bag0, EquipOffhand, true)
		inv.storeItem(dest, off, true)
	} else {
		// MoveItemFromInventory + mail: the item is taken out of the
		// inventory (mailing is not modelled, it is removed).
		inv.removeItem(Bag0, EquipOffhand, true)
		delete(inv.guid, off.GUID)
	}
}

// AutoUnequipOffhandIfNeed exposes the off-hand fix-up for callers that
// change main-hand state themselves.
func (inv *Inventory) AutoUnequipOffhandIfNeed() []Event {
	inv.autoUnequipOffhandIfNeed()
	return inv.takeEvents()
}

// StoreNewItem ports Player::StoreNewItem(dest, item, update): creates the
// item (Item::CreateItem) and stores it.
func (inv *Inventory) StoreNewItem(dest []PosCount, entry uint32, update bool) (*Item, []Event) {
	var count uint32
	for _, d := range dest {
		count += d.Count
	}
	p, ok := inv.proto(entry)
	if !ok {
		return nil, nil
	}
	it := &Item{GUID: inv.newGUID(), Entry: entry, proto: p, Bag: Bag0, Slot: NullSlot}
	it.Fields[0], it.Fields[1], it.Fields[2], it.Fields[3] = it.GUID, guidHighItem, 3 /* TYPEMASK_OBJECT|ITEM */, entry
	it.Fields[4] = 0x3F800000 // OBJECT_FIELD_SCALE_X = 1.0
	it.setCount(count)
	it.Fields[58], it.Fields[59] = p.MaxDurability, p.MaxDurability
	for i := 0; i < 5; i++ {
		it.Fields[16+i] = 0
	}
	last := inv.storeItem(dest, it, update)
	return last, inv.takeEvents()
}

// ---- opcode handlers (ItemHandler.cpp) ----

// SplitItem ports Player::SplitItem via HandleSplitItemOpcode.
func (inv *Inventory) SplitItem(srcBag, srcSlot, dstBag, dstSlot uint8, count uint32) (Failure, []Event) {
	src, dst := Pos(srcBag, srcSlot), Pos(dstBag, dstSlot)
	if src == dst || count == 0 {
		return Failure{}, nil
	}
	if !inv.IsValidPos(srcBag, srcSlot, true) {
		return FailureOf(ErrItemNotFound), nil
	}
	if !inv.IsValidPos(dstBag, dstSlot, false) {
		return FailureOf(ErrItemDoesntGoToSlot), nil
	}
	s := inv.ItemByPos(srcBag, srcSlot)
	if s == nil {
		return inv.fail(ErrItemNotFound, nil, nil), nil
	}
	if s.Count() == count {
		return inv.fail(ErrCouldntSplitItems, s, nil), nil
	}
	if s.Count() < count {
		return inv.fail(ErrTriedToSplitMoreThanCount, s, nil), nil
	}
	n := inv.cloneItem(s, count)
	if n.GUID == 0 { // item guid could not be allocated
		return inv.fail(ErrItemNotFound, s, nil), nil
	}
	switch {
	case IsInventoryPos(dstBag, dstSlot):
		s.setCount(s.Count() - count)
		var dest []PosCount
		if msg := inv.CanStoreItem(dstBag, dstSlot, &dest, n, false); msg != ErrOK {
			s.setCount(s.Count() + count)
			return inv.fail(msg, s, nil), nil
		}
		inv.snapCreate(s)
		inv.storeItem(dest, n, true)
	case IsBankPos(dstBag, dstSlot):
		return inv.fail(ErrTooFarAwayFromBank, s, nil), nil
	case IsEquipmentPos(dstBag, dstSlot):
		s.setCount(s.Count() - count)
		msg, dpos := inv.CanEquipItem(dstSlot, n, false, true)
		if msg != ErrOK {
			s.setCount(s.Count() + count)
			return inv.fail(msg, s, nil), nil
		}
		inv.snapCreate(s)
		inv.equipItem(dpos, n, true)
		inv.autoUnequipOffhandIfNeed()
	}
	return Failure{}, inv.takeEvents()
}

// SwapInvItem ports HandleSwapInvItemOpcode.
func (inv *Inventory) SwapInvItem(srcSlot, dstSlot uint8) (Failure, []Event) {
	if srcSlot == dstSlot {
		return Failure{}, nil
	}
	if !inv.IsValidPos(Bag0, srcSlot, true) {
		return FailureOf(ErrItemNotFound), nil
	}
	if !inv.IsValidPos(Bag0, dstSlot, true) {
		return FailureOf(ErrItemDoesntGoToSlot), nil
	}
	return inv.swapItem(Pos(Bag0, srcSlot), Pos(Bag0, dstSlot))
}

// SwapItemPos ports HandleSwapItem (CMSG_SWAP_ITEM).
func (inv *Inventory) SwapItemPos(dstBag, dstSlot, srcBag, srcSlot uint8) (Failure, []Event) {
	src, dst := Pos(srcBag, srcSlot), Pos(dstBag, dstSlot)
	if src == dst {
		return Failure{}, nil
	}
	if !inv.IsValidPos(srcBag, srcSlot, true) {
		return FailureOf(ErrItemNotFound), nil
	}
	if !inv.IsValidPos(dstBag, dstSlot, true) {
		return FailureOf(ErrItemDoesntGoToSlot), nil
	}
	return inv.swapItem(src, dst)
}

// AutoEquipItemSlot ports HandleAutoEquipItemSlotOpcode.
func (inv *Inventory) AutoEquipItemSlot(itemGUID uint32, dstSlot uint8) (Failure, []Event) {
	if !IsEquipmentPos(Bag0, dstSlot) {
		return Failure{}, nil
	}
	it := inv.ItemByGUID(itemGUID)
	dst := Pos(Bag0, dstSlot)
	if it == nil || it.Pos() == dst {
		return Failure{}, nil
	}
	return inv.swapItem(it.Pos(), dst)
}

// AutoEquipItem ports HandleAutoEquipItemOpcode.
func (inv *Inventory) AutoEquipItem(srcBag, srcSlot uint8) (Failure, []Event) {
	src := inv.ItemByPos(srcBag, srcSlot)
	if src == nil {
		return Failure{}, nil
	}
	msg, dest := inv.CanEquipItem(NullSlot, src, !src.IsBag(), true)
	if msg != ErrOK {
		return inv.fail(msg, src, nil), nil
	}
	srcPos := src.Pos()
	if dest == srcPos {
		return Failure{}, nil
	}
	dbag, dslot := posParts(dest)
	dstItem := inv.ItemByPos(dbag, dslot)
	if dstItem == nil {
		inv.removeItem(srcBag, srcSlot, true)
		inv.equipItem(dest, src, true)
		inv.autoUnequipOffhandIfNeed()
		return Failure{}, inv.takeEvents()
	}
	dstBag, dstSlot := dstItem.Bag, dstItem.Slot
	if msg = inv.CanUnequipItem(dest, !src.IsBag()); msg != ErrOK {
		return inv.fail(msg, dstItem, nil), nil
	}
	var sSrc []PosCount
	var eSrc uint16
	switch {
	case IsInventoryPos(srcBag, srcSlot):
		msg = inv.CanStoreItem(srcBag, srcSlot, &sSrc, dstItem, true)
		if msg != ErrOK {
			sSrc = nil
			msg = inv.CanStoreItem(srcBag, NullSlot, &sSrc, dstItem, true)
		}
		if msg != ErrOK {
			sSrc = nil
			msg = inv.CanStoreItem(NullBag, NullSlot, &sSrc, dstItem, true)
		}
	case IsBankPos(srcBag, srcSlot):
		msg = ErrTooFarAwayFromBank
	case IsEquipmentPos(srcBag, srcSlot):
		msg, eSrc = inv.CanEquipItem(srcSlot, dstItem, true, true)
		if msg == ErrOK {
			msg = inv.CanUnequipItem(eSrc, true)
		}
	}
	if msg != ErrOK {
		return inv.fail(msg, dstItem, src), nil
	}
	inv.removeItem(dstBag, dstSlot, false)
	inv.removeItem(srcBag, srcSlot, false)
	inv.equipItem(dest, src, true)
	switch {
	case IsInventoryPos(srcBag, srcSlot):
		inv.storeItem(sSrc, dstItem, true)
	case IsEquipmentPos(srcBag, srcSlot):
		inv.equipItem(eSrc, dstItem, true)
	}
	inv.autoUnequipOffhandIfNeed()
	return Failure{}, inv.takeEvents()
}

// DestroyItem ports HandleDestroyItemOpcode (count 0 = whole stack).
func (inv *Inventory) DestroyItem(bag, slot uint8, count uint32) (Failure, []Event) {
	pos := Pos(bag, slot)
	if IsEquipmentPos(bag, slot) || IsBagPos(pos) {
		if msg := inv.CanUnequipItem(pos, false); msg != ErrOK {
			return inv.fail(msg, inv.ItemByPos(bag, slot), nil), nil
		}
	}
	it := inv.ItemByPos(bag, slot)
	if it == nil {
		return FailureOf(ErrItemNotFound), nil
	}
	if it.proto.Flags&flagIndestructible != 0 {
		return FailureOf(ErrCantDropSoulbound), nil
	}
	if count != 0 {
		c := count
		inv.destroyItemCount(it, &c, true)
	} else {
		inv.destroyItem(bag, slot, true)
	}
	return Failure{}, inv.takeEvents()
}

// SwapItem ports Player::SwapItem(src, dst).
func (inv *Inventory) swapItem(src, dst uint16) (Failure, []Event) {
	srcBag, srcSlot := posParts(src)
	dstBag, dstSlot := posParts(dst)
	s := inv.ItemByPos(srcBag, srcSlot)
	d := inv.ItemByPos(dstBag, dstSlot)
	if s == nil {
		return Failure{}, nil
	}
	if !inv.st.Alive {
		return inv.fail(ErrYouAreDead, s, d), nil
	}
	if IsEquipmentPos(srcBag, srcSlot) || IsBagPos(src) {
		swap := !IsBagPos(src) || IsBagPos(dst) || (d != nil && d.IsBag() && inv.bagIsEmpty(d))
		if msg := inv.CanUnequipItem(src, swap); msg != ErrOK {
			return inv.fail(msg, s, d), nil
		}
	}
	if IsBagPos(src) && srcSlot == dstBag {
		return inv.fail(ErrNonemptyBagOverOtherBag, s, d), nil
	}
	if IsBagPos(dst) && dstSlot == srcBag {
		return inv.fail(ErrNonemptyBagOverOtherBag, d, s), nil
	}
	if d != nil && (IsEquipmentPos(dstBag, dstSlot) || IsBagPos(dst)) {
		swap := !IsBagPos(dst) || IsBagPos(src) || (s.IsBag() && inv.bagIsEmpty(s))
		if msg := inv.CanUnequipItem(dst, swap); msg != ErrOK {
			return inv.fail(msg, s, d), nil
		}
	}
	if d == nil {
		switch {
		case IsInventoryPos(dstBag, dstSlot):
			var dest []PosCount
			if msg := inv.CanStoreItem(dstBag, dstSlot, &dest, s, false); msg != ErrOK {
				return inv.fail(msg, s, nil), nil
			}
			inv.removeItem(srcBag, srcSlot, true)
			inv.storeItem(dest, s, true)
		case IsBankPos(dstBag, dstSlot):
			return inv.fail(ErrTooFarAwayFromBank, s, nil), nil
		case IsEquipmentPos(dstBag, dstSlot):
			msg, dpos := inv.CanEquipItem(dstSlot, s, false, true)
			if msg != ErrOK {
				return inv.fail(msg, s, nil), nil
			}
			inv.removeItem(srcBag, srcSlot, true)
			inv.equipItem(dpos, s, true)
			inv.autoUnequipOffhandIfNeed()
		}
		return Failure{}, inv.takeEvents()
	}
	// stack merge / simple swap of non-bags
	if !s.IsBag() && !d.IsBag() {
		var msg Result
		var sDest []PosCount
		var eDest uint16
		switch {
		case IsInventoryPos(dstBag, dstSlot):
			msg = inv.CanStoreItem(dstBag, dstSlot, &sDest, s, false)
		case IsBankPos(dstBag, dstSlot):
			msg = ErrTooFarAwayFromBank
		case IsEquipmentPos(dstBag, dstSlot):
			msg, eDest = inv.CanEquipItem(dstSlot, s, false, true)
		default:
			return Failure{}, nil
		}
		if msg == ErrOK {
			ms := maxStack(s.proto)
			if s.Count()+d.Count() <= ms {
				inv.removeItem(srcBag, srcSlot, true)
				switch {
				case IsInventoryPos(dstBag, dstSlot):
					inv.storeItem(sDest, s, true)
				case IsEquipmentPos(dstBag, dstSlot):
					inv.equipItem(eDest, s, true)
					inv.autoUnequipOffhandIfNeed()
				}
			} else {
				s.setCount(s.Count() + d.Count() - ms)
				d.setCount(ms)
				inv.snapCreate(s)
				inv.snapCreate(d)
			}
			return Failure{}, inv.takeEvents()
		}
	}
	// real swap
	msg := Result(10) // never used default
	var sDest []PosCount
	var eDest uint16
	switch {
	case IsInventoryPos(dstBag, dstSlot):
		msg = inv.CanStoreItem(dstBag, dstSlot, &sDest, s, true)
	case IsBankPos(dstBag, dstSlot):
		msg = ErrTooFarAwayFromBank
	case IsEquipmentPos(dstBag, dstSlot):
		msg, eDest = inv.CanEquipItem(dstSlot, s, true, true)
		if msg == ErrOK {
			msg = inv.CanUnequipItem(eDest, true)
		}
	}
	if msg != ErrOK {
		return inv.fail(msg, s, d), nil
	}
	var sDest2 []PosCount
	var eDest2 uint16
	switch {
	case IsInventoryPos(srcBag, srcSlot):
		msg = inv.CanStoreItem(srcBag, srcSlot, &sDest2, d, true)
	case IsBankPos(srcBag, srcSlot):
		msg = ErrTooFarAwayFromBank
	case IsEquipmentPos(srcBag, srcSlot):
		msg, eDest2 = inv.CanEquipItem(srcSlot, d, true, true)
		if msg == ErrOK {
			msg = inv.CanUnequipItem(eDest2, true)
		}
	}
	if msg != ErrOK {
		return inv.fail(msg, d, s), nil
	}
	if s.IsBag() && d.IsBag() {
		var emptyBag, fullBag *Item
		if inv.bagIsEmpty(s) && !IsBagPos(src) {
			emptyBag, fullBag = s, d
		} else if inv.bagIsEmpty(d) && !IsBagPos(dst) {
			emptyBag, fullBag = d, s
		}
		if emptyBag != nil && fullBag != nil {
			var count uint32
			for i := uint32(0); i < fullBag.BagSize(); i++ {
				bi := inv.ItemByPos(fullBag.Slot, uint8(i))
				if bi == nil {
					continue
				}
				if !itemCanGoIntoBag(bi.proto, emptyBag.proto) {
					return inv.fail(ErrNonemptyBagOverOtherBag, s, d), nil
				}
				count++
			}
			if count > emptyBag.BagSize() {
				return inv.fail(ErrItemsCantBeSwapped, s, d), nil
			}
			count = 0
			for i := uint32(0); i < fullBag.BagSize(); i++ {
				bi := inv.ItemByPos(fullBag.Slot, uint8(i))
				if bi == nil {
					continue
				}
				delete(inv.pos, Pos(fullBag.Slot, uint8(i)))
				bi.Bag, bi.Slot = emptyBag.Slot, uint8(count)
				inv.pos[bi.Pos()] = bi
				inv.setContained(bi, emptyBag)
				count++
			}
		}
	}
	inv.removeItem(dstBag, dstSlot, false)
	inv.removeItem(srcBag, srcSlot, false)
	switch {
	case IsInventoryPos(dstBag, dstSlot):
		inv.storeItem(sDest, s, true)
	case IsEquipmentPos(dstBag, dstSlot):
		inv.equipItem(eDest, s, true)
	}
	switch {
	case IsInventoryPos(srcBag, srcSlot):
		inv.storeItem(sDest2, d, true)
	case IsEquipmentPos(srcBag, srcSlot):
		inv.equipItem(eDest2, d, true)
	}
	inv.autoUnequipOffhandIfNeed()
	return Failure{}, inv.takeEvents()
}

// ConsumeItem ports Player::DestroyItemCount(Item*, count, true) as used by
// Spell::TakeCastItem: the stack shrinks by count and the item is destroyed
// when it runs out. No CanDestroy rules apply.
func (inv *Inventory) ConsumeItem(bag, slot uint8, count uint32) []Event {
	it := inv.ItemByPos(bag, slot)
	if it == nil {
		return nil
	}
	c := count
	inv.destroyItemCount(it, &c, true)
	return inv.takeEvents()
}
