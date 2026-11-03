package gamedata

import (
	"errors"
	"strings"
)

var ErrNameLength = errors.New("character name must contain 2 to 12 letters")
var ErrNameAlphabet = errors.New("this implementation supports ASCII Latin character names only")
var ErrNameReserved = errors.New("character name is reserved")

// NormalizeName implements the initial Latin subset, not all MaNGOS locale
// policies. Duplicate-name checks remain the character database's responsibility.
func (c *Catalog) NormalizeName(name string, allowReserved bool) (string, error) {
	if len(name) < 2 || len(name) > 12 {
		return "", ErrNameLength
	}
	for _, b := range []byte(name) {
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z') {
			return "", ErrNameAlphabet
		}
	}
	lower := strings.ToLower(name)
	if !allowReserved && c.reserved[lower] {
		return "", ErrNameReserved
	}
	return strings.ToUpper(lower[:1]) + lower[1:], nil
}
