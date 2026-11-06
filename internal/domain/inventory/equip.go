package inventory

import "wowserver/internal/entity"

// isTwoHandUsed ports Player::IsTwoHandUsed.
func (inv *Inventory) isTwoHandUsed() bool {
	m := inv.ItemByPos(Bag0, EquipMainhand)
	return m != nil && m.proto.InventoryType == inv2HWeapon
}

// FindEquipSlot ports Player::FindEquipSlot(proto, slot, swap).
func (inv *Inventory) FindEquipSlot(p entity.ItemPrototype, slot uint8, swap bool) uint8 {
	cls := inv.st.Class
	slots := [4]uint8{NullSlot, NullSlot, NullSlot, NullSlot}
	switch p.InventoryType {
	case invHead:
		slots[0] = 0
	case invNeck:
		slots[0] = 1
	case invShoulders:
		slots[0] = 2
	case invBody:
		slots[0] = 3
	case invChest, invRobe:
		slots[0] = 4
	case invWaist:
		slots[0] = 5
	case invLegs:
		slots[0] = 6
	case invFeet:
		slots[0] = 7
	case invWrists:
		slots[0] = 8
	case invHands:
		slots[0] = 9
	case invFinger:
		slots[0], slots[1] = 10, 11
	case invTrinket:
		slots[0], slots[1] = 12, 13
	case invCloak:
		slots[0] = 14
	case invWeapon:
		slots[0] = EquipMainhand
		if inv.st.CanDualWield {
			slots[1] = EquipOffhand
		}
	case invShield:
		slots[0] = EquipOffhand
	case invRanged:
		slots[0] = EquipRanged
	case inv2HWeapon:
		slots[0] = EquipMainhand
	case invTabard:
		slots[0] = 18
	case invWeaponMain:
		slots[0] = EquipMainhand
	case invWeaponOff, invHoldable:
		slots[0] = EquipOffhand
	case invThrown, invRangedRight:
		slots[0] = EquipRanged
	case invBag:
		slots = [4]uint8{BagStart, BagStart + 1, BagStart + 2, BagStart + 3}
	case invRelic:
		switch p.SubClass {
		case 7: // libram
			if cls == classPaladin {
				slots[0] = EquipRanged
			}
		case 8: // idol
			if cls == classDruid {
				slots[0] = EquipRanged
			}
		case 9: // totem
			if cls == classShaman {
				slots[0] = EquipRanged
			}
		case 0: // misc (sigil slot in later versions)
			if cls == classWarlock {
				slots[0] = EquipRanged
			}
		}
	default:
		return NullSlot
	}
	if slot != NullSlot {
		if swap || inv.ItemByPos(Bag0, slot) == nil {
			for _, s := range slots {
				if s == slot {
					return slot
				}
			}
		}
		return NullSlot
	}
	// search a free slot first
	for _, s := range slots {
		if s != NullSlot && inv.ItemByPos(Bag0, s) == nil {
			// with a 2H weapon equipped the off-hand slot is empty but not free
			if s != EquipOffhand || !inv.isTwoHandUsed() {
				return s
			}
		}
	}
	// no free slot: the first suitable one (its item will be replaced)
	for _, s := range slots {
		if s != NullSlot {
			return s
		}
	}
	return NullSlot
}

// canUseProto ports Player::CanUseItem(ItemPrototype const*).
func (inv *Inventory) canUseProto(p entity.ItemPrototype) Result {
	classMask := int32(1) << (inv.st.Class - 1)
	raceMask := int32(1) << (inv.st.Race - 1)
	if p.AllowableClass&classMask == 0 || p.AllowableRace&raceMask == 0 {
		return ErrYouCanNeverUseThatItem
	}
	if p.RequiredSkill != 0 {
		v := inv.st.skill(p.RequiredSkill)
		if v == 0 {
			return ErrNoRequiredProficiency
		} else if uint32(v) < p.RequiredSkillRank {
			return ErrCantEquipSkill
		}
	}
	if p.RequiredSpell != 0 && !inv.st.hasSpell(p.RequiredSpell) {
		return ErrNoRequiredProficiency
	}
	if uint32(inv.st.Level) < p.RequiredLevel {
		return ErrCantEquipLevel
	}
	return ErrOK
}

// CanUseItem ports Player::CanUseItem(Item*, direct_action).
func (inv *Inventory) CanUseItem(it *Item, direct bool) Result {
	if it == nil {
		return ErrItemNotFound
	}
	if !inv.st.Alive && direct {
		return ErrYouAreDead
	}
	if msg := inv.canUseProto(it.proto); msg != ErrOK {
		return msg
	}
	if sk := itemSkill(it.proto); sk != 0 && inv.st.skill(sk) == 0 {
		return ErrNoRequiredProficiency
	}
	if it.proto.RequiredReputationFaction != 0 && inv.st.ReputationRank != nil &&
		inv.st.ReputationRank(it.proto.RequiredReputationFaction) < it.proto.RequiredReputationRank {
		return ErrCantEquipReputation
	}
	return ErrOK
}

