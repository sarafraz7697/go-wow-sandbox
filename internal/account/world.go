package account

// WorldSession is a snapshot of the login session, not a player or character.
// Key must never be logged. Expansion is capped to the adapter's capabilities.
type WorldSession struct {
	ID        uint64
	Name      string
	Key       [40]byte
	Expansion uint8
}
