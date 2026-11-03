package account

// DOMAIN_WORLD_SPLIT_PATCH: domain model (no protocol, no SQL)

import (
	"errors"
	"strings"
)

var ErrNotFound = errors.New("account not found")
var ErrConflict = errors.New("account already exists")
var ErrChanged = errors.New("account changed during authentication")

// Phase 2 deliberately supports printable ASCII credentials only. This is a
// subset of MaNGOS's Latin case normalization, not Unicode case folding.
func Normalize(value string) (string, error) {
	if len(value) < 1 || len(value) > 16 {
		return "", errors.New("credentials must contain 1 to 16 ASCII characters")
	}
	for _, c := range []byte(value) {
		if c < 33 || c > 126 || c == ':' {
			return "", errors.New("credentials must be printable ASCII without colon or spaces")
		}
	}
	return strings.ToUpper(value), nil
}

type Account struct {
	ID             uint64
	Name           string
	Salt, Verifier [32]byte
	Banned         bool
	Suspended      bool
	Security       uint8
	CredentialHash string
	LockedIP       string
}
