package tbc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"wowserver/internal/account"
	"wowserver/internal/character"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

type CharacterStore interface {
	List(context.Context, uint64) ([]character.Summary, error)
}

func (h *Handler) characterList(ctx context.Context, a account.WorldSession, name, ip string, log *slog.Logger) (*world.Packet, error) {
	if h.Characters == nil || h.Data == nil {
		return nil, errors.New("character enumeration stores not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, h.RequestTimeout)
	defer cancel()
	rows, err := h.Characters.List(ctx, a.ID)
	if err != nil {
		return nil, fmt.Errorf("load character list: %w", err)
	}
	if len(rows) > 255 {
		return nil, errors.New("too many characters")
	}
	current, err := h.lookup(ctx, name, ip)
	if err != nil || !sameSession(a, current) {
		return nil, account.ErrChanged
	}
	// Count only successfully serialized records, as in BuildEnumData's caller.
	w := wire.NewWriter(MaxServerPayload)
	w.Uint8(0)
	count := byte(0)
	for _, row := range rows {
		b, err := codec.EncodeCharacter(row, h.Data)
		if err != nil {
			log.Warn("character omitted from list", "guid", row.GUID, "reason", err)
			continue
		}
		w.Bytes(b)
		count++
	}
	b, err := w.Result()
	if err != nil {
		return nil, err
	}
	b[0] = count
	log.Debug("character list prepared", "account_id", a.ID, "characters", count, "omitted", len(rows)-int(count))
	return &world.Packet{Opcode: op.SMSG_CHAR_ENUM, Payload: b}, nil
}
