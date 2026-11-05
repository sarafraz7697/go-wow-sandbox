package worldload

import (
	"os"
	"testing"

	"wowserver/internal/entity"
)

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

// allLoadersSnapshot captures the init()-registered loader set once in
// TestMain (after every init() ran, before any test calls ResetForTest).
var allLoadersSnapshot []Loader

func TestMain(m *testing.M) {
	allLoadersSnapshot = Loaders()
	os.Exit(m.Run())
}

// registerSubset re-registers the production loaders whose Name is in
// the wanted set.
func registerSubset(wanted ...string) {
	ResetForTest()
	set := make(map[string]bool, len(wanted))
	for _, w := range wanted {
		set[w] = true
	}
	for _, l := range allLoadersSnapshot {
		if set[l.Name] {
			Register(l)
		}
	}
}
