package character

// PACKAGE_SKELETON_PATCH: domain model (no protocol, no SQL)

// CreationFacts is read under the storage transaction's locks.
type CreationFacts struct {
	RealmCount, OtherRealmsCount uint32
	NameTaken                    bool
	// HasOpposingFaction is true when the account already has a character and
	// the team of its first character differs from the team of the new race
	// (or its race is unknown), see HandleCharCreateOpcode.
	HasOpposingFaction bool
	RealmIcon          uint32
	// Privileged is true for accounts with GetSecurity() > SEC_PLAYER.
	Privileged bool
}

// CreationPolicy is a value, fixed when wiring the service, never supplied by a client.
type CreationPolicy struct {
	RealmLimit, AccountLimit uint32
	// AllowTwoSideAccounts mirrors CONFIG_BOOL_ALLOW_TWO_SIDE_ACCOUNTS
	// (AllowTwoSide.Accounts, default 0).
	AllowTwoSideAccounts bool
}

func DefaultCreationPolicy() CreationPolicy {
	return CreationPolicy{RealmLimit: 10, AccountLimit: 50}
}

// IsPvPRealmIcon mirrors World::IsPvPRealm for the realmlist.icon value:
// REALM_TYPE_PVP (1), REALM_TYPE_RPPVP (8) and REALM_TYPE_FFA_PVP (16).
// REALM_TYPE_RP (6) is a roleplaying realm without PvP team rules.
func IsPvPRealmIcon(icon uint32) bool { return icon == 1 || icon == 8 || icon == 16 }

// Check applies the HandleCharCreateOpcode checks in the reference order:
// name in use, account limit (SUM(realmcharacters.numchars) over all
// realms), realm limit, then the PvP team rule.
func (p CreationPolicy) Check(f CreationFacts) error {
	if f.NameTaken {
		return ErrNameInUse
	}
	if uint64(f.RealmCount)+uint64(f.OtherRealmsCount) >= uint64(p.AccountLimit) {
		return ErrAccountFull
	}
	if f.RealmCount >= p.RealmLimit {
		return ErrRealmFull
	}
	allowTwoSide := !IsPvPRealmIcon(f.RealmIcon) || p.AllowTwoSideAccounts || f.Privileged
	if !allowTwoSide && f.HasOpposingFaction {
		return ErrFaction
	}
	return nil
}
