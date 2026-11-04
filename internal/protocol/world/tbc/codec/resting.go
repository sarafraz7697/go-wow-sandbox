package codec

import (
	"math"

	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
	f "wowserver/internal/protocol/world/tbc/updatefields"
)

// PlayerFlagsResting matches PLAYER_FLAGS_RESTING (0x20) from Player.h.
// Mirrors the bit the reference sets/clears in SetRestType.
const PlayerFlagsResting = 0x20

// IsPointInAreaTrigger mirrors IsPointInAreaTriggerZone from DBCStores.cpp:
//   - if the trigger has a radius, the player must be inside the sphere
//     (radius + delta) of the trigger centre on the same map;
//   - otherwise the player must be inside the oriented box (extent_x/y/z
//     rotated by box_orientation around the trigger centre).
//
// delta defaults to 5.0 in the reference's areatrigger opcode handler.
func IsPointInAreaTrigger(t gamedata.TavernTrigger, mapID uint32, x, y, z, delta float32) bool {
	if mapID != t.Map {
		return false
	}
	if t.Radius > 0 {
		dx, dy, dz := x-t.X, y-t.Y, z-t.Z
		r := t.Radius + delta
		return dx*dx+dy*dy+dz*dz <= r*r
	}
	// Oriented-box check. MaNGOS rotates the player's relative position by
	// 2π - box_orientation and then bounds-checks against the half-extents.
	// The angle is small for taverns so the trig cost is negligible.
	if t.BoxX <= 0 && t.BoxY <= 0 && t.BoxZ <= 0 {
		return false
	}
	dx, dy := x-t.X, y-t.Y
	angle := 2*math.Pi - float64(t.BoxOrientation)
	cos := float32(math.Cos(angle))
	sin := float32(math.Sin(angle))
	rx := dx*cos - dy*sin
	ry := dx*sin + dy*cos
	halfX, halfY, halfZ := t.BoxX+delta, t.BoxY+delta, t.BoxZ+delta
	if rx < -halfX || rx > halfX {
		return false
	}
	if ry < -halfY || ry > halfY {
		return false
	}
	if dz := z - t.Z; dz < -halfZ || dz > halfZ {
		return false
	}
	return true
}

// RestingStateUpdate builds a partial SMSG_UPDATE_OBJECT that toggles only
// the PLAYER_FLAGS_RESTING bit of PLAYER_FLAGS. The current flags are
// passed in (read from p.PlayerFlags) so the new value preserves every
// other bit set on login.
//
// Field index: PLAYER_FLAGS = 236 → block 7, bit 12.
func RestingStateUpdate(guid uint32, currentFlags uint32, resting bool) (world.Packet, error) {
	field := uint32(f.PLAYER_FLAGS)
	blockCount := int(field/32) + 1
	mask := make([]uint32, blockCount)
	mask[field/32] |= 1 << (field % 32)

	value := currentFlags &^ PlayerFlagsResting
	if resting {
		value |= PlayerFlagsResting
	}

	w := wire.NewWriter(64)
	w.Uint32(1) // update count
	w.Uint8(0)  // hasTransport
	w.Uint8(0)  // UPDATETYPE_VALUES
	w.PackedGUID(uint64(guid))
	w.Uint8(uint8(blockCount))
	for _, m := range mask {
		w.Uint32(m)
	}
	w.Uint32(value)
	b, err := w.Result()
	if err != nil {
		return world.Packet{}, err
	}
	return world.Packet{Opcode: op.SMSG_UPDATE_OBJECT, Payload: b}, nil
}
