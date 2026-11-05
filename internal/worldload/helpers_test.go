package worldload

import "wowserver/internal/entity"

// ResetForTest clears the registry so tests can register exactly the
// loaders they exercise. The production init() registrations are also
// cleared; tests that want them must re-register explicitly.
func ResetForTest() {
	registryMu.Lock()
	registry = make(map[string]Loader)
	registryMu.Unlock()
}

// newDatasetForTest allocates a fresh dataset.
func newDatasetForTest() *Dataset { return entity.NewDataset() }
