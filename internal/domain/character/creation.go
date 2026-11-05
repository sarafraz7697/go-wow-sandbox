package character

// PACKAGE_SKELETON_PATCH: domain model (no protocol, no SQL)

import (
	"errors"
	"wowserver/internal/entity"
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
	Template entity.ItemPrototype
	Slot     uint8
	Count    uint32
}
type InitialSkill struct{ ID, Value, Max uint32 }
type InitialReputation struct{ Faction, Flags uint32 }

// Creation is a validated initial snapshot; storage owns GUID allocation.
type Creation struct {
	// TaxiMask mirrors the reference TaxiMaskSize (16 uint32 words, see
	// DBCStructure.h). Only words up to the highest known node are nonzero.
	TaxiMask [16]uint32
	Request  CreateRequest
	Start    gamedata.Start
	Health   uint32
	// Mana (power1) and Energy (power4) are persisted so a freshly created
	// character enters the world with full resources. The reference computes
	// max mana in InitStatsForLevel and restores current powers to max on
	// every login (Player::LoadFromDB); writing the same value at creation
	// reproduces that first-login state because this server clamps the DB
	// value to the recalculated maximum instead of full-healing.
	Mana   uint32
	Energy uint32
	// Team is the ChrRaces BaseLanguage value (7=Alliance, 1=Horde) used by
	// the PvP-realm faction rule, mirroring Player::TeamForRace.
	Team        uint32
	Items       []InitialItem
	Spells      []uint32
	Actions     []gamedata.Action
	Skills      []InitialSkill
	Reputations []InitialReputation
}
