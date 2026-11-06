package inventory

import "wowserver/internal/entity"

// PosCount is ItemPosCount: (bag<<8|slot, count).
type PosCount struct {
	Pos   uint16
	Count uint32
}

func containedIn(v []PosCount, p PosCount) bool {
	for _, x := range v {
		if x.Pos == p.Pos {
			return true
		}
	}
	return false
}

// itemCount ports Player::GetItemCount(entry, true, skip).
func (inv *Inventory) itemCount(entry uint32, skip *Item) uint32 {
	var n uint32
	for _, it := range inv.guid {
		if it == skip || it.Entry != entry {
			continue
		}
		bag, slot := it.Bag, it.Slot
		ok := false
		if bag == Bag0 {
			ok = slot < PlayerSlotEnd && !(slot >= BuybackStart && slot < BuybackEnd)
		} else {
			ok = true
		}
		if ok {
			n += it.Count()
		}
	}
	return n
}

// canTakeMoreSimilarItems ports Player::_CanTakeMoreSimilarItems.
func (inv *Inventory) canTakeMoreSimilarItems(entry, count uint32, item *Item) (Result, uint32) {
	p, ok := inv.proto(entry)
	if !ok {
		return ErrCantCarryMoreOfThis, count
	}
	if p.MaxCount > 0 {
		cur := inv.itemCount(p.ItemID, item)
		if cur+count > p.MaxCount {
			over := cur + count - p.MaxCount
			if over > count {
				over = count
			}
			return ErrCantCarryMoreOfThis, over
		}
	}
	return ErrOK, 0
}

// canBeMergedPartlyWith ports Item::CanBeMergedPartlyWith.
func (i *Item) canBeMergedPartlyWith(p entity.ItemPrototype) Result {
	if i.Entry != p.ItemID {
		return ErrItemCantStack
	}
	if i.Count() >= maxStack(p) {
		return ErrItemCantStack
	}
	return ErrOK
}

// canStoreInSpecificSlot ports _CanStoreItem_InSpecificSlot.
func (inv *Inventory) canStoreInSpecificSlot(bag, slot uint8, dest *[]PosCount, p entity.ItemPrototype, count *uint32, swap bool, src *Item) Result {
	item2 := inv.ItemByPos(bag, slot)
	if item2 == src {
		item2 = nil
	}
	var need uint32
	if item2 == nil || swap {
		if bag == Bag0 {
			if slot >= KeyringStart && slot < KeyringEnd && p.BagFamily&bagFamilyKeys == 0 {
				return ErrItemDoesntGoIntoBag
			}
			if (slot >= BuybackStart && slot < BuybackEnd) || slot >= PlayerSlotEnd {
				return ErrItemDoesntGoIntoBag
			}
		} else {
			b := inv.ItemByPos(Bag0, bag)
			if b == nil {
				return ErrItemDoesntGoIntoBag
			}
			if uint32(slot) >= b.BagSize() {
				return ErrItemDoesntGoIntoBag
			}
			if !itemCanGoIntoBag(p, b.proto) {
				return ErrItemDoesntGoIntoBag
			}
		}
		need = uint32(p.Stackable)
		if p.Stackable <= 0 {
			need = 1
		}
	} else {
		if res := item2.canBeMergedPartlyWith(p); res != ErrOK {
			return res
		}
		need = maxStack(p) - item2.Count()
	}
	if need > *count {
		need = *count
	}
	np := PosCount{Pos: Pos(bag, slot), Count: need}
	if !containedIn(*dest, np) {
		*dest = append(*dest, np)
		*count -= need
	}
	return ErrOK
}

