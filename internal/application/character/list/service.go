// Package list is the character-screen enumeration use case.
//
// DOMAIN_USECASES_PATCH: the account guard and the packet-capacity bound are
// use-case rules and live here; storage stays a replaceable port. Protocol
// handlers call this service and only decide how to serialize the result.
package list

import (
	"context"
	"errors"

	character "wowserver/internal/domain/character"
)

// MaxCharacters matches the TBC SMSG_CHAR_ENUM count byte. The storage port
// enforces the same bound against the database; this is the use-case side.
const MaxCharacters = 255

// Reader is the persistence port of this use case, defined consumer-side.
// Implemented by storage/characters.Store.
type Reader interface {
	List(context.Context, uint64) ([]character.Summary, error)
}

// Service loads the verified account's characters for the character screen.
type Service struct {
	reader Reader
}

func NewService(reader Reader) *Service {
	return &Service{reader: reader}
}

func (s *Service) List(ctx context.Context, accountID uint64) ([]character.Summary, error) {
	if s.reader == nil {
		return nil, errors.New("character list store not configured")
	}
	if accountID == 0 {
		return nil, errors.New("invalid character account")
	}
	rows, err := s.reader.List(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if len(rows) > MaxCharacters {
		return nil, errors.New("character count exceeds TBC packet capacity")
	}
	return rows, nil
}
