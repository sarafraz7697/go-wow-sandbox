package world

// Packet is the unit of communication between the world simulation and
// its subscribers. It carries only the opcode (a 32-bit identifier) and
// the payload (raw bytes). The protocol layer converts between its own
// packet type and this one at the broadcast boundary.
//
// This is a pure value — it is not a wire format and has no I/O. The
// framing, encryption and byte order belong to the protocol layer.
type Packet struct {
	Opcode  uint32
	Payload []byte
}

// Sender delivers packets to one session's writer goroutine. The world
// simulation holds a Sender for each in-world player and calls it from
// the broadcast primitives.
//
//   - Send is used for direct (1-to-1) delivery. It may block until the
//     packet is queued or the session's send timeout expires. Use this
//     for query responses, whisper, and other point-to-point traffic
//     where the caller can afford to wait.
//   - TrySend is used by broadcasts. It must never block — it returns
//     false if the packet was dropped because the destination's queue
//     is full. A slow destination must not stall the broadcast loop.
//
// Both methods must be safe for concurrent use.
type Sender interface {
	Send(...Packet) error
	TrySend(Packet) bool
}

// PlayerPresence is the world simulation's view of an in-world player.
// It carries all the fields the protocol layer needs to build a
// CREATE_OBJECT update block for observers: identity, position, and
// the visible unit/player fields the client renders.
//
// This is a snapshot — the simulation owns a mutable copy that it
// refreshes via Broadcaster.UpdatePosition as the player moves.
type PlayerPresence struct {
	GUID      uint32
	AccountID uint64
	Name      string
	// Team is 7 (Alliance) or 1 (Horde), from ChrRaces.BaseLanguage.
	// Zero means unknown; faction-filtered broadcasts skip the player.
	Team uint32
	// FactionTemplate is ChrRaces.FactionID (Player::setFactionForRace):
	// the UNIT_FIELD_FACTIONTEMPLATE observers receive so their client can
	// resolve faction hostility (Alliance vs Horde) for this player.
	FactionTemplate uint32
	Race            uint8
	Class uint8
	// Map and Zone are the current map id and area/zone id.
	Map  uint32
	Zone uint32
	// Position is the current map-local pose.
	X, Y, Z, Orientation float32
	// --- Fields needed for CREATE_OBJECT (observer spawn) ---
	Gender       uint8
	Level        uint8
	Power        uint8 // power type: 0=mana,1=rage,2=focus,3=energy,4=happiness
	Health       uint32
	MaxHealth    uint32
	Mana, Rage   uint32
	MaxMana      uint32
	Energy       uint32
	DisplayID    uint32
	PlayerFlags  uint32
	PlayerBytes  uint32 // skin/face/hairStyle/hairColor
	PlayerBytes2 uint32 // facialHair/...
	// VisibleItems carries the entry ID of each equipped item (slots 0-18).
	// Zero means the slot is empty. The client uses this to render the
	// correct model for each equipment piece.
	VisibleItems [19]uint32
}

// Broadcaster is the broadcast service the protocol layer calls to route
// packets between in-world players. The implementation lives in
// internal/world; the interface lives here so the protocol layer can
// depend on the contract without importing the simulation.
//
// All methods are safe for concurrent use. A nil receiver (or a nil
// interface) means "no broadcast" — callers must nil-check before use.
type Broadcaster interface {
	// Register adds a player to the world simulation. Returns an error
	// if the GUID is already in-world. The Sender is stored and called
	// from broadcast primitives until Unregister is called.
	Register(p PlayerPresence, s Sender) error
	// Unregister removes a player from the world simulation. Idempotent.
	Unregister(guid uint32)
	// Nearby returns the presences of all players on the same map as
	// srcGUID within the default visibility radius. Used by the protocol
	// layer to build CREATE_OBJECT spawn packets when a player enters
	// the world (so the new player sees existing nearby players).
	//
	// The source player is excluded from the result.
	Nearby(srcGUID uint32) []PlayerPresence
	// UpdatePosition refreshes a player's map/zone/position. Called by
	// the session loop after applying local movement. Returns an error
	// if the player is not registered.
	UpdatePosition(guid uint32, mapID, zone uint32, x, y, z, o float32) error
	// Broadcast delivers a packet to all players who can see the source
	// (same map, within the default visibility radius). If includeSelf
	// is true, the source also receives the packet.
	//
	// Equivalent to MaNGOS WorldObject::SendMessageToSet.
	Broadcast(srcGUID uint32, p Packet, includeSelf bool)
	// BroadcastInRange is like Broadcast but with an explicit radius
	// in yards. Use this for distance-limited effects (spell visals,
	// sounds, emotes).
	//
	// Equivalent to MaNGOS WorldObject::SendMessageToSetInRange.
	BroadcastInRange(srcGUID uint32, radius float32, p Packet, includeSelf bool)
	// SendTo delivers a packet to a single player by GUID. Returns an
	// error if the player is not in-world. Used for whisper, direct
	// spell effects, and query responses.
	SendTo(dstGUID uint32, p Packet) error
	// BroadcastArea delivers to every player on the given map whose
	// position is within radius of (x, y, z). If radius is 0, every
	// player on the map receives the packet. Used for area-wide
	// effects (weather, server announcements, sound effects).
	BroadcastArea(mapID uint32, x, y, z, radius float32, p Packet)
	// BroadcastZone delivers to every player in a specific zone,
	// regardless of exact position. Used for zone-wide messages
	// (zone defense alerts, world PvP announcements).
	BroadcastZone(mapID, zoneID uint32, p Packet)
	// BroadcastAll delivers to every in-world player on the realm.
	// Use sparingly (server-wide announcements, shutdown notices).
	BroadcastAll(p Packet)
}

// NoopBroadcaster is a Broadcaster that does nothing. Use it when the
// simulation is disabled (e.g., in tests) so callers don't need to
// nil-check every call site.
type NoopBroadcaster struct{}

func (NoopBroadcaster) Register(PlayerPresence, Sender) error { return nil }
func (NoopBroadcaster) Unregister(uint32)                     {}
func (NoopBroadcaster) Nearby(uint32) []PlayerPresence        { return nil }
func (NoopBroadcaster) UpdatePosition(uint32, uint32, uint32, float32, float32, float32, float32) error {
	return nil
}
func (NoopBroadcaster) Broadcast(uint32, Packet, bool)                                   {}
func (NoopBroadcaster) BroadcastInRange(uint32, float32, Packet, bool)                   {}
func (NoopBroadcaster) SendTo(uint32, Packet) error                                      { return nil }
func (NoopBroadcaster) BroadcastArea(uint32, float32, float32, float32, float32, Packet) {}
func (NoopBroadcaster) BroadcastZone(uint32, uint32, Packet)                             {}
func (NoopBroadcaster) BroadcastAll(Packet)                                              {}