// canStoreInBag ports _CanStoreItem_InBag.
func (inv *Inventory) canStoreInBag(bag uint8, dest *[]PosCount, p entity.ItemPrototype, count *uint32, merge, nonSpecialized bool, src *Item, skipBag, skipSlot uint8) Result {
	if bag == skipBag {
		return ErrItemDoesntGoIntoBag
	}
	b := inv.ItemByPos(Bag0, bag)
	if b == nil || b == src {
		return ErrItemDoesntGoIntoBag
	}
	if nonSpecialized != (b.proto.Class == classContainer && b.proto.SubClass == subContainer) {
		return ErrItemDoesntGoIntoBag
	}
	if !itemCanGoIntoBag(p, b.proto) {
		return ErrItemDoesntGoIntoBag
	}
	for j := uint32(0); j < b.BagSize(); j++ {
		if uint8(j) == skipSlot {
			continue
		}
		item2 := inv.ItemByPos(bag, uint8(j))
		if item2 == src {
			item2 = nil
		}
		if (item2 != nil) != merge {
			continue
		}
		need := maxStack(p)
		if item2 != nil {
			if item2.canBeMergedPartlyWith(p) != ErrOK {
				continue
			}
			need -= item2.Count()
		}
		if need > *count {
			need = *count
		}
		np := PosCount{Pos: Pos(bag, uint8(j)), Count: need}
		if !containedIn(*dest, np) {
			*dest = append(*dest, np)
			*count -= need
			if *count == 0 {
				return ErrOK
			}
		}
	}
	return ErrOK
}

// canStoreInInventorySlots ports _CanStoreItem_InInventorySlots.
func (inv *Inventory) canStoreInInventorySlots(begin, end uint8, dest *[]PosCount, p entity.ItemPrototype, count *uint32, merge bool, src *Item, skipBag, skipSlot uint8) Result {
	for j := begin; j < end; j++ {
		if Bag0 == skipBag && j == skipSlot {
			continue
		}
		item2 := inv.ItemByPos(Bag0, j)
		if item2 == src {
			item2 = nil
		}
		if (item2 != nil) != merge {
			continue
		}
		need := maxStack(p)
		if item2 != nil {
			if item2.canBeMergedPartlyWith(p) != ErrOK {
				continue
			}
			need -= item2.Count()
		}
		if need > *count {
			need = *count
		}
		np := PosCount{Pos: Pos(Bag0, j), Count: need}
		if !containedIn(*dest, np) {
			*dest = append(*dest, np)
			*count -= need
			if *count == 0 {
				return ErrOK
			}
		}
	}
	return ErrOK
}

// CanStoreItem ports Player::CanStoreItem(bag, slot, dest, pItem, swap).
func (inv *Inventory) CanStoreItem(bag, slot uint8, dest *[]PosCount, item *Item, swap bool) Result {
	return inv.canStoreItem(bag, slot, dest, item.Entry, item.Count(), item, swap)
}

// CanStoreNewItem ports Player::CanStoreNewItem(bag, slot, dest, entry, count).
func (inv *Inventory) CanStoreNewItem(bag, slot uint8, dest *[]PosCount, entry, count uint32) Result {
	return inv.canStoreItem(bag, slot, dest, entry, count, nil, false)
}

