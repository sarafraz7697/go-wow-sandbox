package tbc

import (
	"context"
	"time"

	domainworld "wowserver/internal/domain/world"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

// teleportState mirrors Player::SetSemaphoreTeleportNear/Far plus
// m_teleport_dest: a teleport is pending until the client acknowledges it.
type teleportState struct {
	near, far  bool
	mapID      uint32
	x, y, z, o float32
}

func (t *teleportState) pending() bool { return t.near || t.far }

// teleportTo ports Player::TeleportTo for a map the client can load. It
// returns false when the destination is not a known map (reference:
// IsValidMapCoord).
func (h *Handler) teleportTo(s *Session, mapID uint32, x, y, z, o float32) bool {
	p := s.player
	if p == nil || h.Data == nil {
		return false
	}
	if _, ok := h.Data.Map(mapID); !ok {
		s.log.Warn("teleport to unknown map refused", "map", mapID)
		return false
	}
	// Reset movement/falling; the client re-reports its state after the ack.
	if p.Map == mapID {
		// near teleport: remember the destination and ask the client to land
		s.tele = teleportState{near: true, mapID: mapID, x: x, y: y, z: z, o: o}
		_ = s.send(teleportAck(uint64(p.GUID), x, y, z, o))
		return true
	}
	// far teleport: leave the old map first
	s.tele = teleportState{far: true, mapID: mapID, x: x, y: y, z: z, o: o}
	h.removeAurasWithInterrupt(s, gamedata.AuraInterruptChangeMap|gamedata.AuraInterruptMove|gamedata.AuraInterruptTurning)
	_ = s.send(transferPending(mapID))
	if h.Broadcast != nil {
		d := codec.DestroyObject(p.GUID)
		h.Broadcast.Broadcast(p.GUID, domainworld.Packet{Opcode: d.Opcode, Payload: d.Payload}, false)
		h.Broadcast.Unregister(p.GUID)
	}
	// Reference keeps the old position until the ack but saves m_teleport_dest
	// if the player logs out in between; moving the snapshot now has the same
	// effect and keeps the saved position valid.
	p.Map, p.X, p.Y, p.Z, p.Orientation = mapID, x, y, z, o
	s.vis = visibleSet{}
	_ = s.send(newWorld(mapID, x, y, z, o))
	return true
}

// teleportToHomebind ports Player::TeleportToHomebind: the player's own
// facing is kept.
func (h *Handler) teleportToHomebind(s *Session) bool {
	p := s.player
	if p == nil {
		return false
	}
	return h.teleportTo(s, p.HomeMap, p.HomeX, p.HomeY, p.HomeZ, p.Orientation)
}

// teleportAck handles MSG_MOVE_TELEPORT_ACK (HandleMoveTeleportAckOpcode).
func (h *Handler) teleportAckHandler(_ context.Context, s *Session, pk world.Packet) (*world.Packet, error) {
	r := wire.NewReader(pk.Payload)
	guid := r.Uint64()
	r.Uint32() // counter
	r.Uint32() // client time
	if err := r.Finish(); err != nil {
		return nil, err
	}
	p := s.player
	if p == nil || !s.tele.near || guid != uint64(p.GUID) {
		return nil, nil
	}
	t := s.tele
	s.tele = teleportState{}
	p.X, p.Y, p.Z, p.Orientation = t.x, t.y, t.z, t.o
	if h.Broadcast != nil {
		_ = h.Broadcast.UpdatePosition(p.GUID, p.Map, p.Zone, p.X, p.Y, p.Z, p.Orientation)
	}
	h.refreshVisibility(s)
	return nil, nil
}

// worldportAck handles MSG_MOVE_WORLDPORT_ACK (HandleMoveWorldportAckOpcode):
// the client finished loading the new map, so the server repeats the
// map-dependent login packets and adds the player to the new map.
func (h *Handler) worldportAck(_ context.Context, s *Session, pk world.Packet) (*world.Packet, error) {
	p := s.player
	if p == nil || !s.tele.far {
		return nil, nil
	}
	s.tele = teleportState{}
	now := time.Now()
	h.snapshotSpellState(s, p, now)
	packets, err := codec.WorldportPackets(p, h.Data, now)
	if err != nil {
		s.log.Warn("worldport packets failed", "error", err)
		return nil, err
	}
	if err = s.send(packets...); err != nil {
		return nil, err
	}
	// The client created a fresh player object: aura words must be sent again.
	h.sendAuraState(s)
	h.registerInWorld(s, p)
	s.syncCounter = 0
	s.syncPending = map[uint32]time.Time{}
	if e := h.sendTimeSync(s); e != nil {
		s.nextSync = now.Add(5 * time.Second)
	}
	return nil, nil
}

func teleportAck(guid uint64, x, y, z, o float32) world.Packet {
	w := wire.NewWriter(48)
	w.PackedGUID(guid)
	w.Uint32(0) // counter
	// MovementInfo with cleared flags at the destination.
	w.Uint32(0)
	w.Uint8(0)
	w.Uint32(uint32(time.Now().UnixMilli()))
	w.Float32(x)
	w.Float32(y)
	w.Float32(z)
	w.Float32(o)
	w.Uint32(0) // fall time
	b, _ := w.Result()
	return world.Packet{Opcode: op.MSG_MOVE_TELEPORT_ACK, Payload: b}
}

func transferPending(mapID uint32) world.Packet {
	w := wire.NewWriter(4)
	w.Uint32(mapID)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_TRANSFER_PENDING, Payload: b}
}

func newWorld(mapID uint32, x, y, z, o float32) world.Packet {
	w := wire.NewWriter(20)
	w.Uint32(mapID)
	w.Float32(x)
	w.Float32(y)
	w.Float32(z)
	w.Float32(o)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_NEW_WORLD, Payload: b}
}
