package tbc

import (
	"context"
	"log/slog"

	"wowserver/internal/domain/account"
	"wowserver/internal/domain/character"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

// CharacterCreator is the application port used by the create handler.
// Implemented by character.Service (profile + store); not by SQL directly.
type CharacterCreator interface {
	Create(context.Context, account.WorldSession, string, character.CreateRequest) (uint32, error)
}

// HANDLER_ORCHESTRATION_PATCH
// createCharacter only orchestrates: decode -> application service -> encode.
// Domain rules live in character.Service / policy / profile; persistence in storage.
func (h *Handler) createCharacter(ctx context.Context, a account.WorldSession, ip string, p world.Packet, log *slog.Logger) (*world.Packet, error) {
	req, err := codec.DecodeCreate(p.Payload)
	if err != nil {
		return nil, err
	}
	response := func(code byte) (*world.Packet, error) {
		return &world.Packet{Opcode: op.SMSG_CHAR_CREATE, Payload: []byte{code}}, nil
	}
	if h.Creator == nil {
		return response(0x33)
	}
	ctx, cancel := context.WithTimeout(ctx, h.RequestTimeout)
	defer cancel()
	log.Debug("creating character", "account_id", a.ID, "race", req.Race, "class", req.Class)
	guid, err := h.Creator.Create(ctx, a, ip, req)
	if err != nil {
		code, disconnect := CreateResultCode(err, req.Name)
		if disconnect {
			return nil, err
		}
		log.Warn("character creation failed", "account_id", a.ID, "result", code, "error", err)
		return response(code)
	}
	log.Info("character created", "account_id", a.ID, "guid", guid, "race", req.Race, "class", req.Class)
	return response(0x2f)
}
