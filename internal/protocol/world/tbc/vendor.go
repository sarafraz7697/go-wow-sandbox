package tbc

import (
	"context"
	"math"

	"wowserver/internal/domain/character"
	"wowserver/internal/entity"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
)

const (
	npcFlagVendor       = 0x00000080 // UNIT_NPC_FLAG_VENDOR
	interactionDistance = 5.0        // INTERACTION_DISTANCE
	playerFlagGM        = 0x00000008 // PLAYER_FLAGS_GM
	defaultObjectSize   = 0.388999998569489
	bindWhenPickedUp    = 1 // BIND_WHEN_PICKED_UP
)

// listInventory handles CMSG_LIST_INVENTORY
// (WorldSession::HandleListInventoryOpcode): ObjectGuid of the vendor.
func (h *Handler) listInventory(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	r := wire.NewReader(p.Payload)
	guid := r.Uint64()
	if err := r.Finish(); err != nil {
		return nil, err
	}
	if s.player == nil || !s.player.Entered || h.Data == nil {
		return nil, nil
	}
	if s.player.Health == 0 { // !GetPlayer()->IsAlive()
		return nil, nil
	}
	pkt := h.vendorInventory(s, guid)
	return &pkt, nil
}

// vendorInventory ports WorldSession::SendListInventory for a creature
// vendor.
func (h *Handler) vendorInventory(s *Session, guid uint64) world.Packet {
	spawn, ok := h.npcIfCanInteract(s, guid, npcFlagVendor)
	if !ok {
		// SendSellError(SELL_ERR_CANT_FIND_VENDOR, NULL, ObjectGuid(), 0)
		return codec.SellError(codec.SellErrCantFindVendor, 0, 0, 0)
	}
	// pCreature->StopMoving() is handled by the creature movement system.
	h.stopCreatureMoving(spawn.GUID)

	var vItems, tItems []entity.VendorItem
	vItems = h.Data.VendorItems(spawn.Entry)
	if tmpl, ok := h.Data.CreatureTemplate(spawn.Entry); ok && tmpl.VendorTemplateId != 0 {
		tItems = h.Data.VendorTemplateItems(tmpl.VendorTemplateId)
	}
	if len(vItems) == 0 && len(tItems) == 0 {
		return codec.ListInventory(guid, nil)
	}

	// uint8 customitems = vItems->GetItemCount();
	// uint8 numitems = customitems + (tItems ? tItems->GetItemCount() : 0);
	customitems := uint8(len(vItems))
	numitems := uint8(int(customitems) + int(uint8(len(tItems))))

	player := s.player
	rep := h.playerReputation(player)
	discount := h.Data.ReputationPriceDiscount(spawn.Faction, rep)
	gm := player.PlayerFlags&playerFlagGM != 0
	raceMask := uint32(1) << (uint32(player.Race) - 1)
	classMask := uint32(1) << (uint32(player.Class) - 1)

	var rows []codec.VendorRow
	for i := 0; i < int(numitems); i++ {
		var item entity.VendorItem
		if i < int(customitems) {
			item = vItems[i]
		} else {
			idx := i - int(customitems)
			if idx >= len(tItems) {
				continue // VendorItemData::GetItem returns NULL
			}
			item = tItems[idx]
		}
		proto, ok := h.Data.ItemPrototype(item.Item)
		if !ok {
			continue
		}
		if !gm {
			// class wrong item skip only for bindable case
			if uint32(proto.AllowableClass)&classMask == 0 && proto.Bonding == bindWhenPickedUp {
				continue
			}
			// race wrong item skip always
			if uint32(proto.AllowableRace)&raceMask == 0 {
				continue
			}
			if item.ConditionID != 0 && !h.playerMeetsCondition(player, uint16(item.ConditionID), 0) {
				continue
			}
		}
		maxCount := uint32(0xFFFFFFFF)
		if item.MaxCount > 0 {
			// Creature::GetVendorItemCurrentCount: a restocking vendor
			// starts full; nothing is sold yet.
			maxCount = item.MaxCount
		}
		rows = append(rows, codec.VendorRow{
			ItemID:        item.Item,
			DisplayID:     proto.DisplayInfoID,
			MaxCount:      maxCount,
			Price:         uint32(math.Floor(float64(float32(proto.BuyPrice) * discount))),
			MaxDurability: proto.MaxDurability,
			BuyCount:      proto.BuyCount,
			ExtendedCost:  item.ExtendedCost,
		})
	}
	return codec.ListInventory(guid, rows)
}

