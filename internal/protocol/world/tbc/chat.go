package tbc

import (
	"context"
	"log/slog"
	"strings"
	"unicode/utf8"

	domainworld "wowserver/internal/domain/world"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

// handleMessageChat decodes CMSG_MESSAGECHAT and routes to the appropriate
// chat handler. SAY/YELL/EMOTE broadcast to nearby players. WHISPER looks
// up the target by name in the world registry. CHANNEL broadcasts to
// channel members. PARTY/GUILD/RAID echo to self (no group system yet).
//
// Reference: ChatHandler.cpp HandleMessagechatOpcode + BuildChatPacket.
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
	if len(req.Message) == 0 {
		return nil, nil
	}
	if utf8.RuneCountInString(req.Message) > 255 {
		req.Message = string([]rune(req.Message)[:255])
	}

	log := s.log
	switch req.Type {
	case codec.ChatMsgSay, codec.ChatMsgYell, codec.ChatMsgEmote:
		return h.handleSayYellEmote(s, req, log)
	case codec.ChatMsgWhisper:
		return h.handleWhisper(s, req, log)
	case codec.ChatMsgChannel:
		return h.handleChannelSay(s, req, log)
	case codec.ChatMsgParty, codec.ChatMsgGuild, codec.ChatMsgOfficer,
		codec.ChatMsgRaid, codec.ChatMsgRaidLeader, codec.ChatMsgRaidWarning,
		codec.ChatMsgBattleground, codec.ChatMsgBattlegroundLead:
		return h.echoSelf(s, req, log)
	case codec.ChatMsgAFK, codec.ChatMsgDND:
		return h.echoSelf(s, req, log)
	default:
		return nil, codec.ErrWorldPacket
	}
}

// handleSayYellEmote broadcasts the message to nearby players (including
// the sender). SAY ≈ 25 yards; YELL ≈ 50 yards; EMOTE uses default
// visibility (90 yards). Mirrors MaNGOS Player::Say/Yell/TextEmote.
func (h *Handler) handleSayYellEmote(s *Session, req codec.ChatRequest, log *slog.Logger) (*world.Packet, error) {
	pkt, err := codec.BuildChatPacket(req.Type, req.Language, uint64(s.player.GUID), s.player.Name, 0, req.Message)
	if err != nil {
		return nil, err
	}
	log.Debug("chat message", "type", req.Type, "guid", s.player.GUID, "bytes", len(req.Message))
	if h.Broadcast != nil {
		radius := float32(25)
		if req.Type == codec.ChatMsgYell {
			radius = 50
		}
		h.Broadcast.BroadcastInRange(s.player.GUID, radius,
			domainworld.Packet{Opcode: pkt.Opcode, Payload: pkt.Payload}, true)
	} else {
		_ = s.send(pkt)
	}
	return nil, nil
}

// handleWhisper delivers a whisper to the target player by name.
// Looks up the target in the world registry; if found, sends
// SMSG_MESSAGECHAT (WHISPER) to the target and SMSG_MESSAGECHAT
// (WHISPER_INFORM) to the sender. If not found, sends
// SMSG_CHAT_PLAYER_NOT_FOUND.
func (h *Handler) handleWhisper(s *Session, req codec.ChatRequest, log *slog.Logger) (*world.Packet, error) {
	target := strings.TrimSpace(req.TargetName)
	if target == "" {
		return h.playerNotFound(s, req.TargetName)
	}
	// Self-whisper: echo as WHISPER_INFORM.
	if strings.EqualFold(target, s.player.Name) {
		inform, err := codec.BuildWhisperInform(uint64(s.player.GUID), req.Message)
		if err != nil {
			return nil, err
		}
		_ = s.send(inform)
		return nil, nil
	}
	// Look up target in the world registry by iterating in-world players.
	// This is O(n) but n is small for a single-realm educational server.
	if h.Broadcast != nil {
		type allWorld interface {
			All() []domainworld.PlayerPresence
		}
		if aw, ok := h.Broadcast.(allWorld); ok {
			for _, p := range aw.All() {
				if strings.EqualFold(p.Name, target) {
					// Deliver whisper to target.
					whisper, err := codec.BuildChatPacket(codec.ChatMsgWhisper, req.Language, uint64(s.player.GUID), s.player.Name, uint64(p.GUID), req.Message)
					if err != nil {
						return nil, err
					}
					_ = h.Broadcast.SendTo(p.GUID, domainworld.Packet{Opcode: whisper.Opcode, Payload: whisper.Payload})
					// Send WHISPER_INFORM to sender.
					inform, err := codec.BuildWhisperInform(uint64(p.GUID), req.Message)
					if err != nil {
						return nil, err
					}
					_ = s.send(inform)
					log.Debug("whisper delivered", "from", s.player.GUID, "to", p.GUID, "bytes", len(req.Message))
					return nil, nil
				}
			}
		}
	}
	return h.playerNotFound(s, target)
}

