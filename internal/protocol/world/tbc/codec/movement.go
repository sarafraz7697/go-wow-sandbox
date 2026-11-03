package codec

import (
	"math"
	"wowserver/internal/domain/world"
	"wowserver/internal/protocol/wire"
)

// ValidateMovement checks the complete build-8606 MovementInfo, including
// optional jump, pitch and transport sections, without applying game state.
func ValidateMovement(payload []byte) error {
	r := wire.NewReader(payload)
	flags := r.Uint32()
	r.Uint8()
	r.Uint32()
	finite := true
	floats := func(n int) {
		for i := 0; i < n; i++ {
			v := float64(r.Float32())
			if math.IsNaN(v) || math.IsInf(v, 0) {
				finite = false
			}
		}
	}
	floats(4)
	if flags&0x200 != 0 {
		r.Uint64()
		floats(4)
		r.Uint32()
	}
	if flags&(0x200000|0x2000000) != 0 {
		floats(1)
	}
	r.Uint32()
	if flags&0x1000 != 0 {
		floats(4)
	}
	if flags&0x4000000 != 0 {
		floats(1)
	}
	if !finite {
		return ErrWorldPacket
	}
	return r.Finish()
}

// DecodeMovementPosition validates the full packet before exposing its pose.
// Transport-relative coordinates and flight are outside the local movement scope.
func DecodeMovementPosition(payload []byte) (world.Position, error) {
	var p world.Position
	if err := ValidateMovement(payload); err != nil {
		return p, err
	}
	r := wire.NewReader(payload)
	flags := r.Uint32()
	if flags&(0x200|0x1000000|0x2000000) != 0 {
		return p, ErrWorldPacket
	}
	r.Uint8()
	r.Uint32()
	p.X, p.Y, p.Z, p.Orientation = r.Float32(), r.Float32(), r.Float32(), r.Float32()
	return p, r.Err()
}
func ValidateRootAck(payload []byte, guid uint64) error {
	r := wire.NewReader(payload)
	if r.Uint64() != guid {
		return ErrWorldPacket
	}
	r.Uint32()
	if r.Err() != nil {
		return r.Err()
	}
	return ValidateMovement(r.Bytes(r.Remaining()))
}
