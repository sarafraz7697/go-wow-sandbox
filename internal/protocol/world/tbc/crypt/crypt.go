// Package crypt implements build 8606 world header cryptography (the
// AuthCrypt port). It knows nothing about framing or sockets.
//
// DOMAIN_WORLD_SPLIT_PATCH: moved from internal/protocol/world/tbc/crypt.go.
// deriveKey is now DeriveKey, headerCipher is now Cipher, and the cipher is
// obtained through NewCipher so callers never touch unexported state.
package crypt

import (
	"crypto/hmac"
	"crypto/sha1"
	"errors"
)

// ErrCryptState reports a zero session key or a direction whose cipher was
// already enabled.
var ErrCryptState = errors.New("header encryption already enabled or session key is zero")

// DeriveKey follows AuthCrypt::Init -> HMACSHA1::UpdateBigNumber: the
// little-endian session integer is serialized at its minimal byte length.
func DeriveKey(session [40]byte) [20]byte {
	seed := []byte{0x38, 0xa7, 0x83, 0x15, 0xf8, 0x92, 0x25, 0x30, 0x71, 0x98, 0x67, 0xb1, 0x8c, 0x04, 0xe2, 0xaa}
	n := len(session)
	for n > 0 && session[n-1] == 0 {
		n--
	}
	h := hmac.New(sha1.New, seed)
	h.Write(session[:n])
	var key [20]byte
	copy(key[:], h.Sum(nil))
	return key
}

// Cipher is one direction of the header obfuscation. Each direction owns a
// separate state. Never copy a state after first use or share it between
// writers/readers. There is no RC4 or keystream drop in TBC.
type Cipher struct {
	key      [20]byte
	index    int
	previous byte
}

// NewCipher derives the header cipher from a session key. A zero key is
// rejected, matching the reference "no session" behavior.
func NewCipher(key [40]byte) (*Cipher, error) {
	if key == ([40]byte{}) {
		return nil, ErrCryptState
	}
	return &Cipher{key: DeriveKey(key)}, nil
}

// Encrypt obfuscates a server header in place.
func (c *Cipher) Encrypt(header []byte) {
	for i, b := range header {
		v := (b ^ c.key[c.index]) + c.previous
		header[i] = v
		c.previous = v
		c.index = (c.index + 1) % len(c.key)
	}
}

// Decrypt restores a client header in place.
func (c *Cipher) Decrypt(header []byte) {
	for i, b := range header {
		header[i] = (b - c.previous) ^ c.key[c.index]
		c.previous = b
		c.index = (c.index + 1) % len(c.key)
	}
}
