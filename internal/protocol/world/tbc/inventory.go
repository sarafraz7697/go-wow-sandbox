package tbc

import (
	"context"
	"fmt"
	"strings"

	"wowserver/internal/domain/character"
	"wowserver/internal/domain/inventory"
	"wowserver/internal/entity"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	f "wowserver/internal/protocol/world/tbc/updatefields"
)

const spellDualWield = 674 // Dual Wield: sets Player::m_canDualWield

// sessionInventory returns the session's live inventory, building it from the
// loaded snapshot on first use, with the player facts refreshed.
func (h *Handler) sessionInventory(s *Session) *inventory.Inventory {
	if s.player == nil || h.Data == nil {
		return nil
	}
	p := s.player
	st := inventory.PlayerState{
		Level: p.Level, Race: p.Race, Class: p.Class,
		Alive: p.Health > 0,
		SkillValue: func(id uint32) uint16 {
			for _, sk := range p.Skills {
				if sk.ID == id {
					return uint16(sk.Value)
				}
			}
			return 0
		},
		HasSpell: func(id uint32) bool {
			for _, sp := range p.Spells {
				if sp == id {
					return true
				}
			}
			return false
		},
	}
	st.CanDualWield = st.HasSpell(spellDualWield)
	if s.inv == nil {
		proto := func(entry uint32) (entity.ItemPrototype, bool) { return h.Data.ItemPrototype(entry) }
		s.loadedItems = make(map[uint32]struct{}, len(p.Items))
		for _, it := range p.Items {
			s.loadedItems[it.GUID] = struct{}{}
		}
		s.inv = inventory.New(p.Items, p.GUID, proto, st, func() uint32 { return h.allocateItemGUID(s) })
	} else {
		s.inv.SetState(st)
	}
	return s.inv
}

// allocateItemGUID reserves a persistent item guid (0 on failure).
func (h *Handler) allocateItemGUID(s *Session) uint32 {
	a, ok := h.Players.(interface {
		AllocateItemGUID(context.Context, uint32) (uint32, error)
	})
	if !ok {
		return 0
	}
	g, err := a.AllocateItemGUID(context.Background(), s.player.GUID)
	if err != nil {
		s.log.Warn("item guid allocation failed", "error", err)
		return 0
	}
	s.allocatedItems = append(s.allocatedItems, g)
	return g
}

// applyInventoryEvents sends what the reference sends for a mutation: item
// create/destroy blocks immediately, in order, followed by the queued
// player/container VALUES updates (Map::SendObjectUpdates).
func (h *Handler) applyInventoryEvents(s *Session, inv *inventory.Inventory, events []inventory.Event) {
	var player []codec.FieldChange
	var bagOrder []uint32
	bags := map[uint32][]codec.FieldChange{}
	for _, e := range events {
		switch e.Kind {
		case inventory.EvItemCreate:
			_ = s.send(codec.ItemCreateUpdate(e.Snap, e.Bag, e.Snap.Proto()))
		case inventory.EvItemDestroy:
			_ = s.send(codec.DestroyObjectGUID(codec.ItemGUID(e.GUID)))
		case inventory.EvInvSlot:
			at := f.PLAYER_FIELD_INV_SLOT_HEAD + int(e.Slot)*2
			hi := uint32(0)
			if e.Value != 0 {
				hi = 0x40000000
			}
			player = append(player, codec.FieldChange{Index: at, Value: e.Value}, codec.FieldChange{Index: at + 1, Value: hi})
		case inventory.EvVisible:
			var it *inventory.Item
			if e.Value != 0 {
				it = inv.ItemByGUID(e.GUID)
			}
			player = append(player, codec.VisibleItemFields(int(e.Slot), it)...)
		case inventory.EvBagSlot:
			at := f.CONTAINER_FIELD_SLOT_1 + int(e.Slot)*2
			hi := uint32(0)
			if e.Value != 0 {
				hi = 0x40000000
			}
			if _, seen := bags[e.GUID]; !seen {
				bagOrder = append(bagOrder, e.GUID)
			}
			bags[e.GUID] = append(bags[e.GUID], codec.FieldChange{Index: at, Value: e.Value}, codec.FieldChange{Index: at + 1, Value: hi})
		}
	}
	if len(player) > 0 {
		_ = s.send(codec.ValuesUpdate(uint64(s.player.GUID), player))
	}
	for _, g := range bagOrder {
		_ = s.send(codec.ValuesUpdate(codec.ItemGUID(g), bags[g]))
	}
	if len(events) > 0 {
		s.player.Items = inv.Items()
		s.player.InventoryDirty = true
		if equipmentChanged(events) {
			h.refreshStats(s)
		}
	}
}

func (h *Handler) finishInventoryOp(s *Session, inv *inventory.Inventory, fl inventory.Failure, events []inventory.Event) {
	if fl.Code != inventory.ErrOK {
		_ = s.send(codec.InventoryChangeFailure(fl))
		return
	}
	h.applyInventoryEvents(s, inv, events)
}

// inventoryReady reports whether inventory packets can be handled now.
func (h *Handler) inventoryReady(s *Session) *inventory.Inventory {
	if s.player == nil || !s.player.Entered {
		return nil
	}
	return h.sessionInventory(s)
}

