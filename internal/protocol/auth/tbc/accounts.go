package tbc

import (
	"context"
	"errors"
	"wowserver/internal/domain/account"
)

// lookupAccount resolves the account and also returns the normalized
// client-sent login name. The SRP identity hashes must use the client-sent
// name (reference AuthSocket.cpp hashes _login, which the client already
// sends in the expected form), not the DB-stored username: with a
// case-insensitive collation the two can differ for accounts created
// outside this tooling, and hashing the stored form would break the M1 check.
func (h Handler) lookupAccount(ctx context.Context, rawName, ip string, trace func(string, ...any)) (account.Account, string, byte) {
	var empty account.Account
	if access, ok := h.Store.(AccessStore); ok {
		allowed, err := access.IPAllowed(ctx, ip)
		if err != nil {
			return empty, "", 8
		}
		if !allowed {
			return empty, "", 3
		}
	}
	name, err := account.Normalize(rawName)
	if err != nil {
		return empty, "", 4
	}
	trace("looking up account")
	a, err := h.Store.Find(ctx, name)
	if errors.Is(err, account.ErrNotFound) {
		trace("account not found")
		return empty, "", 4
	}
	if err != nil {
		trace("account lookup failed", "reason", "database read failed")
		return empty, "", 8
	}
	switch a.Access(ip) {
	case account.Banned:
		return empty, "", 3
	case account.Suspended:
		return empty, "", 0x0c
	case account.IPLocked:
		return empty, "", 0x10
	}
	return a, name, 0
}
