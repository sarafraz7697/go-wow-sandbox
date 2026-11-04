package world

import (
	domainworld "wowserver/internal/domain/world"
)

// Broadcast delivers a packet to all players who can see the source
// (same map, within DefaultVisibilityRange). If includeSelf is true,
// the source also receives the packet.
//
// Equivalent to MaNGOS WorldObject::SendMessageToSet.
func (w *World) Broadcast(srcGUID uint32, p domainworld.Packet, includeSelf bool) {
	w.broadcastInRange(srcGUID, DefaultVisibilityRange, p, includeSelf)
}

// BroadcastInRange is like Broadcast but with an explicit radius in
// yards. Equivalent to MaNGOS WorldObject::SendMessageToSetInRange.
func (w *World) BroadcastInRange(srcGUID uint32, radius float32, p domainworld.Packet, includeSelf bool) {
	w.broadcastInRange(srcGUID, radius, p, includeSelf)
}

func (w *World) broadcastInRange(srcGUID uint32, radius float32, p domainworld.Packet, includeSelf bool) {
	targets := w.collectInRange(srcGUID, radius, includeSelf)
	for _, t := range targets {
		t.TrySend(p)
	}
}

// collectInRange returns the senders of all players on the same map as
// srcGUID whose position is within radius of the source. The lock is
// held only for the collection; sends happen outside the lock.
func (w *World) collectInRange(srcGUID uint32, radius float32, includeSelf bool) []domainworld.Sender {
	w.mu.RLock()
	defer w.mu.RUnlock()
	src, ok := w.players[srcGUID]
	if !ok {
		return nil
	}
	radius2 := radius * radius
	var out []domainworld.Sender
	for guid, e := range w.players {
		if !includeSelf && guid == srcGUID {
			continue
		}
		if e.presence.Map != src.presence.Map {
			continue
		}
		dx := e.presence.X - src.presence.X
		dy := e.presence.Y - src.presence.Y
		dz := e.presence.Z - src.presence.Z
		if dx*dx+dy*dy+dz*dz > radius2 {
			continue
		}
		out = append(out, e.sender)
	}
	return out
}

// SendTo delivers a packet to a single player by GUID. Used for whisper,
// direct spell effects, and query responses. Returns ErrPlayerNotFound
// if the player is not in-world.
//
// Unlike the broadcast methods, SendTo uses the blocking Send path
// (with the session's own timeout) so the caller knows whether the
// packet was actually queued.
func (w *World) SendTo(dstGUID uint32, p domainworld.Packet) error {
	w.mu.RLock()
	e, ok := w.players[dstGUID]
	w.mu.RUnlock()
	if !ok {
		return ErrPlayerNotFound
	}
	return e.sender.Send(p)
}

// BroadcastArea delivers to every player on the given map whose position
// is within radius of (x, y, z). If radius is 0, every player on the
// map receives the packet. Used for area-wide effects.
func (w *World) BroadcastArea(mapID uint32, x, y, z, radius float32, p domainworld.Packet) {
	targets := w.collectArea(mapID, x, y, z, radius)
	for _, t := range targets {
		t.TrySend(p)
	}
}

func (w *World) collectArea(mapID uint32, x, y, z, radius float32) []domainworld.Sender {
	w.mu.RLock()
	defer w.mu.RUnlock()
	radius2 := radius * radius
	var out []domainworld.Sender
	for _, e := range w.players {
		if e.presence.Map != mapID {
			continue
		}
		if radius > 0 {
			dx := e.presence.X - x
			dy := e.presence.Y - y
			dz := e.presence.Z - z
			if dx*dx+dy*dy+dz*dz > radius2 {
				continue
			}
		}
		out = append(out, e.sender)
	}
	return out
}

// BroadcastZone delivers to every player in a specific zone, regardless
// of exact position. Used for zone-wide messages.
func (w *World) BroadcastZone(mapID, zoneID uint32, p domainworld.Packet) {
	w.mu.RLock()
	var targets []domainworld.Sender
	for _, e := range w.players {
		if e.presence.Map == mapID && e.presence.Zone == zoneID {
			targets = append(targets, e.sender)
		}
	}
	w.mu.RUnlock()
	for _, t := range targets {
		t.TrySend(p)
	}
}

// BroadcastAll delivers to every in-world player on the realm.
func (w *World) BroadcastAll(p domainworld.Packet) {
	w.mu.RLock()
	targets := make([]domainworld.Sender, 0, len(w.players))
	for _, e := range w.players {
		targets = append(targets, e.sender)
	}
	w.mu.RUnlock()
	for _, t := range targets {
		t.TrySend(p)
	}
}
