// Package world hosts the per-realm simulation: the player registry,
// broadcast primitives, and chat channels.
//
// Channels are simple name→member-set maps. They are not persistent (no DB)
// and not cross-realm. A channel is created on first join and destroyed when
// the last member leaves. Members receive SMSG_CHANNEL_NOTIFY on join/leave
// and SMSG_MESSAGECHAT (CHAT_MSG_CHANNEL) on Say.
package world

import (
	"sync"

	domainworld "wowserver/internal/domain/world"
)

// ChannelFlags from Channel.h. We only set a few that the client cares
// about for the built-in channels.
const (
	ChannelFlagGeneral = 0x10
	ChannelFlagCity    = 0x20
	ChannelFlagTrade   = 0x04
	ChannelFlagLFG     = 0x40
)

// ChannelNotifyType from Channel.h CHAT_*_NOTICE values.
const (
	NotifyJoined      = 0x00
	NotifyLeft        = 0x01
	NotifyYouJoined   = 0x02
	NotifyYouLeft     = 0x03
	NotifyWrongPass   = 0x04
	NotifyNotMember   = 0x05
	NotifyPlayerFound = 0x09
)

// ChannelMember is a player in a channel. We store the GUID and the Sender
// (for direct delivery) plus member flags (owner/moderator/muted).
type channelMember struct {
	sender domainworld.Sender
	flags  uint8
}

const (
	memberOwner     = 0x01
	memberModerator = 0x02
	memberMuted     = 0x08
)

// Channel is a chat channel with a name and a set of members.
type Channel struct {
	name    string
	flags   uint8
	mu      sync.RWMutex
	members map[uint32]*channelMember
}

// newChannel creates an empty channel with the given name and flags.
func newChannel(name string, flags uint8) *Channel {
	return &Channel{
		name:    name,
		flags:   flags,
		members: make(map[uint32]*channelMember),
	}
}

// Join adds a player to the channel. Returns true if this is the first
// member (the joiner becomes owner). The caller is responsible for sending
// SMSG_CHANNEL_NOTIFY to the joiner and existing members.
func (c *Channel) Join(guid uint32, s domainworld.Sender) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, exists := c.members[guid]
	m := &channelMember{sender: s}
	if len(c.members) == 0 {
		m.flags |= memberOwner
	}
	c.members[guid] = m
	return !exists
}

// Leave removes a player from the channel. Returns the list of remaining
// member senders (for the "player left" notify broadcast).
func (c *Channel) Leave(guid uint32) []domainworld.Sender {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.members, guid)
	out := make([]domainworld.Sender, 0, len(c.members))
	for _, m := range c.members {
		out = append(out, m.sender)
	}
	return out
}

// Members returns all member GUIDs and senders.
func (c *Channel) Members() ([]uint32, []domainworld.Sender) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	guids := make([]uint32, 0, len(c.members))
	senders := make([]domainworld.Sender, 0, len(c.members))
	for guid, m := range c.members {
		guids = append(guids, guid)
		senders = append(senders, m.sender)
	}
	return guids, senders
}

// IsMember returns true if guid is in the channel.
func (c *Channel) IsMember(guid uint32) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.members[guid]
	return ok
}

// IsMuted returns true if the member is muted.
func (c *Channel) IsMuted(guid uint32) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.members[guid]
	return ok && m.flags&memberMuted != 0
}

// Count returns the number of members.
func (c *Channel) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.members)
}

// Empty returns true if the channel has no members.
func (c *Channel) Empty() bool {
	return c.Count() == 0
}

// Name returns the channel name.
func (c *Channel) Name() string { return c.name }

// Flags returns the channel flags.
func (c *Channel) Flags() uint8 { return c.flags }
