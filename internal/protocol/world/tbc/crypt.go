// Package tbc implements build 8606 World framing, not a World server.
package tbc

import (
	"crypto/hmac"
	"crypto/sha1"
)

// deriveKey follows AuthCrypt::Init -> HMACSHA1::UpdateBigNumber: the
// little-endian session integer is serialized at its minimal byte length.
func deriveKey(session [40]byte) [20]byte {
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

// Each direction owns a separate state. Never copy a state after first use or
// share it between writers/readers. There is no RC4 or keystream drop in TBC.
type headerCipher struct {
	key      [20]byte
	index    int
	previous byte
}

func (c *headerCipher) encrypt(header []byte) {
	for i, b := range header {
		v := (b ^ c.key[c.index]) + c.previous
		header[i] = v
		c.previous = v
		c.index = (c.index + 1) % len(c.key)
	}
}
func (c *headerCipher) decrypt(header []byte) {
	for i, b := range header {
		header[i] = (b - c.previous) ^ c.key[c.index]
		c.previous = b
		c.index = (c.index + 1) % len(c.key)
	}
}
