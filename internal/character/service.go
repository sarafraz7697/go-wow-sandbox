package character

import (
	"context"
	"wowserver/internal/account"
)

type CreationProfile interface {
	Build(CreateRequest) (Creation, error)
}
type CreationStore interface {
	Create(context.Context, account.WorldSession, string, Creation) (uint32, error)
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
func (s *Service) Create(ctx context.Context, a account.WorldSession, ip string, r CreateRequest) (uint32, error) {
	plan, err := s.profile.Build(r)
	if err != nil {
		return 0, err
	}
	return s.store.Create(ctx, a, ip, plan)
}
