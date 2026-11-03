package tbc

import (
	"context"
	"errors"
	"log/slog"
	"wowserver/internal/account"
	"wowserver/internal/character"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

type CharacterCreator interface {
	Create(context.Context, account.WorldSession, string, character.Creation) (uint32, error)
}
type CreationProfile interface {
	Build(character.CreateRequest) (character.Creation, error)
}

func (h *Handler) createCharacter(ctx context.Context, a account.WorldSession, ip string, p world.Packet, log *slog.Logger) (*world.Packet, error) {
	r := wire.NewReader(p.Payload)
	req := character.CreateRequest{Name: r.CString(48), Race: r.Uint8(), Class: r.Uint8(), Gender: r.Uint8(), Skin: r.Uint8(), Face: r.Uint8(), HairStyle: r.Uint8(), HairColor: r.Uint8(), FacialHair: r.Uint8(), Outfit: r.Uint8()}
	if err := r.Finish(); err != nil {
		return nil, err
	}
	response := func(code byte) (*world.Packet, error) {
		return &world.Packet{Opcode: op.SMSG_CHAR_CREATE, Payload: []byte{code}}, nil
	}
	if h.Creator == nil || h.Creation == nil {
		return response(0x33)
	}
	plan, err := h.Creation.Build(req)
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
		}
		log.Debug("character creation rejected", "account_id", a.ID, "reason", err, "result", code)
		return response(code)
	}
	ctx, cancel := context.WithTimeout(ctx, h.RequestTimeout)
	defer cancel()
	log.Debug("creating character", "account_id", a.ID, "race", req.Race, "class", req.Class, "items", len(plan.Items), "spells", len(plan.Spells))
	guid, err := h.Creator.Create(ctx, a, ip, plan)
	if err != nil {
		code := byte(0x31)
		switch {
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
