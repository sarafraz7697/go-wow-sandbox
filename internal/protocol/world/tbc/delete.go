package tbc

import (
	"context"
	"log/slog"

	"wowserver/internal/domain/account"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

// deleteCharacter decodes the GUID, calls the application service, and encodes
// the result. Domain rules (ownership, online state) live in the service;
// persistence in storage.
func (h *Handler) deleteCharacter(ctx context.Context, a account.WorldSession, p world.Packet, log *slog.Logger) (*world.Packet, error) {
	guid, err := codec.DecodeDeleteRequest(p.Payload)
	if err != nil {
		return nil, err
	}
	response := func(code byte) (*world.Packet, error) {
		return &world.Packet{Opcode: op.SMSG_CHAR_DELETE, Payload: []byte{code}}, nil
	}
	if h.Deleter == nil {
		return response(0x3C) // CHAR_DELETE_FAILED
	}
	ctx, cancel := context.WithTimeout(ctx, h.RequestTimeout)
	defer cancel()
	log.Debug("deleting character", "account_id", a.ID, "guid", guid)
	if err = h.Deleter.Delete(ctx, a.ID, guid); err != nil {
		log.Warn("character delete failed", "account_id", a.ID, "guid", guid, "error", err)
		return response(DeleteResultCode(err))
	}
	log.Info("character deleted", "account_id", a.ID, "guid", guid)
	return response(0x3B) // CHAR_DELETE_SUCCESS
}
