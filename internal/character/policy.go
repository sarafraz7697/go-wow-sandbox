package character

// CreationFacts is read under the storage transaction's locks.
type CreationFacts struct {
	RealmCount, OtherRealmsCount  uint32
	NameTaken, HasOpposingFaction bool
	RealmIcon                     uint32
}

// CreationPolicy is a value, fixed when wiring the service, never supplied by a client.
type CreationPolicy struct{ RealmLimit, AccountLimit uint32 }

func DefaultCreationPolicy() CreationPolicy { return CreationPolicy{10, 50} }
func (p CreationPolicy) Check(f CreationFacts) error {
	if f.RealmCount >= p.RealmLimit {
		return ErrRealmFull
	}
	if uint64(f.RealmCount)+uint64(f.OtherRealmsCount) >= uint64(p.AccountLimit) {
		return ErrAccountFull
	}
	if f.NameTaken {
		return ErrNameInUse
	}
	if (f.RealmIcon == 1 || f.RealmIcon == 6) && f.HasOpposingFaction {
		return ErrFaction
	}
	return nil
}
