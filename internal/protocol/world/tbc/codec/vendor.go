package codec

import (
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

// VendorRow is one entry of SMSG_LIST_INVENTORY after the server-side
// filtering of WorldSession::SendListInventory.
type VendorRow struct {
	ItemID, DisplayID, MaxCount, Price, MaxDurability, BuyCount, ExtendedCost uint32
}

// ListInventory builds SMSG_LIST_INVENTORY exactly like
// WorldSession::SendListInventory (ItemHandlerVendor.cpp):
//
//	ObjectGuid vendor (8 bytes)
//	uint8      count
//	count x { uint32 slot(1..count), itemId, displayId, maxcount (0xFFFFFFFF
//	          = unlimited), price, maxDurability, buyCount, extendedCost }
//
// A vendor without any (visible) item gets count 0 followed by one extra
// zero byte, the "Vendor has no inventory" error code.
func ListInventory(vendor uint64, rows []VendorRow) world.Packet {
	w := wire.NewWriter(8 + 1 + len(rows)*32 + 1)
	w.Uint64(vendor)
	if len(rows) == 0 {
		w.Uint8(0) // count == 0, next byte is the error code
		w.Uint8(0) // "Vendor has no inventory"
		b, _ := w.Result()
		return world.Packet{Opcode: op.SMSG_LIST_INVENTORY, Payload: b}
	}
	w.Uint8(uint8(len(rows)))
	for i, r := range rows {
		w.Uint32(uint32(i + 1))
		w.Uint32(r.ItemID)
		w.Uint32(r.DisplayID)
		w.Uint32(r.MaxCount)
		w.Uint32(r.Price)
		w.Uint32(r.MaxDurability)
		w.Uint32(r.BuyCount)
		w.Uint32(r.ExtendedCost)
	}
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_LIST_INVENTORY, Payload: b}
}

// Sell result codes (SellResult, Item.h).
const (
	SellErrCantFindItem   = 1
	SellErrCantSellItem   = 2
	SellErrCantFindVendor = 3
	SellErrYouDontOwnItem = 4
)

// SellError builds SMSG_SELL_ITEM like Player::SendSellError: vendor guid,
// item guid, an optional uint32 param (only when > 0) and the result code.
func SellError(msg uint8, vendor, item uint64, param uint32) world.Packet {
	w := wire.NewWriter(8 + 8 + 4 + 1)
	w.Uint64(vendor)
	w.Uint64(item)
	if param > 0 {
		w.Uint32(param)
	}
	w.Uint8(msg)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_SELL_ITEM, Payload: b}
}
