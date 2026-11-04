package codec

import (
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
	f "wowserver/internal/protocol/world/tbc/updatefields"
)

// StandState constants for UNIT_FIELD_BYTES_1 byte 0.
// Reference: SharedDefines.h UnitStandStateType + Unit.h UNIT_BYTES_1_OFFSET_STAND_STATE = 0.
const (
	StandStateStand = 0
	StandStateSit   = 1
)

// LogoutStateUpdate builds a partial SMSG_UPDATE_OBJECT that changes only
// UNIT_FIELD_BYTES_1 (stand state) for one player. This mirrors the reference
// HandleLogoutRequestOpcode path: SetStandState(UNIT_STAND_STATE_SIT).
//
// Reference: Unit::SetStandState writes to UNIT_FIELD_BYTES_1 byte 0
// (UNIT_BYTES_1_OFFSET_STAND_STATE = 0). Do NOT use UNIT_FIELD_BYTES_2 —
// its byte 0 is the sheath state, which is why an earlier version made the
// player draw weapons instead of sitting.
//
// Format: SMSG_UPDATE_OBJECT with one UPDATETYPE_VALUES block.
//
//	uint32 blockCount (1)
//	uint8  hasTransport (0)
//	uint8  UPDATETYPE_VALUES (0)
//	packedGUID
//	uint8  maskBlockCount
//	uint32[] mask
//	uint32[] values (only set fields, ascending field order)
func LogoutStateUpdate(guid uint32, standState uint8) (world.Packet, error) {
	// UNIT_FIELD_BYTES_1 = 159 → block 159/32 = 4, bit 159%32 = 31.
	field := uint32(f.UNIT_FIELD_BYTES_1)
	blockCount := int(field/32) + 1
	mask := make([]uint32, blockCount)
	mask[field/32] |= 1 << (field % 32)

	w := wire.NewWriter(64)
	w.Uint32(1) // update count: one object
	w.Uint8(0)  // hasTransport: false (matches UpdateData::BuildPacket layout).
	w.Uint8(0)  // UPDATETYPE_VALUES (0): packed GUID + mask + values only.
	w.PackedGUID(uint64(guid))
	w.Uint8(uint8(blockCount))
	for _, m := range mask {
		w.Uint32(m)
	}
	// Only the set field is sent. Put standState in byte 0 (little-endian:
	// lowest 8 bits). The other three bytes (pet loyalty, vis flags, misc)
	// are 0 for a player with no pet and no special flags; sending 0 is
	// safe because the login update already set them.
	w.Uint32(uint32(standState))
	b, err := w.Result()
	if err != nil {
		return world.Packet{}, err
	}
	return world.Packet{Opcode: op.SMSG_UPDATE_OBJECT, Payload: b}, nil
}

// StandStateUpdate is a thin wrapper kept for call-site clarity.
func StandStateUpdate(guid uint32, standState uint8) (world.Packet, error) {
	return LogoutStateUpdate(guid, standState)
}

// StandStateMessage builds the standalone SMSG_STANDSTATE_UPDATE packet the
// reference sends in addition to the partial UPDATE_OBJECT when it calls
// Unit::SetStandState on a player. Body: 1 byte state. (UNIT_FIELD_BYTES_1 is
// also written by the partial update; sending both matches MaNGOS exactly.)
//
// Reference: Unit::SetStandState -> WorldPacket(SMSG_STANDSTATE_UPDATE, 1) << state.
func StandStateMessage(standState uint8) world.Packet {
	return world.Packet{Opcode: op.SMSG_STANDSTATE_UPDATE, Payload: []byte{standState}}
}

// ForceMoveRoot builds SMSG_FORCE_MOVE_ROOT (Player::SetRoot(true)).
// Body: packedGUID + uint32(0) counter. The reference also broadcasts this
// to other players; we send it to the owning session only (local movement,
// no visibility/broadcast pipeline yet).
//
// Reference: PlayerMovement.cpp Player::SetRoot(true).
func ForceMoveRoot(guid uint32) world.Packet {
	w := wire.NewWriter(16)
	w.PackedGUID(uint64(guid))
	w.Uint32(0)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_FORCE_MOVE_ROOT, Payload: b}
}

// ForceMoveUnroot builds SMSG_FORCE_MOVE_UNROOT (Player::SetRoot(false)).
// Body: packedGUID + uint32(0) counter.
//
// Reference: PlayerMovement.cpp Player::SetRoot(false).
func ForceMoveUnroot(guid uint32) world.Packet {
	w := wire.NewWriter(16)
	w.PackedGUID(uint64(guid))
	w.Uint32(0)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_FORCE_MOVE_UNROOT, Payload: b}
}

// LogoutResponse builds SMSG_LOGOUT_RESPONSE for the CMSG_LOGOUT_REQUEST
// handler. Body: uint32 reason (0 = ok), uint8 instantLogout.
//
// Reference: MiscHandler.cpp HandleLogoutRequestOpcode.
//
//	reason = 1 in combat, 2 dueling, 3 falling; instantLogout = 1 means the
//	server logs the player out immediately without the 20-second countdown.
func LogoutResponse(reason uint32, instantLogout bool) world.Packet {
	w := wire.NewWriter(8)
	w.Uint32(reason)
	if instantLogout {
		w.Uint8(1)
	} else {
		w.Uint8(0)
	}
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_LOGOUT_RESPONSE, Payload: b}
}

// UnitFlagStunned returns 0 — kept only so call sites that referenced it
// in a previous slice still compile. Stun is intentionally not applied
// during logout until the weapon-display pipeline is correct.
func UnitFlagStunned() uint32 { return 0 }
