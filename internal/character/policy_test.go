package character

import (
	"errors"
	"testing"
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
		{CreationFacts{RealmIcon: 6, HasOpposingFaction: true}, ErrFaction},
		{CreationFacts{RealmIcon: 0, HasOpposingFaction: true}, nil},
	} {
		if err := DefaultCreationPolicy().Check(tc.facts); !errors.Is(err, tc.err) {
			t.Fatalf("facts=%+v got %v want %v", tc.facts, err, tc.err)
		}
	}
}
