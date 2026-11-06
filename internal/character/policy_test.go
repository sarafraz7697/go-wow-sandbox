package character

import (
	"errors"
	"testing"
	. "wowserver/internal/domain/character"
)

func TestCreationPolicy(t *testing.T) {
	for _, tc := range []struct {
		facts CreationFacts
		err   error
	}{
		{CreationFacts{RealmCount: 9, OtherRealmsCount: 40}, nil},
		{CreationFacts{RealmCount: 10}, ErrRealmFull},
		{CreationFacts{RealmCount: 9, OtherRealmsCount: 41}, ErrAccountFull},
		{CreationFacts{OtherRealmsCount: ^uint32(0)}, ErrAccountFull},
		{CreationFacts{NameTaken: true}, ErrNameInUse},
		{CreationFacts{RealmIcon: 1, HasOpposingFaction: true}, ErrFaction},
		{CreationFacts{RealmIcon: 8, HasOpposingFaction: true}, ErrFaction},
		{CreationFacts{RealmIcon: 16, HasOpposingFaction: true}, ErrFaction},
		{CreationFacts{RealmIcon: 1, HasOpposingFaction: false}, nil},
		{CreationFacts{RealmIcon: 1, HasOpposingFaction: true, Privileged: true}, nil},
		{CreationFacts{RealmIcon: 6, HasOpposingFaction: true}, nil},
		{CreationFacts{RealmIcon: 0, HasOpposingFaction: true}, nil},
	} {
		if err := DefaultCreationPolicy().Check(tc.facts); !errors.Is(err, tc.err) {
			t.Fatalf("facts=%+v got %v want %v", tc.facts, err, tc.err)
		}
	}
	two := DefaultCreationPolicy()
	two.AllowTwoSideAccounts = true
	if err := two.Check(CreationFacts{RealmIcon: 1, HasOpposingFaction: true}); err != nil {
		t.Fatalf("AllowTwoSideAccounts must lift the team rule, got %v", err)
	}
	// Reference order: name in use is reported before the limits.
	if err := DefaultCreationPolicy().Check(CreationFacts{NameTaken: true, RealmCount: 10}); !errors.Is(err, ErrNameInUse) {
		t.Fatalf("name-in-use must win over realm limit, got %v", err)
	}
}
