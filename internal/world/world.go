// Package world implements the per-realm simulation registry and the
// broadcast primitives that route packets between in-world players.
//
// The simulation owns a map of in-world players keyed by character GUID.
// Each entry carries the player's current presence (map, zone, position,
// team) and a Sender (the session's packet delivery closure).
//
// Broadcast primitives mirror MaNGOS's WorldObject::SendMessageToSet
// family: they collect targets under a short-lived read lock, then send
// outside the lock so a slow destination cannot stall the broadcast loop.
// Sends use the non-blocking TrySend path; a full queue drops the packet
// rather than blocking.
//
// This package must not import protocol, storage, application or app;
// the domain/world interfaces are the only dependency.
package world

import (
	"errors"
	"sync"

	domainworld "wowserver/internal/domain/world"
)

// Errors returned by the registry.
var (
	ErrPlayerNotFound = errors.New("player not in world")
	ErrPlayerExists   = errors.New("player already in world")
)

// DefaultVisibilityRange matches MaNGOS DEFAULT_VISIBILITY_DISTANCE
// (90 yards on continents). Used when Broadcast is called without an
// explicit radius. The reference's MAX_VISIBILITY_DISTANCE (333 yards)
// is used for "far" notifications; callers that need it pass an
// explicit radius to BroadcastInRange.
const DefaultVisibilityRange float32 = 90.0

type entry struct {
	presence domainworld.PlayerPresence
	sender   domainworld.Sender
	channels map[string]bool // channel names this player is in
}

// World is the per-realm simulation registry. It owns the list of
// players currently in-world and implements domain/world.Broadcaster.
// It also owns the chat channel registry.
//
// One World instance is shared by all connections of a realm; it is
// constructed at worldd startup and passed to each Handler via the
// app composition root.
type World struct {
	mu       sync.RWMutex
	players  map[uint32]*entry
	channels map[string]*Channel // channel name -> Channel
}

// New constructs an empty World.
func New() *World {
	return &World{players: make(map[uint32]*entry), channels: make(map[string]*Channel)}
}

// Register adds a player to the simulation. Returns ErrPlayerExists if
// the GUID is already in-world. The Sender is stored and called from
// broadcast primitives until Unregister is called.
func (w *World) Register(p domainworld.PlayerPresence, s domainworld.Sender) error {
	if p.GUID == 0 || s == nil {
		return errors.New("invalid world player registration")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.players[p.GUID]; ok {
		return ErrPlayerExists
	}
	w.players[p.GUID] = &entry{presence: p, sender: s}
	return nil
}

// Unregister removes a player. Idempotent.
func (w *World) Unregister(guid uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.players, guid)
}

// UpdatePosition refreshes a player's map/zone/position. Called by the
// session loop after applying local movement. Returns ErrPlayerNotFound
// if the player is not registered.
func (w *World) UpdatePosition(guid uint32, mapID, zone uint32, x, y, z, o float32) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	e, ok := w.players[guid]
	if !ok {
		return ErrPlayerNotFound
	}
	e.presence.Map, e.presence.Zone = mapID, zone
	e.presence.X, e.presence.Y, e.presence.Z, e.presence.Orientation = x, y, z, o
	return nil
}

// Get returns a snapshot of a player's presence and a bool indicating
// whether the player is in-world. Not part of the Broadcaster interface;
// used by diagnostics and future group/guild systems.
func (w *World) Get(guid uint32) (domainworld.PlayerPresence, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	e, ok := w.players[guid]
	if !ok {
		return domainworld.PlayerPresence{}, false
	}
	return e.presence, true
}

// All returns a snapshot of every in-world player's presence. Used by
// whisper name lookup. Not part of the Broadcaster interface.
func (w *World) All() []domainworld.PlayerPresence {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]domainworld.PlayerPresence, 0, len(w.players))
	for _, e := range w.players {
		out = append(out, e.presence)
	}
	return out
}

// Nearby returns the presences of all players on the same map as srcGUID
// within DefaultVisibilityRange. The source player is excluded.
//
// Called by the protocol layer when a player enters the world so the
// new player can receive CREATE_OBJECT spawn packets for existing
// nearby players.
func (w *World) Nearby(srcGUID uint32) []domainworld.PlayerPresence {
	w.mu.RLock()
	defer w.mu.RUnlock()
	src, ok := w.players[srcGUID]
	if !ok {
		return nil
	}
	radius2 := DefaultVisibilityRange * DefaultVisibilityRange
	var out []domainworld.PlayerPresence
	for guid, e := range w.players {
		if guid == srcGUID {
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
		out = append(out, e.presence)
	}
	return out
}

// Count returns the number of in-world players. For diagnostics.
func (w *World) Count() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.players)
}

// --- Channel management ---

// JoinChannel adds a player to the named channel, creating the channel if
// it does not exist. Returns the channel (so the caller can send notify
// packets) and the list of existing member senders (so the caller can
// broadcast the "player joined" notify).
func (w *World) JoinChannel(name string, guid uint32, sender domainworld.Sender) (*Channel, []domainworld.Sender, bool) {
	w.mu.Lock()
	ch, ok := w.channels[name]
	if !ok {
		ch = newChannel(name, 0)
		w.channels[name] = ch
	}
	w.mu.Unlock()
	// Track channel membership on the player entry so Unregister can clean up.
	w.mu.RLock()
	if e, ok := w.players[guid]; ok {
		if e.channels == nil {
			e.channels = make(map[string]bool)
		}
		e.channels[name] = true
	}
	w.mu.RUnlock()
	_, existing := ch.Members()
	wasNew := ch.Join(guid, sender)
	return ch, existing, wasNew
}

// LeaveChannel removes a player from the named channel. Returns the
// channel (may be nil if it was destroyed) and the remaining member
// senders (for the "player left" notify).
func (w *World) LeaveChannel(name string, guid uint32) (*Channel, []domainworld.Sender) {
	w.mu.Lock()
	ch, ok := w.channels[name]
	w.mu.Unlock()
	if !ok {
		return nil, nil
	}
	remaining := ch.Leave(guid)
	// Remove channel membership from the player entry.
	w.mu.RLock()
	if e, ok := w.players[guid]; ok && e.channels != nil {
		delete(e.channels, name)
	}
	w.mu.RUnlock()
	// Destroy empty channels.
	if ch.Empty() {
		w.mu.Lock()
		delete(w.channels, name)
		w.mu.Unlock()
		return nil, remaining
	}
	return ch, remaining
}

// GetChannel returns the channel by name, or nil if it does not exist.
func (w *World) GetChannel(name string) *Channel {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.channels[name]
}

// PlayerChannels returns the names of channels a player is in.
// Used by Unregister to clean up channel membership on logout.
func (w *World) PlayerChannels(guid uint32) []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	e, ok := w.players[guid]
	if !ok || e.channels == nil {
		return nil
	}
	out := make([]string, 0, len(e.channels))
	for name := range e.channels {
		out = append(out, name)
	}
	return out
}
