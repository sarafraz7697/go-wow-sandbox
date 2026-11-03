package character

import (
	"context"
	"errors"
	"testing"
	"wowserver/internal/account"
	. "wowserver/internal/application/character"
	. "wowserver/internal/domain/character"
)

type profileStub struct{ err error }

func (p profileStub) Build(r CreateRequest) (Creation, error) {
	r.Name = "Normalized"
	return Creation{Request: r}, p.err
}

type creationStoreStub struct {
	calls int
	name  string
	err   error
}

func (s *creationStoreStub) Create(_ context.Context, _ account.WorldSession, _ string, p Creation) (uint32, error) {
	s.calls++
	s.name = p.Request.Name
	return 42, s.err
}
func TestCreationService(t *testing.T) {
	store := &creationStoreStub{}
	service := NewService(profileStub{err: ErrAppearance}, store)
	if _, err := service.Create(context.Background(), account.WorldSession{}, "", CreateRequest{}); !errors.Is(err, ErrAppearance) || store.calls != 0 {
		t.Fatal("invalid profile reached storage")
	}
	service = NewService(profileStub{}, store)
	if id, err := service.Create(context.Background(), account.WorldSession{}, "", CreateRequest{Name: "raw"}); err != nil || id != 42 || store.name != "Normalized" {
		t.Fatal("validated plan not persisted")
	}
	store.err = ErrNameInUse
	if _, err := service.Create(context.Background(), account.WorldSession{}, "", CreateRequest{}); !errors.Is(err, ErrNameInUse) {
		t.Fatal("storage rejection hidden")
	}
}
