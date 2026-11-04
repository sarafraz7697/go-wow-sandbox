package tbc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	domainworld "wowserver/internal/domain/world"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

// handleMessageChat decodes CMSG_MESSAGECHAT and produces the chat packets
// the player sees. Without a world loop there are no other players to
// broadcast to, so SAY/YELL/EMOTE only echo to the sender. WHISPER looks up
// the target by name (no other-player delivery yet; we return "player not
// found" for non-self).
//
// Reference: ChatHandler.cpp HandleMessagechatOpcode + ChatHandler::BuildChatPacket.
func (h *Handler) handleMessageChat(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	if s.player == nil {
		return nil, nil
	}
	req, err := codec.DecodeChatRequest(p.Payload)
	if err != nil {
		return nil, err
	}
	if req.Type >= codec.MaxChatMessageType {
		return nil, codec.ErrWorldPacket
	}
	// Enforce the 255-byte message limit the reference applies in CheckChatMessage.
	if len(req.Message) == 0 {
		return nil, nil
	}
	if utf8.RuneCountInString(req.Message) > 255 {
		// Reference truncates rather than disconnects; do the same.
		req.Message = string([]rune(req.Message)[:255])
	}

	log := s.log
	switch req.Type {
	case codec.ChatMsgSay, codec.ChatMsgYell, codec.ChatMsgEmote:
		return h.handleSayYellEmote(s, req, log)
	case codec.ChatMsgWhisper:
		return h.handleWhisper(s, req, log)
	case codec.ChatMsgParty, codec.ChatMsgGuild, codec.ChatMsgOfficer,
		codec.ChatMsgRaid, codec.ChatMsgRaidLeader, codec.ChatMsgRaidWarning,
		codec.ChatMsgBattleground, codec.ChatMsgBattlegroundLead:
		// No group/guild/raid/bg system yet. Echo to self so the client
		// shows the message instead of hanging on a pending chat state.
		return h.echoSelf(s, req, log)
	case codec.ChatMsgChannel:
		// No channel system yet. Echo to self.
		return h.echoSelf(s, req, log)
	case codec.ChatMsgAFK, codec.ChatMsgDND:
		// AFK/DND toggle. No persistence yet; just accept and echo.
		return h.echoSelf(s, req, log)
	default:
		// Reference logs an error for unknown types; we reject.
		return nil, codec.ErrWorldPacket
	}
}

// handleSayYellEmote broadcasts the message to nearby players (including
// the sender). SAY has a ~25-yard radius; YELL has ~50 yards; EMOTE uses
// the default visibility range. Mirrors MaNGOS Player::Say/Yell/TextEmote
// which call SendMessageToSetInRange with the appropriate distance.
func (h *Handler) handleSayYellEmote(s *Session, req codec.ChatRequest, log *slog.Logger) (*world.Packet, error) {
	pkt, err := codec.BuildChatPacket(req.Type, req.Language, uint64(s.player.GUID), s.player.Name, 0, req.Message)
	if err != nil {
		return nil, err
	}
	log.Debug("chat message", "type", req.Type, "guid", s.player.GUID, "bytes", len(req.Message))
	if h.Broadcast != nil {
		radius := float32(25) // SAY range (CONFIG_FLOAT_LISTEN_RANGE_SAY)
		if req.Type == codec.ChatMsgYell {
			radius = 50 // YELL range
		}
		h.Broadcast.BroadcastInRange(s.player.GUID, radius,
			domainworld.Packet{Opcode: pkt.Opcode, Payload: pkt.Payload}, true)
	} else {
		_ = s.send(pkt)
	}
	return nil, nil
}

// handleWhisper attempts to deliver a whisper. Without a world registry of
// online players, we can only echo back a WHISPER_INFORM if the target is
// the sender (self-whisper) or return a "player not found" notice.
func (h *Handler) handleWhisper(s *Session, req codec.ChatRequest, log *slog.Logger) (*world.Packet, error) {
	target := strings.TrimSpace(req.TargetName)
	if target == "" {
		return h.playerNotFound(s, req.TargetName)
	}
	// No other-player lookup yet. If the target name matches the sender,
	// echo as WHISPER_INFORM. Otherwise notify the sender the player was
	// not found.
	if strings.EqualFold(target, s.player.Name) {
		inform, err := codec.BuildWhisperInform(uint64(s.player.GUID), req.Message)
		if err != nil {
			return nil, err
		}
		_ = s.send(inform)
		return nil, nil
	}
	return h.playerNotFound(s, target)
}

func (h *Handler) playerNotFound(s *Session, name string) (*world.Packet, error) {
	// SMSG_CHAT_PLAYER_NOT_FOUND: CString name
	w := codec.NewNameNotFoundWriter(name)
	_ = s.send(w)
	return nil, nil
}

// echoSelf sends the message back to the sender with the original type. This
// lets the client render the message in the correct channel tab even though
// no other player received it.
func (h *Handler) echoSelf(s *Session, req codec.ChatRequest, log *slog.Logger) (*world.Packet, error) {
	pkt, err := codec.BuildChatPacket(req.Type, req.Language, uint64(s.player.GUID), s.player.Name, 0, req.Message)
	if err != nil {
		return nil, err
	}
	log.Debug("chat echo", "type", req.Type, "guid", s.player.GUID)
	_ = s.send(pkt)
	return nil, nil
}

// handleTextEmote decodes CMSG_TEXT_EMOTE and echoes SMSG_TEXT_EMOTE to the
// sender. Reference: HandleTextEmoteOpcode + EmoteChatBuilder. Without
// EmotesText.dbc we cannot validate the emote entry; we accept any value
// and let the client render it.
func (h *Handler) handleTextEmote(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	if s.player == nil {
		return nil, nil
	}
	req, err := codec.DecodeTextEmoteRequest(p.Payload)
	if err != nil {
		return nil, err
	}
	pkt, err := codec.BuildTextEmote(uint64(s.player.GUID), req.TextEmote, req.EmoteNum, "")
	if err != nil {
		return nil, err
	}
	_ = s.send(pkt)
	return nil, nil
}

// handleMessagechatIgnored handles CMSG_CHAT_IGNORED (sent when a player
// ignores a whisper). Reference: HandleChatIgnoredOpcode. Without a
// multi-player registry this is a no-op consume.
func (h *Handler) handleChatIgnored(_ context.Context, _ *Session, p world.Packet) (*world.Packet, error) {
	// uint64 guid + uint8 unk
	if len(p.Payload) != 9 {
		return nil, codec.ErrWorldPacket
	}
	return nil, nil
}

// registerChatRoutes wires the chat opcodes. All require an in-world player
// (StateInWorld or StateLoggingOut). CMSG_MESSAGECHAT is the main chat
// entry point; CMSG_TEXT_EMOTE is /emote; CMSG_CHAT_IGNORED is the
// whisper-ignore reply.
func (h *Handler) registerChatRoutes(router *Router) {
	inWorld := []State{StateInWorld, StateLoggingOut}
	add := func(opcode uint32, fn handlerFunc) {
		router.routes[opcode] = route{inWorld, fn}
	}
	add(op.CMSG_MESSAGECHAT, h.handleMessageChat)
	add(op.CMSG_TEXT_EMOTE, h.handleTextEmote)
	add(op.CMSG_CHAT_IGNORED, h.handleChatIgnored)
}

// Suppress unused-import warning for fmt; will be used when we add
// notification messages in a follow-up.
var _ = fmt.Sprintf
