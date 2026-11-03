// Package character models account-owned characters independently of the wire.
package character

// Summary is the stored character-selection snapshot, not a loaded player.
type Summary struct {
	GUID                           uint32
	Name                           string
	Race, Class, Gender, Level     uint8
	PlayerBytes, PlayerBytes2      uint32
	Zone, Map                      uint32
	X, Y, Z                        float32
	Guild, PlayerFlags, AtLogin    uint32
	PetEntry, PetDisplay, PetLevel uint32
	EquipmentCache                 string
}