// npcIfCanInteract ports Player::GetNPCIfCanInteractWith for creatures:
// the guid must name a live spawn on the player's map with the NPC flag,
// that is not hostile and within INTERACTION_DISTANCE (extents included,
// InReach -> Placement::WithinDist).
func (h *Handler) npcIfCanInteract(s *Session, guid uint64, npcFlagMask uint32) (gamedata.ResolvedCreatureSpawn, bool) {
	var none gamedata.ResolvedCreatureSpawn
	player := s.player
	if guid == 0 || player == nil || !player.Entered {
		return none, false
	}
	entry, counter, ok := codec.DecodeCreatureGUID(guid)
	if !ok {
		return none, false
	}
	ref, ok := h.Data.CreatureSpawnByGUID(counter)
	if !ok || ref.Data.ID != entry || ref.Data.MapID != player.Map {
		return none, false
	}
	spawn, ok := h.Data.ResolveCreatureSpawn(ref)
	if !ok {
		return none, false
	}
	if npcFlagMask != 0 && spawn.NpcFlags&npcFlagMask == 0 {
		return none, false
	}
	x, y, z, alive := h.creaturePosition(spawn)
	if !alive {
		return none, false
	}
	race, ok := h.Data.Race(uint32(player.Race))
	if !ok {
		return none, false
	}
	if h.Data.CreatureHostileToPlayer(spawn.Faction, race.Faction, h.playerReputation(player)) &&
		player.PlayerFlags&playerFlagGM == 0 {
		return none, false
	}
	playerExtent := float32(defaultObjectSize)
	if display := h.playerDisplay(player); display != 0 {
		if m, ok := h.Data.CreatureModelInfo(display); ok {
			playerExtent = m.BoundingRadius
		}
	}
	dx, dy, dz := float64(x-player.X), float64(y-player.Y), float64(z-player.Z)
	reach := float64(interactionDistance + spawn.BoundingRadius + playerExtent)
	if dx*dx+dy*dy+dz*dz > reach*reach {
		return none, false
	}
	return spawn, true
}

// playerDisplay returns the player's native display id like registerInWorld.
func (h *Handler) playerDisplay(p *character.Player) uint32 {
	race, ok := h.Data.Race(uint32(p.Race))
	if !ok {
		return 0
	}
	if p.Gender == 1 {
		return race.FemaleDisplay
	}
	return race.MaleDisplay
}

// playerReputation adapts the player's stored FactionStates.
func (h *Handler) playerReputation(p *character.Player) gamedata.PlayerReputation {
	standing := make(map[uint32]int32, len(p.Reputations))
	for _, r := range p.Reputations {
		standing[r.Faction] = r.Standing
	}
	return gamedata.PlayerReputation{
		RaceMask:  uint32(1) << (uint32(p.Race) - 1),
		ClassMask: uint32(1) << (uint32(p.Class) - 1),
		Standing:  func(f uint32) (int32, bool) { v, ok := standing[f]; return v, ok },
	}
}

// Condition types of PlayerCondition (ObjectMgr.h ConditionType).
const (
	condNot, condOr, condAnd     = -3, -2, -1
	condNone                     = 0
	condAura                     = 1
	condItem                     = 2
	condItemEquipped             = 3
	condReputationRankMin        = 5
	condTeam                     = 6
	condSkill                    = 7
	condNoAura                   = 11
	condActiveGameEvent          = 12
	condRaceClass                = 14
	condLevel                    = 15
	condNoItem                   = 16
	condSpell                    = 17
	condReserved1, condReserved2 = 20, 21
	condItemWithBank             = 23
	condNoItemWithBank           = 24
	condNotActiveGameEvent       = 25
	condActiveHoliday            = 26
	condNotActiveHoliday         = 27
	condSkillBelow               = 29
	condReputationRankMax        = 30
	condReserved3, condReserved4 = 31, 34
	condGender                   = 35
)

