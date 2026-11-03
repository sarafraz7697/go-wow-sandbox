// Package world will host the phase-10 simulation: the single-owner game
// loop, map and object state, and the fixed 50 ms tick.
//
// DOMAIN_WORLD_SPLIT_PATCH: contract only — there is no simulation yet.
//
// Ownership rules the implementation must keep:
//
//   - One goroutine owns all mutable world state. No mutex guards map or
//     object state; sessions communicate through mailboxes only
//     (Intent in, Event out; see internal/application/world).
//   - The loop never blocks on sockets or SQL: it enqueues events into
//     per-session outboxes and hands persistence to the storage ports at
//     tick or logout boundaries.
//   - Protocol handlers decode and Submit intents; they never read or write
//     map state directly, and they never import this package.
//   - Static data comes from internal/gamedata and is immutable after
//     startup.
//
// This package must not import protocol, storage, application or app; the
// application/world ports are satisfied structurally. scripts/check_imports.py
// enforces this.
package world
