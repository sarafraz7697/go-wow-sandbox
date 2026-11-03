// Package world will host the phase-10 domain model owned by the simulation:
// world objects and their invariants, movement and chat rules as plain
// values, and the player-in-world state transitions the loop applies.
//
// DOMAIN_USECASES_PATCH: contract only. Types here are pure values with
// invariants — no I/O, no packets, no SQL. One goroutine owns all mutable
// state at runtime (see internal/world/doc.go); this package must not import
// protocol, storage, application, world or app.
package world
