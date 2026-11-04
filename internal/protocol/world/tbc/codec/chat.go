package codec

import (
	"errors"

	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

// ChatMsg values from mangosone/server SharedDefines.h ChatMsg enum.
const (
	ChatMsgSystem           = 0x00
	ChatMsgSay              = 0x01
	ChatMsgParty            = 0x02
	ChatMsgRaid             = 0x03
	ChatMsgGuild            = 0x04
	ChatMsgOfficer          = 0x05
	ChatMsgYell             = 0x06
	ChatMsgWhisper          = 0x07
	ChatMsgWhisperInform    = 0x08
	ChatMsgReply            = 0x09
	ChatMsgEmote            = 0x0A
	ChatMsgTextEmote        = 0x0B
	ChatMsgMonsterSay       = 0x0C
	ChatMsgMonsterYell      = 0x0E
	ChatMsgMonsterWhisper   = 0x0F
	ChatMsgMonsterEmote     = 0x10
	ChatMsgChannel          = 0x11
	ChatMsgAFK              = 0x17
	ChatMsgDND              = 0x18
	ChatMsgIgnored          = 0x19
	ChatMsgRaidLeader       = 0x27
	ChatMsgRaidWarning      = 0x28
	ChatMsgBattleground     = 0x2C
	ChatMsgBattlegroundLead = 0x2D
)

// Language values from mangosone/server SharedDefines.h Language enum.
const (
	LangUniversal   = 0
	LangOrcish      = 1
	LangDarnassian  = 2
	LangTaurahe     = 3
	LangDwarvish    = 6
	LangCommon      = 7
	LangDemonic     = 8
	LangTitan       = 9
	LangThalassian  = 10
	LangDraconic    = 11
	LangKalimag     = 12
	LangGnomish     = 13
	LangTroll       = 14
	LangGutterspeak = 33
	LangDraenei     = 35
	LangAddon       = 0xFFFFFFFF
)

// MaxChatMessageType from SharedDefines.h (#define MAX_CHAT_MSG_TYPE 0x2F).
const MaxChatMessageType = 0x2F

// ChatTagFlags from Chat.h ChatTagFlags. We only need CHAT_TAG_NONE (0)
// for now; GM/AFK/DND tags are future work.
const ChatTagNone = 0

// ErrChatTooLong is returned when a chat message exceeds the 255-byte limit
// the reference enforces in CheckChatMessage.
var ErrChatTooLong = errors.New("chat message exceeds 255 bytes")

// ChatRequest is the decoded CMSG_MESSAGECHAT payload.
//
// Reference: ChatHandler.cpp HandleMessagechatOpcode.
//   - uint32 type (ChatMsg)
//   - uint32 language (Language)
//   - For SAY/YELL/EMOTE: message (CString)
//   - For WHISPER: targetName (CString) + message (CString)
//   - For CHANNEL: channelName (CString) + message (CString)
//   - For PARTY/GUILD/etc: message (CString)
type ChatRequest struct {
	Type        uint8
	Language    uint32
	Message     string
	TargetName  string // WHISPER only
	ChannelName string // CHANNEL only
}

// DecodeChatRequest parses CMSG_MESSAGECHAT. The type determines which
// trailing fields are present; unknown types return an error.
// Reference: HandleMessagechatOpcode reads type and lang both as uint32.
func DecodeChatRequest(payload []byte) (ChatRequest, error) {
	r := wire.NewReader(payload)
	rawType := r.Uint32()
	req := ChatRequest{
		Type:     uint8(rawType),
		Language: r.Uint32(),
	}
	if r.Err() != nil {
		return req, r.Err()
	}
	if rawType >= MaxChatMessageType {
		return req, ErrWorldPacket
	}
	switch req.Type {
	case ChatMsgSay, ChatMsgYell, ChatMsgEmote, ChatMsgParty, ChatMsgGuild,
		ChatMsgOfficer, ChatMsgRaid, ChatMsgRaidLeader, ChatMsgRaidWarning,
		ChatMsgBattleground, ChatMsgBattlegroundLead, ChatMsgAFK, ChatMsgDND:
		req.Message = r.CString(255)
	case ChatMsgWhisper:
		req.TargetName = r.CString(48)
		req.Message = r.CString(255)
	case ChatMsgChannel:
		req.ChannelName = r.CString(128)
		req.Message = r.CString(255)
	default:
		return req, ErrWorldPacket
	}
	return req, r.Finish()
}

// BuildChatPacket builds SMSG_MESSAGECHAT following the reference
// ChatHandler::BuildChatPacket "default" branch (used for SAY/YELL/PARTY/
// GUILD/WHISPER/CHANNEL/etc).
//
// Wire layout (default branch):
//
//	uint8  msgType
//	uint32 language
//	uint64 senderGUID  (full, not packed)
//	uint32 0           (constant since 2.1.0)
//	if CHANNEL: CString channelName
//	uint64 targetGUID  (full; 0 for self-only SAY/YELL)
//	uint32 messageLen  (strlen+1)
//	CString message
//	uint8  chatTag
//
// For WHISPER_INFORM (sender sees own whisper echo), the sender is the
// receiver and targetGUID is the original target.
// BuildChatPacket builds SMSG_MESSAGECHAT following the reference
// ChatHandler::BuildChatPacket "default" branch (used for SAY/YELL/PARTY/
// GUILD/WHISPER/CHANNEL/etc).
//
// Reference: ChatHandler.cpp BuildChatPacket default branch.
// Wire layout (default branch):
//
//	uint8  msgType
//	uint32 language
//	uint64 senderGUID  (full, not packed — ObjectGuid::operator<<)
//	uint32 0           (constant since 2.1.0)
//	if CHANNEL: CString channelName
//	uint64 targetGUID  (full; 0 for self-only SAY/YELL)
//	uint32 messageLen  (strlen+1, includes null terminator)
//	bytes  message     (CString: message + null)
//	uint8  chatTag
func BuildChatPacket(msgType uint8, language uint32, senderGUID uint64, senderName string, targetGUID uint64, message string) (world.Packet, error) {
	if len(message) > 255 {
		return world.Packet{}, ErrChatTooLong
	}
	w := wire.NewWriter(MaxServerPayload)
	w.Uint8(msgType)
	w.Uint32(language)
	w.Uint64(senderGUID)
	w.Uint32(0)
	// default branch: no channel name (we don't support CHANNEL yet)
	w.Uint64(targetGUID)
	// Reference: data << uint32(strlen(message) + 1); data << message;
	// CString in wire.Writer only writes string+null, so we add the
	// uint32 length prefix explicitly.
	w.Uint32(uint32(len(message) + 1))
	w.CString(message, 255)
	w.Uint8(ChatTagNone)
	b, err := w.Result()
	if err != nil {
		return world.Packet{}, err
	}
	return world.Packet{Opcode: op.SMSG_MESSAGECHAT, Payload: b}, nil
}

// BuildWhisperInform builds SMSG_MESSAGECHAT with CHAT_MSG_WHISPER_INFORM.
// This is the echo the sender sees after sending a whisper, confirming the
// message was sent. targetGUID is the receiver's GUID.
func BuildWhisperInform(targetGUID uint64, message string) (world.Packet, error) {
	return BuildChatPacket(ChatMsgWhisperInform, LangUniversal, 0, "", targetGUID, message)
}

// BuildChannelChatPacket builds SMSG_MESSAGECHAT for a CHANNEL message.
// The CHANNEL branch writes channelName before the targetGUID field.
//
// Reference: ChatHandler::BuildChatPacket CHAT_MSG_CHANNEL branch:
//
//	uint8  msgType
//	uint32 language
//	uint64 senderGUID
//	uint32 0
//	CString channelName
//	uint64 targetGUID (0 for channel)
//	uint32 messageLen
//	CString message
//	uint8  chatTag
func BuildChannelChatPacket(msgType uint8, language uint32, senderGUID uint64, senderName, channelName, message string) (world.Packet, error) {
	if len(message) > 255 {
		return world.Packet{}, ErrChatTooLong
	}
	w := wire.NewWriter(MaxServerPayload)
	w.Uint8(msgType)
	w.Uint32(language)
	w.Uint64(senderGUID)
	w.Uint32(0)
	w.CString(channelName, 128)
	w.Uint64(0) // targetGUID for channel
	w.Uint32(uint32(len(message) + 1))
	w.CString(message, 255)
	w.Uint8(ChatTagNone)
	b, err := w.Result()
	if err != nil {
		return world.Packet{}, err
	}
	return world.Packet{Opcode: op.SMSG_MESSAGECHAT, Payload: b}, nil
}

// BuildSystemMessage builds SMSG_MESSAGECHAT with CHAT_MSG_SYSTEM and
// LANG_UNIVERSAL. Used for server notifications (e.g. "player not found").
func BuildSystemMessage(message string) (world.Packet, error) {
	return BuildChatPacket(ChatMsgSystem, LangUniversal, 0, "", 0, message)
}

// BuildTextEmote builds SMSG_TEXT_EMOTE.
// Reference: EmoteChatBuilder in ChatHandler.cpp.
//
//	uint64 senderGUID
//	uint32 textEmote   (EmotesText.dbc entry)
//	uint32 emoteNum    (animation variation, 0..5)
//	uint32 nameLen     (strlen+1 of target name, or 1 for empty)
//	CString targetName (or single 0 byte)
func BuildTextEmote(senderGUID uint64, textEmote, emoteNum uint32, targetName string) (world.Packet, error) {
	w := wire.NewWriter(64)
	w.Uint64(senderGUID)
	w.Uint32(textEmote)
	w.Uint32(emoteNum)
	if targetName == "" {
		w.Uint32(1)
		w.Uint8(0)
	} else {
		w.CString(targetName, 48)
	}
	b, err := w.Result()
	if err != nil {
		return world.Packet{}, err
	}
	return world.Packet{Opcode: op.SMSG_TEXT_EMOTE, Payload: b}, nil
}

// NewNameNotFoundWriter builds SMSG_CHAT_PLAYER_NOT_FOUND.
// Reference: SendPlayerNotFoundNotice. Layout: CString name.
func NewNameNotFoundWriter(name string) world.Packet {
	w := wire.NewWriter(64)
	w.CString(name, 48)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_CHAT_PLAYER_NOT_FOUND, Payload: b}
}

// DecodeTextEmoteRequest parses CMSG_TEXT_EMOTE.
// Reference: HandleTextEmoteOpcode.
//
//	uint32 textEmote
//	uint32 emoteNum
//	uint64 targetGUID
type TextEmoteRequest struct {
	TextEmote  uint32
	EmoteNum   uint32
	TargetGUID uint64
}

func DecodeTextEmoteRequest(payload []byte) (TextEmoteRequest, error) {
	r := wire.NewReader(payload)
	req := TextEmoteRequest{
		TextEmote:  r.Uint32(),
		EmoteNum:   r.Uint32(),
		TargetGUID: r.Uint64(),
	}
	return req, r.Finish()
}
