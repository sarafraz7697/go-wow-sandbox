package account

// DOMAIN_WORLD_SPLIT_PATCH: domain model (no protocol, no SQL)

// WorldSession is a snapshot of the login session, not a player or character.
// Key must never be logged. Expansion is capped to the adapter's capabilities.
type WorldSession struct {
	ID        uint64
	Name      string
	Key       [40]byte
	Expansion uint8
	// Security is account.gmlevel (AccountTypes: 0 player .. 3 admin).
	Security uint8
}