// playerMeetsCondition ports ObjectMgr::IsPlayerMeetToCondition /
// PlayerCondition::Meets for the condition types this server can decide
// from the loaded character snapshot (no auras, no active game events and
// holidays, as nothing activates them). A missing condition row is "not
// met", exactly like the reference. Types that need state the server does
// not track yet (quests, area, instance scripts, waypoints...) are "not
// met" too, which hides the item rather than selling it by mistake.
func (h *Handler) playerMeetsCondition(p *character.Player, id uint16, depth int) bool {
	if depth > 8 {
		return false
	}
	cond, ok := h.Data.PlayerCondition(uint32(id))
	if !ok {
		return false
	}
	v1, v2 := cond.Value1, cond.Value2
	t := int32(cond.Type)
	switch t {
	case condNot:
		return !h.playerMeetsCondition(p, uint16(v1), depth+1)
	case condOr:
		return h.playerMeetsCondition(p, uint16(v1), depth+1) || h.playerMeetsCondition(p, uint16(v2), depth+1)
	case condAnd:
		return h.playerMeetsCondition(p, uint16(v1), depth+1) && h.playerMeetsCondition(p, uint16(v2), depth+1)
	case condNone:
		return true
	case condAura:
		return false // aura state is not simulated: a fresh login has none
	case condNoAura:
		return true
	case condItem:
		return playerItemCount(p, v1, false) >= v2
	case condNoItem:
		return !(playerItemCount(p, v1, false) >= v2)
	case condItemWithBank:
		return playerItemCount(p, v1, true) >= v2
	case condNoItemWithBank:
		return !(playerItemCount(p, v1, true) >= v2)
	case condItemEquipped:
		for _, it := range p.Items {
			if it.Bag == 0 && it.Slot < 19 && it.Entry == v1 {
				return true
			}
		}
		return false
	case condReputationRankMin, condReputationRankMax:
		if !h.Data.HasFaction(v1) { // faction && ...
			return false
		}
		rank := h.Data.ReputationRank(v1, h.playerReputation(p))
		if t == condReputationRankMin {
			return rank >= int(v2)
		}
		return rank <= int(v2)
	case condTeam:
		race, ok := h.Data.Race(uint32(p.Race))
		if !ok {
			return false
		}
		team := uint32(67) // HORDE
		if race.Team == 7 {
			team = 469 // ALLIANCE
		}
		return team == v1
	case condSkill:
		for _, sk := range p.Skills {
			if sk.ID == v1 {
				return sk.Value >= v2
			}
		}
		return false
	case condSkillBelow:
		has, value := false, uint32(0)
		for _, sk := range p.Skills {
			if sk.ID == v1 {
				has, value = true, sk.Value
				break
			}
		}
		if v2 == 1 {
			return !has
		}
		return has && value < v2
	case condActiveGameEvent, condActiveHoliday:
		return false
	case condNotActiveGameEvent, condNotActiveHoliday:
		return true
	case condRaceClass:
		raceMask := uint32(1) << (uint32(p.Race) - 1)
		classMask := uint32(1) << (uint32(p.Class) - 1)
		return (v1 == 0 || raceMask&v1 != 0) && (v2 == 0 || classMask&v2 != 0)
	case condLevel:
		switch v2 {
		case 0:
			return uint32(p.Level) == v1
		case 1:
			return uint32(p.Level) >= v1
		case 2:
			return uint32(p.Level) <= v1
		}
		return false
	case condSpell:
		has := false
		for _, sp := range p.Spells {
			if sp == v1 {
				has = true
				break
			}
		}
		switch v2 {
		case 0:
			return has
		case 1:
			return !has
		}
		return false
	case condGender:
		return uint32(p.Gender) == v1
	case condReserved1, condReserved2, condReserved3, condReserved4:
		return false
	}
	return false
}

// playerItemCount mirrors Player::GetItemCount: backpack (23..38), keyring
// (86..117) and bag contents, plus the bank (39..73) when inBankAlso.
func playerItemCount(p *character.Player, entry uint32, inBankAlso bool) uint32 {
	var n uint32
	for _, it := range p.Items {
		if it.Entry != entry {
			continue
		}
		counted := false
		switch {
		case it.Bag != 0:
			counted = true // contained in a bag
		case it.Slot >= 23 && it.Slot <= 38, it.Slot >= 86 && it.Slot <= 117:
			counted = true
		case inBankAlso && it.Slot >= 39 && it.Slot <= 73:
			counted = true
		}
		if counted {
			n += it.Fields[14] // ITEM_FIELD_STACK_COUNT
		}
	}
	return n
}
