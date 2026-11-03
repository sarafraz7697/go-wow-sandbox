// Package auth is the reserved home for the login use cases.
//
// DOMAIN_USECASES_PATCH: contract only. Today the ban/lock/IP decisions run
// in internal/domain/account and the SRP handshake stays in protocol/auth/tbc.
// When login grows orchestration of its own (failed-login throttling policy,
// session publishing rules, reconnect admission), it moves here as use cases
// that protocol/auth/tbc calls. This package must not import protocol,
// storage, world or app.
package auth
