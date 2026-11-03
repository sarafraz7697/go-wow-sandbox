package create

// DOMAIN_USECASES_PATCH: moved from internal/application/character. The
// old package name remains only as a temporary shim (compat.go there).

import (
	"context"

	"wowserver/internal/domain/account"
	domain "wowserver/internal/domain/character"
)

// Application service: coordinates profile validation and persistence.
// Application service: coordinates profile validation and persistence.
// Domain types live in internal/domain/character; this package must not
// import protocol or storage implementations (only interfaces).

type CreationProfile interface {
	Build(domain.CreateRequest) (domain.Creation, error)
}
type CreationStore interface {
	Create(context.Context, account.WorldSession, string, domain.Creation) (uint32, error)
}

// Service coordinates validation and persistence. The store must evaluate the
// configured policy against locked facts before writing, within one transaction.
type Service struct {
	profile CreationProfile
	store   CreationStore
}

func NewService(profile CreationProfile, store CreationStore) *Service {
	return &Service{profile: profile, store: store}
}
func (s *Service) Create(ctx context.Context, a account.WorldSession, ip string, r domain.CreateRequest) (uint32, error) {
	plan, err := s.profile.Build(r)
	if err != nil {
		return 0, err
	}
	return s.store.Create(ctx, a, ip, plan)
}
