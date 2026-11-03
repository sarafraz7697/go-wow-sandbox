// Package routes is the reserved home for phase-10 packet routes.
//
// DOMAIN_USECASES_PATCH: contract only. A route here may only decode a
// packet, call an application use case or submit an application/world Intent,
// and encode the reply (decode -> use case -> encode). It must not import
// storage, internal/world or app, and must not reach into session FSM
// internals: state-dependent handlers stay registered by the tbc session
// package until the simulation loop takes over.
package routes
