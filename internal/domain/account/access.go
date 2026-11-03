package account

// DOMAIN_WORLD_SPLIT_PATCH: domain model (no protocol, no SQL)

// AccessDecision is independent of protocol result codes.
type AccessDecision uint8

const (
	Allowed AccessDecision = iota
	Banned
	Suspended
	IPLocked
)

func (a Account) Access(ip string) AccessDecision {
	if a.Banned {
		return Banned
	}
	if a.Suspended {
		return Suspended
	}
	if a.LockedIP != "" && a.LockedIP != ip {
		return IPLocked
	}
	return Allowed
}
