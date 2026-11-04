// Package world will host the phase-10 domain model owned by the simulation:
// world objects and their invariants, movement and chat rules as plain
// values, and the player-in-world state transitions the loop applies.
//
// DOMAIN_USECASES_PATCH: contract only. Types here are pure values with
// invariants — no I/O, no SQL. The Packet struct is a pure value
// (opcode + payload bytes), not a protocol wire format; the protocol
// layer converts between its own packet type and this one at the
// broadcast boundary. One goroutine owns all mutable simulation state
// at runtime (see internal/world/doc.go); this package must not import
// protocol, storage, application, world (simulation) or app.
package world
