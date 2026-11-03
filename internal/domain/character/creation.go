package character

// PACKAGE_SKELETON_PATCH: domain model (no protocol, no SQL)

import (
	"errors"
	"wowserver/internal/gamedata"
)

var (
	ErrNameRepeated = errors.New("three consecutive letters in character name")
	ErrDisabled     = errors.New("character combination disabled")
	ErrAppearance   = errors.New("invalid character appearance")
	ErrNameInUse    = errors.New("character name already in use")
	ErrRealmFull    = errors.New("realm character limit reached")
	ErrAccountFull  = errors.New("account character limit reached")
	ErrFaction      = errors.New("opposing faction on PvP realm")
)

type CreateRequest struct {
	Name                                                                      string
	Race, Class, Gender, Skin, Face, HairStyle, HairColor, FacialHair, Outfit uint8
}
type InitialItem struct {
	Template gamedata.Item
	Slot     uint8
	Count    uint32
}
type InitialSkill struct{ ID, Value, Max uint32 }
type InitialReputation struct{ Faction, Flags uint32 }

// Creation is a validated initial snapshot; storage owns GUID allocation.
type Creation struct {
	TaxiMask    [8]uint32
	Request     CreateRequest
	Start       gamedata.Start
	Health      uint32
	Items       []InitialItem
	Spells      []uint32
	Actions     []gamedata.Action
	Skills      []InitialSkill
	Reputations []InitialReputation
}
