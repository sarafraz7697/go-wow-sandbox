// Package chat is the reserved home for the phase-10 chat use cases
// (say/yell/party lines).
//
// DOMAIN_USECASES_PATCH: contract only. Channel, language and spam rules must
// live here (or in domain/world), never in protocol route handlers. Routes
// decode packets into application/world Intents (IntentChat) and encode
// Events; this package must not import protocol, storage or app.
package chat
