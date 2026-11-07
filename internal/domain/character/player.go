package character

// PACKAGE_SKELETON_PATCH: domain model (no protocol, no SQL)

import "errors"

var ErrPlayerUnavailable = errors.New("character unavailable for world login")
var ErrPlayerUnsupported = errors.New("character data or pending login operation is not supported")

// Delete guards mirroring HandleCharDeleteOpcode's early rejections: a guild
// leader or an arena team captain is refused before any row is touched.
var ErrGuildLeader = errors.New("character is a guild leader")
var ErrArenaCaptain = errors.New("character is an arena team captain")

// Player is a session-owned snapshot. Local movement is applied by the movement
// use case after packet/bounds validation; there is no authoritative simulation.
type Player struct {
	Stats *Stats
	Summary
	Orientation                                  float32
	XP, Money, Health, Rage, Played, LevelPlayed uint32
	Mana, Focus, Energy, Happiness               uint32
	HomeMap, HomeZone                            uint32
	HomeX, HomeY, HomeZ                          float32
	Tutorials                                    [8]uint32
	Actions                                      [132]uint32
	Spells                                       []uint32
	Skills                                       []InitialSkill
	Reputations                                  []Reputation
	Items                                        []PlayerItem
	// InventoryDirty is set when the in-world inventory changed; the store
	// then rewrites character_inventory/item_instance at release.
	// RemovedItems lists item guids deleted during the session.
	InventoryDirty bool
	// Cinematic is characters.cinematic: 0 until the race intro was triggered.
	Cinematic uint32
	// HomeMissing is set when character_homebind has no row for the character.
	HomeMissing bool
	// LoadWarnings lists child rows the loader skipped instead of rejecting the login.
	LoadWarnings []string
	// EquipmentCache is characters.equipmentCache (item id + perm/temp
	// enchant pair for every equipment slot), shown in the character list.
	EquipmentCache string
	RemovedItems   []uint32
	Entered                                      bool
	// Resting mirrors PLAYER_FLAGS_RESTING (0x20). True while the player is
	// inside an areatrigger_tavern volume; the logout handler uses it to take
	// the instant-logout path the reference calls LogoutPlayer(true).
	Resting bool
	// InnTriggerID is the AreaTrigger.dbc id of the tavern the player is
	// currently resting in. Zero when not resting. The tick loop re-checks
	// the player's position against this trigger's volume every tick and
	// clears Resting when the player leaves.
	InnTriggerID uint32
}
type Reputation struct {
	Faction, Flags uint32
	Standing       int32
}

// Fields is the validated native item record read by the 8606 storage adapter.
type PlayerItem struct {
	GUID, Entry uint32
	Bag         uint32
	Slot        uint8
	Fields      [60]uint32
}
