// Package world defines the version-independent packet envelope. Opcode
// numbers, framing and header cryptography belong to version adapters.
package world

// Packet owns Payload. A sender must not mutate it while a write is in progress.
type Packet struct {
	Opcode  uint32
	Payload []byte
}