// handleChannelSay broadcasts a message to all members of the named
// channel. Mirrors Channel::Say.
func (h *Handler) handleChannelSay(s *Session, req codec.ChatRequest, log *slog.Logger) (*world.Packet, error) {
	name := strings.TrimSpace(req.ChannelName)
	if name == "" {
		return nil, nil
	}
	type channelWorld interface {
		GetChannel(string) interface {
			IsMember(uint32) bool
			IsMuted(uint32) bool
			Members() ([]uint32, []domainworld.Sender)
		}
	}
	w, ok := h.Broadcast.(channelWorld)
	if !ok {
		return h.echoSelf(s, req, log)
	}
	ch := w.GetChannel(name)
	if ch == nil {
		// Not in channel: send "not member" notify.
		notify := codec.ChannelNotify(codec.ChannelNotifyNotMember, name)
		_ = s.send(notify)
		return nil, nil
	}
	if !ch.IsMember(s.player.GUID) {
		notify := codec.ChannelNotify(codec.ChannelNotifyNotMember, name)
		_ = s.send(notify)
		return nil, nil
	}
	if ch.IsMuted(s.player.GUID) {
		notify := codec.ChannelNotify(codec.ChannelNotifyMuted, name)
		_ = s.send(notify)
		return nil, nil
	}
	// Build the channel chat packet and send to all members (including self).
	pkt, err := codec.BuildChannelChatPacket(codec.ChatMsgChannel, req.Language, uint64(s.player.GUID), s.player.Name, name, req.Message)
	if err != nil {
		return nil, err
	}
	_, senders := ch.Members()
	for _, sender := range senders {
		sender.TrySend(domainworld.Packet{Opcode: pkt.Opcode, Payload: pkt.Payload})
	}
	log.Debug("channel message", "channel", name, "guid", s.player.GUID, "bytes", len(req.Message))
	return nil, nil
}

func (h *Handler) playerNotFound(s *Session, name string) (*world.Packet, error) {
	w := codec.NewNameNotFoundWriter(name)
	_ = s.send(w)
	return nil, nil
}

func (h *Handler) echoSelf(s *Session, req codec.ChatRequest, log *slog.Logger) (*world.Packet, error) {
	pkt, err := codec.BuildChatPacket(req.Type, req.Language, uint64(s.player.GUID), s.player.Name, 0, req.Message)
	if err != nil {
		return nil, err
	}
	log.Debug("chat echo", "type", req.Type, "guid", s.player.GUID)
	_ = s.send(pkt)
	return nil, nil
}

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
	// Broadcast text emote to nearby players (including self).
	if h.Broadcast != nil {
		h.Broadcast.BroadcastInRange(s.player.GUID, 90, // default visibility range
			domainworld.Packet{Opcode: pkt.Opcode, Payload: pkt.Payload}, true)
	} else {
		_ = s.send(pkt)
	}
	return nil, nil
}

func (h *Handler) handleChatIgnored(_ context.Context, _ *Session, p world.Packet) (*world.Packet, error) {
	if len(p.Payload) != 9 {
		return nil, codec.ErrWorldPacket
	}
	return nil, nil
}

func (h *Handler) registerChatRoutes(router *Router) {
	inWorld := []State{StateInWorld, StateLoggingOut}
	add := func(opcode uint32, fn handlerFunc) {
		router.routes[opcode] = route{inWorld, fn}
	}
	add(op.CMSG_MESSAGECHAT, h.handleMessageChat)
	add(op.CMSG_TEXT_EMOTE, h.handleTextEmote)
	add(op.CMSG_CHAT_IGNORED, h.handleChatIgnored)
}
