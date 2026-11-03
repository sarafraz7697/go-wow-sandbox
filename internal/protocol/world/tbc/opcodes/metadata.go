// Package opcodes is the generated build 8606 catalogue. A known opcode does
// not imply an implemented Go handler. Routing/handler registration is phase 5.
package opcodes

//go:generate node ../../../../../tools/generate-opcodes.mjs

type Status uint8

const (
	STATUS_AUTHED Status = iota
	STATUS_LOGGEDIN
	STATUS_TRANSFER
	STATUS_LOGGEDIN_OR_RECENTLY_LOGGEDOUT
	STATUS_NEVER
	STATUS_UNHANDLED
)

type Processing uint8

const (
	PROCESS_INPLACE Processing = iota
	PROCESS_THREADUNSAFE
	PROCESS_THREADSAFE
)

// Processing preserves reference metadata, not a directive to spawn Go
// goroutines. CMSG_AUTH_SESSION is STATUS_NEVER here because the transport
// handles it before session dispatch; it is not permitted by this table.
type Metadata struct {
	Name       string
	Status     Status
	Processing Processing
}

// Lookup returns a copy, so consumers cannot mutate the shared catalogue.
func Lookup(opcode uint32) (Metadata, bool) {
	if opcode >= uint32(len(catalogue)) {
		return Metadata{}, false
	}
	m := catalogue[opcode]
	return m, m.Name != ""
}
