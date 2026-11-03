package world

import "math"

// Position is a map-local pose, independent of client packet formats.
type Position struct {
	X, Y, Z, Orientation float32
}

func (p Position) Valid() bool {
	for _, v := range []float32{p.X, p.Y, p.Z, p.Orientation} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
	}
	return math.Abs(float64(p.X)) <= 17066.666 && math.Abs(float64(p.Y)) <= 17066.666 && math.Abs(float64(p.Z)) <= 100000
}
