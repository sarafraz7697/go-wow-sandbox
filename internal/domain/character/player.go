package character

// PACKAGE_SKELETON_PATCH: domain model (no protocol, no SQL)

import "errors"

var ErrPlayerUnavailable = errors.New("character unavailable for world login")
var ErrPlayerUnsupported = errors.New("phase 9 supports living level-1 Human Warriors on map 0 only")

// Player is a session-owned snapshot. Local movement is applied by the movement
// use case after packet/bounds validation; there is no authoritative simulation.
type Player struct {
	Summary
	Orientation                                  float32
	XP, Money, Health, Rage, Played, LevelPlayed uint32
	HomeMap, HomeZone                            uint32
	HomeX, HomeY, HomeZ                          float32
	Tutorials                                    [8]uint32
	Actions                                      [132]uint32
	Spells                                       []uint32
	Skills                                       []InitialSkill
	Reputations                                  []Reputation
	Items                                        []PlayerItem
	Entered                                      bool
}
type Reputation struct {
	Faction, Flags uint32
	Standing       int32
}

// Fields is the validated native item record read by the 8606 storage adapter.
type PlayerItem struct {
	GUID, Entry uint32
	Slot        uint8
	Fields      [60]uint32
}
