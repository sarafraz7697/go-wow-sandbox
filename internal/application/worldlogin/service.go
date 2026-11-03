// Package worldlogin owns the world-entry use cases (phase 9 seam, phase 10
// growth point).
//
// DOMAIN_USECASES_PATCH: protocol routes keep the session FSM and packet
// sequencing; the decisions about which player snapshot may attach to a
// verified session live here. When the phase-10 loop lands, relogin and
// admission rules join this package instead of protocol handlers.
package worldlogin

import (
	"context"
	"errors"

	character "wowserver/internal/domain/character"
)

// Players is the persistence port of this use case, defined consumer-side.
// Implemented by storage/characters.Store.
type Players interface {
	AcquirePlayer(context.Context, uint64, uint32) (*character.Player, error)
	ReleasePlayer(context.Context, uint64, *character.Player, uint32) error
}

// Service attaches and detaches loaded player snapshots to verified sessions.
// The method names intentionally match the protocol-side PlayerStore port so
// the service satisfies it structurally.
type Service struct {
	players Players
}

func NewService(players Players) *Service {
	return &Service{players: players}
}

// AcquirePlayer loads and reserves the account's player for world entry.
// A zero account or GUID is never valid; ownership, online-state and
// supported-shape rules are enforced by the port within its transaction.
func (s *Service) AcquirePlayer(ctx context.Context, accountID uint64, guid uint32) (*character.Player, error) {
	if s.players == nil {
		return nil, errors.New("world login store not configured")
	}
	if accountID == 0 || guid == 0 {
		return nil, character.ErrPlayerUnavailable
	}
	return s.players.AcquirePlayer(ctx, accountID, guid)
}

// ReleasePlayer saves and releases the snapshot at logout or teardown.
func (s *Service) ReleasePlayer(ctx context.Context, accountID uint64, p *character.Player, seconds uint32) error {
	if s.players == nil {
		return errors.New("world login store not configured")
	}
	if p == nil {
		return nil
	}
	return s.players.ReleasePlayer(ctx, accountID, p, seconds)
}
