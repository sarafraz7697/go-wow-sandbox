package tbc

import (
	"context"
	"errors"
	"log/slog"
	"wowserver/internal/account"
	"wowserver/internal/character"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

type CharacterCreator interface {
	Create(context.Context, account.WorldSession, string, character.CreateRequest) (uint32, error)
}

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
		code := byte(0x31)
		switch {
		case errors.Is(err, character.ErrNameRepeated):
			code = 0x55
		case errors.Is(err, character.ErrDisabled):
			code = 0x33
		case errors.Is(err, gamedata.ErrNameReserved):
			code = 0x52
		case errors.Is(err, gamedata.ErrNameAlphabet):
			code = 0x4f
		case errors.Is(err, gamedata.ErrNameLength):
			code = 0x4d
			if len(req.Name) == 0 {
				code = 0x4c
			} else if len(req.Name) > 12 {
				code = 0x4e
			}

		case errors.Is(err, account.ErrChanged):
			return nil, err
		case errors.Is(err, character.ErrNameInUse):
			code = 0x32
		case errors.Is(err, character.ErrFaction):
			code = 0x34
		case errors.Is(err, character.ErrRealmFull):
			code = 0x35
		case errors.Is(err, character.ErrAccountFull):
			code = 0x36
		}
		log.Warn("character creation failed", "account_id", a.ID, "result", code, "error", err)
		return response(code)
	}
	log.Info("character created", "account_id", a.ID, "guid", guid, "race", req.Race, "class", req.Class)
	return response(0x2f)
}