// syncInventoryForSave copies the live inventory into the snapshot and
// computes which item rows were deleted during the session.
func (h *Handler) syncInventoryForSave(s *Session) {
	if s.inv == nil || s.player == nil || !s.player.InventoryDirty {
		return
	}
	items := s.inv.Items()
	s.player.Items = items
	live := make(map[uint32]struct{}, len(items))
	for _, it := range items {
		live[it.GUID] = struct{}{}
	}
	var removed []uint32
	for g := range s.loadedItems {
		if _, ok := live[g]; !ok {
			removed = append(removed, g)
		}
	}
	for _, g := range s.allocatedItems {
		if _, ok := live[g]; !ok {
			removed = append(removed, g)
		}
	}
	s.player.RemovedItems = removed
	s.player.EquipmentCache = equipmentCache(items)
}

// CMSG_SWAP_INV_ITEM: uint8 srcslot, uint8 dstslot.
func (h *Handler) swapInvItem(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	r := wire.NewReader(p.Payload)
	src, dst := r.Uint8(), r.Uint8()
	if err := r.Finish(); err != nil {
		return nil, err
	}
	if inv := h.inventoryReady(s); inv != nil {
		fl, ev := inv.SwapInvItem(src, dst)
		h.finishInventoryOp(s, inv, fl, ev)
	}
	return nil, nil
}

// CMSG_SWAP_ITEM: uint8 dstbag, dstslot, srcbag, srcslot.
func (h *Handler) swapItem(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	r := wire.NewReader(p.Payload)
	dstBag, dstSlot, srcBag, srcSlot := r.Uint8(), r.Uint8(), r.Uint8(), r.Uint8()
	if err := r.Finish(); err != nil {
		return nil, err
	}
	if inv := h.inventoryReady(s); inv != nil {
		fl, ev := inv.SwapItemPos(dstBag, dstSlot, srcBag, srcSlot)
		h.finishInventoryOp(s, inv, fl, ev)
	}
	return nil, nil
}

// CMSG_AUTOEQUIP_ITEM: uint8 srcbag, srcslot.
func (h *Handler) autoEquipItem(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	r := wire.NewReader(p.Payload)
	bag, slot := r.Uint8(), r.Uint8()
	if err := r.Finish(); err != nil {
		return nil, err
	}
	if inv := h.inventoryReady(s); inv != nil {
		fl, ev := inv.AutoEquipItem(bag, slot)
		h.finishInventoryOp(s, inv, fl, ev)
	}
	return nil, nil
}

// CMSG_AUTOEQUIP_ITEM_SLOT: ObjectGuid itemGuid, uint8 dstslot.
func (h *Handler) autoEquipItemSlot(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	r := wire.NewReader(p.Payload)
	guid := r.Uint64()
	slot := r.Uint8()
	if err := r.Finish(); err != nil {
		return nil, err
	}
	if inv := h.inventoryReady(s); inv != nil && guid>>32 == 0x40000000 {
		fl, ev := inv.AutoEquipItemSlot(uint32(guid), slot)
		h.finishInventoryOp(s, inv, fl, ev)
	}
	return nil, nil
}

// CMSG_SPLIT_ITEM: uint8 srcbag, srcslot, dstbag, dstslot, count.
func (h *Handler) splitItem(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	r := wire.NewReader(p.Payload)
	sb, ss, db, ds, count := r.Uint8(), r.Uint8(), r.Uint8(), r.Uint8(), r.Uint8()
	if err := r.Finish(); err != nil {
		return nil, err
	}
	if inv := h.inventoryReady(s); inv != nil {
		fl, ev := inv.SplitItem(sb, ss, db, ds, uint32(count))
		h.finishInventoryOp(s, inv, fl, ev)
	}
	return nil, nil
}

// CMSG_DESTROYITEM: uint8 bag, slot, count, data1, data2, data3.
func (h *Handler) destroyItem(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	r := wire.NewReader(p.Payload)
	bag, slot, count := r.Uint8(), r.Uint8(), r.Uint8()
	r.Uint8()
	r.Uint8()
	r.Uint8()
	if err := r.Finish(); err != nil {
		return nil, err
	}
	if inv := h.inventoryReady(s); inv != nil {
		fl, ev := inv.DestroyItem(bag, slot, uint32(count))
		h.finishInventoryOp(s, inv, fl, ev)
	}
	return nil, nil
}

// equipmentCache ports the PlayerSave.cpp equipmentCache string: for each of
// the 19 equipment slots "<item id> <MAKE_PAIR32(perm ench, temp ench)> ".
func equipmentCache(items []character.PlayerItem) string {
	var slots [inventory.EquipmentEnd]*character.PlayerItem
	for i := range items {
		if items[i].Bag == 0 && items[i].Slot < inventory.EquipmentEnd {
			slots[items[i].Slot] = &items[i]
		}
	}
	var b strings.Builder
	for _, it := range slots {
		var entry, pair uint32
		if it != nil {
			entry = it.Entry
			pair = it.Fields[22] | it.Fields[22+3]<<16 // PERM and TEMP enchantment ids
		}
		fmt.Fprintf(&b, "%d %d ", entry, pair)
	}
	return b.String()
}

// equipmentChanged reports whether an equipment or bag-equip slot changed,
// which is when the reference re-applies item mods (_ApplyItemMods).
func equipmentChanged(events []inventory.Event) bool {
	for _, e := range events {
		if e.Kind == inventory.EvVisible || (e.Kind == inventory.EvInvSlot && e.Slot < inventory.EquipmentEnd) {
			return true
		}
	}
	return false
}
