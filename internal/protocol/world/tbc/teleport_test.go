package tbc

import (
	"testing"

	op "wowserver/internal/protocol/world/tbc/opcodes"
)

func TestTeleportPackets(t *testing.T) {
	a := teleportAck(7, 1, 2, 3, 4)
	if a.Opcode != op.MSG_MOVE_TELEPORT_ACK || len(a.Payload) != 2+4+4+1+4+16+4 {
		t.Fatalf("teleport ack: opcode %#x len %d", a.Opcode, len(a.Payload))
	}
	if p := transferPending(530); len(p.Payload) != 4 || p.Opcode != op.SMSG_TRANSFER_PENDING {
		t.Fatalf("transfer pending: %v", p)
	}
	if n := newWorld(530, 1, 2, 3, 4); len(n.Payload) != 20 || n.Opcode != op.SMSG_NEW_WORLD {
		t.Fatalf("new world: %v", n)
	}
	if (&teleportState{}).pending() {
		t.Fatal("empty state must not be pending")
	}
}
