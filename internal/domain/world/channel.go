package world

// Channel registry contract. The concrete implementation lives in
// internal/world; the interface lives here so the protocol layer can
// depend on the contract without importing the simulation.
//
// The methods intentionally mirror the reference chat-channel behaviour
// (Channel::Join/Leave/Say in Channel.cpp): joining returns the channel
// plus the senders of the members that were already present; leaving
// returns the channel (nil when it was destroyed because the last member
// left) plus the remaining member senders so the caller can broadcast
// the LEFT notice.

// ChannelInfo is a read-only view of one chat channel.
type ChannelInfo interface {
	// Name returns the channel name as sent by the client.
	Name() string
	// Flags returns the channel flags byte sent in YOU_JOINED.
	Flags() uint8
	// Count returns the number of current members.
	Count() int
	// IsMember reports whether the GUID is currently in the channel.
	IsMember(guid uint32) bool
	// IsMuted reports whether the member is muted.
	IsMuted(guid uint32) bool
	// Empty reports whether the channel has no members.
	Empty() bool
	// Members returns all member GUIDs and their senders.
	Members() ([]uint32, []Sender)
}

// ChannelRegistry is the channel side of the world simulation. It is
// satisfied by the same concrete type that implements Broadcaster; the
// protocol layer type-asserts its Broadcaster to this interface.
//
// All methods are safe for concurrent use.
type ChannelRegistry interface {
	// JoinChannel adds a player to the named channel, creating the channel
	// if needed. It returns the channel, the senders of the members that
	// were already present (for the JOINED notice) and whether the joiner
	// is new to the channel.
	JoinChannel(name string, guid uint32, sender Sender) (ChannelInfo, []Sender, bool)
	// LeaveChannel removes a player from the named channel. It returns the
	// channel (nil when the channel was destroyed because it became empty)
	// and the remaining member senders (for the LEFT notice).
	LeaveChannel(name string, guid uint32) (ChannelInfo, []Sender)
	// GetChannel returns the named channel or nil when it does not exist.
	GetChannel(name string) ChannelInfo
	// PlayerChannels returns the names of every channel the player is in.
	// Used by the logout cleanup (reference: Player::CleanupChannels).
	PlayerChannels(guid uint32) []string
}
