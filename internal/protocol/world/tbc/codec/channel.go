package codec

import (
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

// ChannelNotify types from Channel.h CHAT_*_NOTICE.
const (
	ChannelNotifyJoined              = 0x00
	ChannelNotifyLeft                = 0x01
	ChannelNotifyYouJoined           = 0x02
	ChannelNotifyYouLeft             = 0x03
	ChannelNotifyWrongPass           = 0x04
	ChannelNotifyNotMember           = 0x05
	ChannelNotifyNotModerator        = 0x06
	ChannelNotifyPasswordChanged     = 0x07
	ChannelNotifyOwnerChanged        = 0x08
	ChannelNotifyPlayerNotFound      = 0x09
	ChannelNotifyNotOwner            = 0x0A
	ChannelNotifyChannelOwner        = 0x0B
	ChannelNotifyModeChange          = 0x0C
	ChannelNotifyAnnouncementsOn     = 0x0D
	ChannelNotifyAnnouncementsOff    = 0x0E
	ChannelNotifyModerationOn        = 0x0F
	ChannelNotifyModerationOff       = 0x10
	ChannelNotifyMuted               = 0x11
	ChannelNotifyPlayerKicked        = 0x12
	ChannelNotifyBanned              = 0x13
	ChannelNotifyPlayerBanned        = 0x14
	ChannelNotifyPlayerUnbanned      = 0x15
	ChannelNotifyPlayerNotBanned     = 0x16
	ChannelNotifyPlayerAlreadyMember = 0x17
	ChannelNotifyInvite              = 0x18
	ChannelNotifyInviteWrongFaction  = 0x19
	ChannelNotifyWrongFaction        = 0x1A
	ChannelNotifyInvalidName         = 0x1B
)

// ChannelNotify builds SMSG_CHANNEL_NOTIFY.
//
// Reference: Channel::MakeNotifyPacket:
//
//	uint8 notifyType
//	CString channelName
//	(optional payload depends on notifyType)
//
// For YouJoined (0x02), the payload is:
//
//	uint8 flags, uint32 channelId, uint32(0)
//
// For Joined (0x00) / Left (0x01), the payload is:
//
//	uint64 guid
//
// For YouLeft (0x03), the payload is:
//
//	uint32 channelId
//	uint8  0 (the reference writes 0; the comment says 0x00 or 0x01)
func ChannelNotify(notifyType uint8, channelName string) world.Packet {
	w := wire.NewWriter(64)
	w.Uint8(notifyType)
	w.CString(channelName, 128)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_CHANNEL_NOTIFY, Payload: b}
}

// ChannelNotifyYouJoined builds the YOU_JOINED notify with the extra
// flags/channelId/0 fields. channelId is 0 for custom channels.
func BuildChannelNotifyYouJoined(channelName string, flags uint8, channelId uint32) world.Packet {
	w := wire.NewWriter(64)
	w.Uint8(ChannelNotifyYouJoined)
	w.CString(channelName, 128)
	w.Uint8(flags)
	w.Uint32(channelId)
	w.Uint32(0)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_CHANNEL_NOTIFY, Payload: b}
}

// ChannelNotifyYouLeft builds the YOU_LEFT notify. Reference
// Channel::MakeYouLeft appends the channelId and a trailing uint8 after
// the channel name; without that byte the packet is one byte short and the
// client may drop the whole notify.
func BuildChannelNotifyYouLeft(channelName string, channelId uint32) world.Packet {
	w := wire.NewWriter(64)
	w.Uint8(ChannelNotifyYouLeft)
	w.CString(channelName, 128)
	w.Uint32(channelId)
	w.Uint8(0) // can be 0x00 and 0x01 in the reference
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_CHANNEL_NOTIFY, Payload: b}
}

// ChannelNotifyJoined builds the JOINED notify (someone joined) with the
// joining player's GUID.
func BuildChannelNotifyJoined(channelName string, guid uint32) world.Packet {
	w := wire.NewWriter(64)
	w.Uint8(ChannelNotifyJoined)
	w.CString(channelName, 128)
	w.Uint64(uint64(guid))
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_CHANNEL_NOTIFY, Payload: b}
}

// ChannelNotifyLeft builds the LEFT notify (someone left) with the
// leaving player's GUID.
func BuildChannelNotifyLeft(channelName string, guid uint32) world.Packet {
	w := wire.NewWriter(64)
	w.Uint8(ChannelNotifyLeft)
	w.CString(channelName, 128)
	w.Uint64(uint64(guid))
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_CHANNEL_NOTIFY, Payload: b}
}

// DecodeJoinChannel parses CMSG_JOIN_CHANNEL.
// Reference: HandleJoinChannelOpcode:
//
//	uint32 channelId, uint8 unk1, uint8 unk2, CString name, CString pass
func DecodeJoinChannel(payload []byte) (channelName, password string, channelId uint32, err error) {
	r := wire.NewReader(payload)
	channelId = r.Uint32()
	r.Uint8() // unk1
	r.Uint8() // unk2
	channelName = r.CString(128)
	password = r.CString(128)
	return channelName, password, channelId, r.Finish()
}

// DecodeLeaveChannel parses CMSG_LEAVE_CHANNEL.
// Reference: HandleLeaveChannelOpcode:
//
//	uint32 unk, CString name
func DecodeLeaveChannel(payload []byte) (channelName string, err error) {
	r := wire.NewReader(payload)
	r.Uint32() // unk
	channelName = r.CString(128)
	return channelName, r.Finish()
}
