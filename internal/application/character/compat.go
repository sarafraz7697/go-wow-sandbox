package character

// DOMAIN_USECASES_PATCH: temporary re-export shim. The use case moved to
// internal/application/character/create; this alias file keeps existing
// callers and package tests compiling unchanged. Remove it in the next
// cleanup slice once call sites and tests import the create package
// directly. White-box tests that construct Service literals with unexported
// fields must move to package create.

import create "wowserver/internal/application/character/create"

type CreationProfile = create.CreationProfile
type CreationStore = create.CreationStore
type Service = create.Service

var NewService = create.NewService
