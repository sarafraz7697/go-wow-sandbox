package codec

import (
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
	f "wowserver/internal/protocol/world/tbc/updatefields"
)

// StandState constants for UNIT_FIELD_BYTES_2 byte 0.
// Reference: SharedDefines.h UNIT_STAND_STATE_*.
const (
	StandStateStand = 0
	StandStateSit   = 1
)

// UNIT_FLAG_STUNNED from SharedDefines.h. Set during logout countdown so the
// client cannot move and the sit animation plays cleanly.
const unitFlagStunned = 0x00040000

// LogoutStateUpdate builds a partial SMSG_UPDATE_OBJECT that changes
// UNIT_FIELD_BYTES_2 (stand state) and UNIT_FIELD_FLAGS (stun) in one
// update. This mirrors the reference HandleLogoutRequestOpcode path:
// SetStandState(SIT) + SetRoot + SetFlag(UNIT_FLAG_STUNNED).
//
// Use stand=StandStateStand and flags=0 to revert on logout cancel or
// movement-cancelled logout.
//
// Format: UPDATETYPE_VALUES (1) carries packed GUID + value mask + values,
// with no typeID, no updateFlag and no movement block.
func LogoutStateUpdate(guid uint32, standState uint8, unitFlags uint32) (world.Packet, error) {
	// Build the value mask. Both fields live in block 6 (192..223).
	bytes2Field := uint32(f.UNIT_FIELD_BYTES_2) // 209
	flagsField := uint32(f.UNIT_FIELD_FLAGS)    // 207
	blockCount := int(bytes2Field/32) + 1       // 209/32 = 6 → 7 blocks
	if int(flagsField/32)+1 > blockCount {
		blockCount = int(flagsField/32) + 1
	}
	mask := make([]uint32, blockCount)
	mask[bytes2Field/32] |= 1 << (bytes2Field % 32)
	mask[flagsField/32] |= 1 << (flagsField % 32)

	w := wire.NewWriter(64)
	w.Uint32(1) // update count: one object
	w.Uint8(1)  // UPDATETYPE_VALUES: GUID + mask + values only.
	w.PackedGUID(uint64(guid))
	w.Uint8(uint8(blockCount))
	for _, m := range mask {
		w.Uint32(m)
	}
	// Send the set fields in ascending field-index order, not mask-bit order.
	// UNIT_FIELD_FLAGS (207) < UNIT_FIELD_BYTES_2 (209).
	w.Uint32(unitFlags)
	w.Uint32(uint32(standState) | 0x2800)
	b, err := w.Result()
	if err != nil {
		return world.Packet{}, err
	}
	return world.Packet{Opcode: op.SMSG_UPDATE_OBJECT, Payload: b}, nil
}

// StandStateUpdate is a thin wrapper kept for call-site clarity when only the
// stand state changes (no stun flag toggle).
func StandStateUpdate(guid uint32, standState uint8) (world.Packet, error) {
	return LogoutStateUpdate(guid, standState, 0)
}

// UnitFlagStunned returns the UNIT_FLAG_STUNNED value used during logout.
// Exposed as a function so callers do not hardcode the constant.
func UnitFlagStunned() uint32 { return unitFlagStunned }
