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
// We do NOT set UNIT_FLAG_STUNNED here. The reference sets it together with
// SetRoot, but without a weapon-display pipeline the stunned flag makes the
// client draw weapons (sheath = MELEE). A future slice can re-add stun once
// virtual item slots are sent correctly.
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

// UnitFlagStunned returns 0 — kept only so call sites that referenced it
// in a previous slice still compile. Stun is intentionally not applied
// during logout until the weapon-display pipeline is correct.
func UnitFlagStunned() uint32 { return 0 }
