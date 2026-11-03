// Package delete is the character-deletion use case.
//
// DOMAIN_USECASES_PATCH: the ownership and online-state guard is a use-case
// rule; storage stays a replaceable port. Protocol handlers call this service
// and only decide how to serialize the result.
package delete

import (
	"context"
	"errors"

	"wowserver/internal/domain/character"
)

// Deleter is the persistence port, defined consumer-side.
// Implemented by storage/characters.Store.
type Deleter interface {
	Delete(context.Context, uint64, uint32) error
}

type Service struct {
	store Deleter
}

func NewService(store Deleter) *Service {
	return &Service{store: store}
}

// Delete removes the account's character. A zero account or GUID is invalid;
// ownership and offline-state rules are enforced by the port within its
// transaction. Returns character.ErrPlayerUnavailable for missing, owned-by-
// another-account, or online characters.
func (s *Service) Delete(ctx context.Context, accountID uint64, guid uint32) error {
	if s.store == nil {
		return errors.New("character delete store not configured")
	}
	if accountID == 0 || guid == 0 {
		return character.ErrPlayerUnavailable
	}
	return s.store.Delete(ctx, accountID, guid)
}
