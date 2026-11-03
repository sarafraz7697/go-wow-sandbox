// Package movement is the reserved home for the phase-10 movement use cases.
//
// DOMAIN_USECASES_PATCH: contract only. Report validation policy and the
// anti-cheat rules that decide which client movement may mutate world state
// must live here (or in domain/world), never in protocol route handlers.
// Routes decode MSG_MOVE_* packets into application/world Intents
// (IntentMove); this package must not import protocol, storage or app.
package movement