// canStoreItem ports Player::_CanStoreItem.
func (inv *Inventory) canStoreItem(bag, slot uint8, dest *[]PosCount, entry, count uint32, item *Item, swap bool) Result {
	p, ok := inv.proto(entry)
	if !ok {
		if swap {
			return ErrItemsCantBeSwapped
		}
		return ErrItemNotFound
	}
	// pItem->IsBindedNotWith(this): every carried item is owned by this player.
	res, noSimilar := inv.canTakeMoreSimilarItems(entry, count, item)
	if res != ErrOK {
		if count == noSimilar {
			return res
		}
		count -= noSimilar
	}
	done := func() (Result, bool) {
		if count == 0 {
			if noSimilar == 0 {
				return ErrOK, true
			}
			return ErrCantCarryMoreOfThis, true
		}
		return 0, false
	}
	if bag != NullBag && slot != NullSlot {
		if res = inv.canStoreInSpecificSlot(bag, slot, dest, p, &count, swap, item); res != ErrOK {
			return res
		}
		if r, d := done(); d {
			return r
		}
	}
	if bag != NullBag {
		if p.Stackable > 1 {
			if bag == Bag0 {
				if res = inv.canStoreInInventorySlots(KeyringStart, KeyringEnd, dest, p, &count, true, item, bag, slot); res != ErrOK {
					return res
				}
				if r, d := done(); d {
					return r
				}
				if res = inv.canStoreInInventorySlots(ItemStart, ItemEnd, dest, p, &count, true, item, bag, slot); res != ErrOK {
					return res
				}
				if r, d := done(); d {
					return r
				}
			} else {
				res = inv.canStoreInBag(bag, dest, p, &count, true, false, item, NullBag, slot)
				if res != ErrOK {
					res = inv.canStoreInBag(bag, dest, p, &count, true, true, item, NullBag, slot)
				}
				if res != ErrOK {
					return res
				}
				if r, d := done(); d {
					return r
				}
			}
		}
		if bag == Bag0 {
			if p.BagFamily&bagFamilyKeys != 0 {
				res = inv.canStoreInInventorySlots(KeyringStart, KeyringStart+(KeyringEnd-KeyringStart), dest, p, &count, false, item, bag, slot)
				if res != ErrOK {
					return res
				}
				if r, d := done(); d {
					return r
				}
			}
			if res = inv.canStoreInInventorySlots(ItemStart, ItemEnd, dest, p, &count, false, item, bag, slot); res != ErrOK {
				return res
			}
			if r, d := done(); d {
				return r
			}
		} else {
			res = inv.canStoreInBag(bag, dest, p, &count, false, false, item, NullBag, slot)
			if res != ErrOK {
				res = inv.canStoreInBag(bag, dest, p, &count, false, true, item, NullBag, slot)
			}
			if res != ErrOK {
				return res
			}
			if r, d := done(); d {
				return r
			}
		}
	}
	if p.Stackable > 1 {
		if res = inv.canStoreInInventorySlots(KeyringStart, KeyringEnd, dest, p, &count, true, item, bag, slot); res != ErrOK {
			return res
		}
		if r, d := done(); d {
			return r
		}
		if res = inv.canStoreInInventorySlots(ItemStart, ItemEnd, dest, p, &count, true, item, bag, slot); res != ErrOK {
			return res
		}
		if r, d := done(); d {
			return r
		}
		if p.BagFamily != 0 {
			for i := uint8(BagStart); i < BagEnd; i++ {
				if res = inv.canStoreInBag(i, dest, p, &count, true, false, item, bag, slot); res != ErrOK {
					continue
				}
				if r, d := done(); d {
					return r
				}
			}
		}
		for i := uint8(BagStart); i < BagEnd; i++ {
			if res = inv.canStoreInBag(i, dest, p, &count, true, true, item, bag, slot); res != ErrOK {
				continue
			}
			if r, d := done(); d {
				return r
			}
		}
	}
	if p.BagFamily != 0 {
		if p.BagFamily&bagFamilyKeys != 0 {
			if res = inv.canStoreInInventorySlots(KeyringStart, KeyringEnd, dest, p, &count, false, item, bag, slot); res != ErrOK {
				return res
			}
			if r, d := done(); d {
				return r
			}
		}
		for i := uint8(BagStart); i < BagEnd; i++ {
			if res = inv.canStoreInBag(i, dest, p, &count, false, false, item, bag, slot); res != ErrOK {
				continue
			}
			if r, d := done(); d {
				return r
			}
		}
	}
	if item != nil && item.IsBag() && !inv.bagIsEmpty(item) {
		return ErrNonemptyBagOverOtherBag
	}
	if res = inv.canStoreInInventorySlots(ItemStart, ItemEnd, dest, p, &count, false, item, bag, slot); res != ErrOK {
		return res
	}
	if r, d := done(); d {
		return r
	}
	for i := uint8(BagStart); i < BagEnd; i++ {
		if res = inv.canStoreInBag(i, dest, p, &count, false, true, item, bag, slot); res != ErrOK {
			continue
		}
		if r, d := done(); d {
			return r
		}
	}
	return ErrInventoryFull
}
