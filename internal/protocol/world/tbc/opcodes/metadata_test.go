package opcodes

import "testing"

func TestCatalogue(t *testing.T) {
	for i := uint32(0); i < NUM_MSG_TYPES; i++ {
		m, ok := Lookup(i)
		if !ok || m.Name == "" {
			t.Fatal("missing opcode", i)
		}
	}
	for _, op := range []uint32{NUM_MSG_TYPES, 0xffffffff} {
		if _, ok := Lookup(op); ok {
			t.Fatal("unknown accepted")
		}
	}
	for _, tc := range []struct {
		op     uint32
		name   string
		status Status
	}{{0x37, "CMSG_CHAR_ENUM", STATUS_AUTHED}, {0x1ed, "CMSG_AUTH_SESSION", STATUS_NEVER}, {0x1dc, "CMSG_PING", STATUS_AUTHED}, {0xee, "MSG_MOVE_HEARTBEAT", STATUS_LOGGEDIN}} {
		m, ok := Lookup(tc.op)
		if !ok || m.Name != tc.name || m.Status != tc.status {
			t.Fatal(tc, m)
		}
	}
	m, _ := Lookup(CMSG_CHAR_ENUM)
	m.Name = "changed"
	other, _ := Lookup(CMSG_CHAR_ENUM)
	if other.Name == m.Name {
		t.Fatal("mutable table exposed")
	}
}
