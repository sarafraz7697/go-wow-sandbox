// Package world defines the phase-10 contract between the protocol adapters
// and the simulation loop. Skeleton only: the shapes may be refined while the
// loop is implemented, but the dependency direction is fixed —
// protocol -> application/world -> internal/world, never backwards.
//
// DOMAIN_WORLD_SPLIT_PATCH
package world

import "context"

// Session identifies one attached client connection inside the simulation.
type Session struct {
	ID   uint64 // account id, matches the protocol admission registry
	GUID uint32 // player object; zero until the player enters the world
	Name string
	Map  uint32
}

// IntentKind enumerates the client-driven state changes the loop accepts.
type IntentKind uint8

const (
	IntentMove   IntentKind = iota + 1 // decoded movement reports
	IntentChat                         // say/yell/party lines
	IntentLogout                       // explicit logout requests
)

// Intent is a decoded, version-independent request produced by a protocol
// route handler. Payload is owned by the producing use-case; the loop treats
// it as opaque and validates every field before applying it.
type Intent struct {
	Session uint64
	Kind    IntentKind
	Payload any
}

// EventKind enumerates loop-produced updates delivered to attached sessions.
type EventKind uint8

const (
	EventMovement       EventKind = iota + 1 // object movement snapshots
	EventChat                                // chat deliveries
	EventLogoutComplete                      // teardown confirmation
)

// Event is a version-independent simulation result. A protocol-side pump
// encodes events into build-specific packets outside the loop.
type Event struct {
	Kind    EventKind
	Source  uint64 // emitting session or object id
	Payload any
}

// Outbox receives events for one attached session. The loop must never block
// on a slow consumer; dropping or disconnecting is the implementation's call.
type Outbox interface {
	Enqueue(Event) error
}

// World is the write side used by protocol adapters. Implementations must be
// safe for concurrent use, only enqueue work, and never simulate inline.
type World interface {
	Attach(session Session, out Outbox) error
	Detach(session uint64) error
	Submit(intent Intent) error
}

// Runner drives the deterministic tick loop on its own goroutine. Exactly one
// goroutine owns all mutable map state; see internal/world/doc.go.
type Runner interface {
	Run(ctx context.Context) error
}
