package codec

import (
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

// Movement::MonsterMoveType (MoveSpline.h).
const (
	monsterMoveNormal = 0
	monsterMoveStop   = 1
)

// splineFlagRunmode is MoveSplineFlag::Runmode: PacketBuilder always ORs it
// into the flags word ("client has strange issues without that flag").
const splineFlagRunmode = 0x00000100

// MonsterMoveLinear builds SMSG_MONSTER_MOVE for a two-point linear spline
// exactly as MoveSplineInit::Launch + PacketBuilder::WriteMonsterMove do:
//
//	packed guid | start xyz | uint32 splineId | uint8 MonsterMoveNormal |
//	uint32 flags (Runmode) | uint32 duration | uint32 pointCount(=1) | dest xyz
//
// A linear path with only the final point has last_idx == 1 and no packed
// intermediate offsets (WriteLinearPath).
func MonsterMoveLinear(guid uint64, sx, sy, sz, dx, dy, dz float32, durationMs uint32) world.Packet {
	w := wire.NewWriter(48)
	w.PackedGUID(guid)
	w.Float32(sx)
	w.Float32(sy)
	w.Float32(sz)
	w.Uint32(0) // MoveSpline::GetId (args.splineId is never assigned)
	w.Uint8(monsterMoveNormal)
	w.Uint32(splineFlagRunmode)
	w.Uint32(durationMs)
	w.Uint32(1)
	w.Float32(dx)
	w.Float32(dy)
	w.Float32(dz)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_MONSTER_MOVE, Payload: b}
}

// MonsterMoveStop builds the MoveSplineInit::Stop packet:
// packed guid | position xyz | uint32 splineId | uint8 MonsterMoveStop.
func MonsterMoveStop(guid uint64, x, y, z float32) world.Packet {
	w := wire.NewWriter(32)
	w.PackedGUID(guid)
	w.Float32(x)
	w.Float32(y)
	w.Float32(z)
	w.Uint32(0)
	w.Uint8(monsterMoveStop)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_MONSTER_MOVE, Payload: b}
}
