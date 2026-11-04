package tbc

import (
	"errors"

	"wowserver/internal/domain/account"
	"wowserver/internal/domain/character"
	"wowserver/internal/gamedata"
)

// HANDLER_ORCHESTRATION_PATCH
// CreateResultCode maps domain errors to SMSG_CHAR_CREATE result codes.
// Pure function: no I/O, no Handler, no session state.
// Success is not represented; callers send 0x2f on nil error.
// disconnect is true when the session must end (account/session invalidated).
func CreateResultCode(err error, name string) (code byte, disconnect bool) {
	if err == nil {
		return 0x2f, false
	}
	if errors.Is(err, account.ErrChanged) {
		return 0, true
	}
	code = 0x31 // CHAR_CREATE_ERROR
	switch {
	case errors.Is(err, character.ErrNameRepeated):
		code = 0x55 // CHAR_CREATE_NAME_THREE_CONSECUTIVE
	case errors.Is(err, character.ErrDisabled):
		code = 0x33 // CHAR_CREATE_DISABLED
	case errors.Is(err, gamedata.ErrNameReserved):
		code = 0x52 // CHAR_CREATE_NAME_RESERVED
	case errors.Is(err, gamedata.ErrNameAlphabet):
		code = 0x4f // CHAR_CREATE_NAME_ONLY_LETTERS (approx)
	case errors.Is(err, gamedata.ErrNameLength):
		code = 0x4d
		if len(name) == 0 {
			code = 0x4c
		} else if len(name) > 12 {
			code = 0x4e
		}
	case errors.Is(err, character.ErrNameInUse):
		code = 0x32 // CHAR_CREATE_NAME_IN_USE
	case errors.Is(err, character.ErrFaction):
		code = 0x34 // CHAR_CREATE_PVP_TEAMS_VIOLATION
	case errors.Is(err, character.ErrRealmFull):
		code = 0x35 // CHAR_CREATE_SERVER_LIMIT
	case errors.Is(err, character.ErrAccountFull):
		code = 0x36 // CHAR_CREATE_ACCOUNT_LIMIT
	}
	return code, false
}

// DeleteResultCode maps a delete error to the SMSG_CHAR_DELETE response byte.
// Codes from mangosone/server SharedDefines.h:
//
//	CHAR_DELETE_SUCCESS              = 0x3B
//	CHAR_DELETE_FAILED               = 0x3C
//	CHAR_DELETE_FAILED_GUILD_LEADER  = 0x3E
//	CHAR_DELETE_FAILED_ARENA_CAPTAIN = 0x3F
//
// Any other error (missing, not owned, online) maps to CHAR_DELETE_FAILED.
// Never use 0x49 — that is CHAR_LOGIN_LOCKED_BY_BILLING and shows
// "character locked, contact support" in the client.
func DeleteResultCode(err error) byte {
	if err == nil {
		return 0x3B
	}
	switch {
	case errors.Is(err, character.ErrGuildLeader):
		return 0x3E // CHAR_DELETE_FAILED_GUILD_LEADER
	case errors.Is(err, character.ErrArenaCaptain):
		return 0x3F // CHAR_DELETE_FAILED_ARENA_CAPTAIN
	}
	return 0x3C
}
