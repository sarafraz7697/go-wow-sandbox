package movement

import (
	"errors"
	"math"

	"wowserver/internal/domain/character"
	"wowserver/internal/domain/world"
)

// Apply updates only the owning session's snapshot. This initial local-movement
// use case performs bounds checks, not terrain, speed or collision validation.
// It must only be called by the session event loop, never the socket reader.
func Apply(player *character.Player, position world.Position) error {
	if player == nil || !player.Entered || !position.Valid() {
		return errors.New("invalid player movement position")
	}
	o := math.Mod(float64(position.Orientation), 2*math.Pi)
	if o < 0 {
		o += 2 * math.Pi
	}
	player.X, player.Y, player.Z = position.X, position.Y, position.Z
	player.Orientation = float32(o)
	return nil
}