// hasItemEquipped ports HasItemOrGemWithIdEquipped (gems are not modelled).
func (inv *Inventory) hasItemEquipped(entry, count uint32, except uint8) bool {
	var n uint32
	for s := uint8(0); s < EquipmentEnd; s++ {
		if s == except {
			continue
		}
		if it := inv.ItemByPos(Bag0, s); it != nil && it.Entry == entry {
			n += it.Count()
			if n >= count {
				return true
			}
		}
	}
	return false
}

// canEquipUniqueItem ports Player::CanEquipUniqueItem.
func (inv *Inventory) canEquipUniqueItem(it *Item, except uint8) Result {
	if it.proto.Flags&flagUniqueEquipped != 0 && inv.hasItemEquipped(it.proto.ItemID, 1, except) {
		return ErrItemUniqueEquipable
	}
	return ErrOK
}

// CanEquipItem ports Player::CanEquipItem(slot, dest, pItem, swap, direct_action).
func (inv *Inventory) CanEquipItem(slot uint8, it *Item, swap, direct bool) (Result, uint16) {
	if it == nil {
		if !swap {
			return ErrItemNotFound, 0
		}
		return ErrItemsCantBeSwapped, 0
	}
	p := it.proto
	// CanTakeMoreSimilarItems(pItem)
	if res, _ := inv.canTakeMoreSimilarItems(it.Entry, it.Count(), it); res != ErrOK {
		return res, 0
	}
	if direct {
		if inv.st.Stunned {
			return ErrYouAreStunned, 0
		}
		if !canChangeEquipStateInCombat(p) && inv.st.InCombat {
			return ErrNotInCombat, 0
		}
		if inv.st.LoggingOut {
			return ErrYouAreStunned, 0
		}
		if inv.st.InCombat && p.Class == classWeapon && inv.st.WeaponSwitchCooldown {
			return ErrCantDoRightNow, 0
		}
		if inv.st.Casting {
			return ErrCantDoRightNow, 0
		}
	}
	eslot := inv.FindEquipSlot(p, slot, swap)
	if eslot == NullSlot {
		return ErrItemCantBeEquipped, 0
	}
	if msg := inv.CanUseItem(it, direct); msg != ErrOK {
		return msg, 0
	}
	if !swap && inv.ItemByPos(Bag0, eslot) != nil {
		return ErrNoEquipmentSlotAvailable, 0
	}
	except := NullSlot
	if swap {
		except = eslot
	}
	if r := inv.canEquipUniqueItem(it, except); r != ErrOK {
		return r, 0
	}
	if p.Class == classQuiver {
		for i := uint8(BagStart); i < BagEnd; i++ {
			if b := inv.ItemByPos(Bag0, i); b != nil && b != it {
				if b.proto.Class == p.Class && (!swap || b.Slot != eslot) {
					if b.proto.SubClass == subAmmoPouch {
						return ErrCanEquipOnly1AmmoPouch, 0
					}
					return ErrCanEquipOnly1Quiver, 0
				}
			}
		}
	}
	t := p.InventoryType
	if eslot == EquipOffhand {
		if t == invWeapon || t == invWeaponOff {
			if !inv.st.CanDualWield {
				return ErrCantDualWield, 0
			}
		} else if t == inv2HWeapon {
			return ErrCantDualWield, 0
		}
		if inv.isTwoHandUsed() {
			return ErrCantEquipWithTwohanded, 0
		}
	}
	if t == inv2HWeapon {
		if eslot != EquipMainhand {
			return ErrItemCantBeEquipped, 0
		}
		if off := inv.ItemByPos(Bag0, EquipOffhand); off != nil {
			var offDest []PosCount
			if !direct || inv.CanUnequipItem(Pos(Bag0, EquipOffhand), false) != ErrOK ||
				inv.CanStoreItem(NullBag, NullSlot, &offDest, off, false) != ErrOK {
				if swap {
					return ErrItemsCantBeSwapped, 0
				}
				return ErrInventoryFull, 0
			}
		}
	}
	return ErrOK, Pos(Bag0, eslot)
}

// CanUnequipItem ports Player::CanUnequipItem(pos, swap).
func (inv *Inventory) CanUnequipItem(pos uint16, swap bool) Result {
	bag, slot := posParts(pos)
	if !IsEquipmentPos(bag, slot) && !IsBagPos(pos) {
		return ErrOK
	}
	it := inv.ItemByPos(bag, slot)
	if it == nil {
		return ErrOK
	}
	if !canChangeEquipStateInCombat(it.proto) && inv.st.InCombat {
		return ErrNotInCombat
	}
	if inv.st.LoggingOut {
		return ErrYouAreStunned
	}
	if !swap && it.IsBag() && !inv.bagIsEmpty(it) {
		return ErrCanOnlyDoWithEmptyBags
	}
	return ErrOK
}
