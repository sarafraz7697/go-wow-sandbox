package tbc

import (
	"context"
	"time"

	"wowserver/internal/domain/character"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	f "wowserver/internal/protocol/world/tbc/updatefields"
)

// Player death, release spirit and resurrection (PlayerDeath.cpp,
// MiscHandler.cpp, NPCHandler.cpp).

const (
	playerFlagsGhost            = 0x00000010 // PLAYER_FLAGS_GHOST (Player.h)
	ghostSpellID                = 8326       // SPELL_AURA_GHOST + ghost speed
	nightElfGhostSpellID        = 20584      // wisp form speed bonuses
	resurrectionSickness        = 15007      // SPELL_ID_PASSIVE_RESURRECTION_SICKNESS
	unitByte1AlwaysStand        = 0x01       // UNIT_BYTE1_FLAG_ALWAYS_STAND
	deathSicknessLevel          = 11         // CONFIG_INT32_DEATH_SICKNESS_LEVEL
	npcFlagSpiritHealer         = 0x00004000 // UNIT_NPC_FLAG_SPIRITHEALER
	playerFieldByteReleaseTimer = 0x08       // PLAYER_FIELD_BYTE_RELEASE_TIMER
)

// killPlayer ports Player::KillPlayer for a player whose health reached zero.
// killer is the guid credited with the kill (self-inflicted deaths carry the
// player's own guid). Combat stops, casts cancel and every aura leaves the
// corpse (Unit::SetDeathState -> RemoveAllAurasOnDeath); the passives are
// re-applied on resurrection.
func (h *Handler) killPlayer(s *Session, killer uint64, now time.Time) {
	p := s.player
	if p == nil || p.Stats == nil || p.Health > 0 {
		return
	}
	p.Rage, p.Energy, p.Focus, p.Happiness = 0, 0, 0, 0
	h.attackStop(s, false)
	if s.spells != nil {
		if s.spells.cast != nil {
			h.cancelCurrent(s)
		}
		for _, hd := range s.spells.auras.Holders() {
			s.spells.auras.Remove(hd.Slot)
		}
		h.syncAuraFields(s)
	}
	// The death dialog counts down to the auto release.
	s.fieldBytes |= playerFieldByteReleaseTimer
	h.sendFields(s,
		codec.FieldChange{Index: f.UNIT_FIELD_HEALTH, Value: 0},
		codec.FieldChange{Index: f.UNIT_FIELD_POWER2, Value: 0},
		codec.FieldChange{Index: f.UNIT_FIELD_POWER4, Value: 0},
		codec.FieldChange{Index: f.PLAYER_FIELD_BYTES, Value: s.fieldBytes})
	if killer != 0 && killer != uint64(p.GUID) {
		s.log.Info("player killed", "guid", p.GUID, "killer", killer)
	}
}

// checkPlayerDeath routes a health change into death when it reached zero.
func (h *Handler) checkPlayerDeath(s *Session, killer uint64, now time.Time) {
	if s.player != nil && s.player.Health == 0 {
		h.killPlayer(s, killer, now)
	}
}

// applyGhostAuras casts the ghost spells of BuildPlayerRepop: 8326 (ghost,
// ghost speed) and the night elf wisp form 20584.
func (h *Handler) applyGhostAuras(s *Session) {
	for _, id := range []uint32{ghostSpellID, nightElfGhostSpellID} {
		if id == nightElfGhostSpellID && s.player.Race != 4 {
			continue
		}
		info := h.Data.Spell(id)
		if info == nil {
			continue
		}
		if hd := h.newPassiveHolder(s, info); hd != nil {
			s.sp().auras.Add(hd)
		}
	}
	h.syncAuraFields(s)
}

// buildPlayerRepop ports Player::BuildPlayerRepop: the ghost spells apply,
// health becomes 1, PLAYER_FLAGS_GHOST is set and the corpse state byte
// (UNIT_FIELD_BYTES_1 byte 3) becomes ALWAYS_STAND.
func (h *Handler) buildPlayerRepop(s *Session) {
	p := s.player
	h.applyGhostAuras(s)
	p.Health = 1
	p.PlayerFlags |= playerFlagsGhost
	s.fieldBytes &^= playerFieldByteReleaseTimer
	s.bytes1 = s.bytes1&^0xFF000000 | unitByte1AlwaysStand<<24
	h.sendFields(s,
		codec.FieldChange{Index: f.UNIT_FIELD_HEALTH, Value: 1},
		codec.FieldChange{Index: f.PLAYER_FLAGS, Value: p.PlayerFlags},
		codec.FieldChange{Index: f.PLAYER_FIELD_BYTES, Value: s.fieldBytes},
		codec.FieldChange{Index: f.UNIT_FIELD_BYTES_1, Value: s.bytes1})
}

// repopAtGraveyard ports Player::RepopAtGraveyard with the homebind standing
// in for the graveyard (no world_safe_locs data is loaded yet); the spirit
// healer marker points there.
func (h *Handler) repopAtGraveyard(s *Session) {
	p := s.player
	if !h.teleportTo(s, p.HomeMap, p.HomeX, p.HomeY, p.HomeZ, p.Orientation) {
		return
	}
	_ = s.send(codec.DeathReleaseLoc(p.HomeMap, p.HomeX, p.HomeY, p.HomeZ))
}

// CMSG_REPOP_REQUEST: uint8 ignored (HandleRepopRequestOpcode).
func (h *Handler) repopRequest(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	r := wire.NewReader(p.Payload)
	r.Uint8()
	if err := r.Finish(); err != nil {
		return nil, err
	}
	pl := s.player
	if pl == nil || !pl.Entered || pl.Health > 0 || pl.PlayerFlags&playerFlagsGhost != 0 {
		return nil, nil
	}
	h.buildPlayerRepop(s)
	h.repopAtGraveyard(s)
	return nil, nil
}

// CMSG_SPIRIT_HEALER_ACTIVATE: npc guid (HandleSpiritHealerActivateOpcode ->
// SendSpiritResurrect): the ghost revives at the healer with half health and
// mana, and resurrection sickness when old enough.
func (h *Handler) spiritHealerActivate(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	r := wire.NewReader(p.Payload)
	guid := r.Uint64()
	if err := r.Finish(); err != nil {
		return nil, err
	}
	pl := s.player
	if pl == nil || !pl.Entered || pl.PlayerFlags&playerFlagsGhost == 0 {
		return nil, nil
	}
	if _, ok := h.npcIfCanInteract(s, guid, npcFlagSpiritHealer); !ok {
		return nil, nil
	}
	h.resurrectPlayer(s, 0.5, true)
	return nil, nil
}

// CMSG_RESURRECT_RESPONSE: uint64 resurrecter, uint8 status
// (HandleResurrectResponseOpcode): accepts a pending resurrect offer.
func (h *Handler) resurrectResponse(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	r := wire.NewReader(p.Payload)
	guid := r.Uint64()
	status := r.Uint8()
	if err := r.Finish(); err != nil {
		return nil, err
	}
	pl := s.player
	if pl == nil || !pl.Entered || pl.Health > 0 {
		return nil, nil
	}
	if status == 0 || s.pendingResurrect == 0 || guid != s.pendingResurrect {
		s.pendingResurrect = 0
		return nil, nil
	}
	s.pendingResurrect = 0
	h.resurrectPlayer(s, 0.5, false)
	return nil, nil
}

// resurrectPlayer ports Player::ResurrectPlayer: the spirit healer marker
// clears, the ghost flags and auras leave, the passives return, health and
// powers are restored to the given share, and resurrection sickness applies
// for spirit healer revivals of characters at or above level 11 (one minute
// per level above 10, ten minutes from level 20).
func (h *Handler) resurrectPlayer(s *Session, restorePct float32, sickness bool) {
	p := s.player
	_ = s.send(codec.DeathReleaseLocClear())
	p.PlayerFlags &^= playerFlagsGhost
	s.fieldBytes &^= playerFieldByteReleaseTimer
	s.bytes1 &^= 0xFF000000
	if s.spells != nil {
		for _, id := range []uint32{ghostSpellID, nightElfGhostSpellID} {
			if hd := s.spells.auras.BySpell(id); hd != nil {
				s.spells.auras.Remove(hd.Slot)
			}
		}
		h.applyPassiveAuras(s) // life returns with the passives
		h.syncAuraFields(s)
	}
	h.refreshStats(s)
	if restorePct > 0 && p.Stats != nil {
		p.Health = max(uint32(float32(p.Stats.MaxHealth)*restorePct), 1)
		p.Mana = uint32(float32(p.Stats.MaxMana) * restorePct)
		p.Energy = uint32(float32(100) * restorePct)
		p.Rage = 0
		h.sendFields(s,
			codec.FieldChange{Index: f.UNIT_FIELD_HEALTH, Value: p.Health},
			codec.FieldChange{Index: f.UNIT_FIELD_POWER1, Value: p.Mana},
			codec.FieldChange{Index: f.UNIT_FIELD_POWER4, Value: p.Energy},
			codec.FieldChange{Index: f.PLAYER_FLAGS, Value: p.PlayerFlags},
			codec.FieldChange{Index: f.PLAYER_FIELD_BYTES, Value: s.fieldBytes},
			codec.FieldChange{Index: f.UNIT_FIELD_BYTES_1, Value: s.bytes1})
	}
	if !sickness || p.Level < deathSicknessLevel {
		return
	}
	info := h.Data.Spell(resurrectionSickness)
	if info == nil {
		return
	}
	hd := h.newPassiveHolder(s, info)
	if hd == nil {
		return
	}
	hd.Passive = false
	if int32(p.Level) < deathSicknessLevel+9 {
		hd.MaxDuration = (int32(p.Level) - deathSicknessLevel + 1) * 60 * 1000
		hd.Duration = hd.MaxDuration
	} else {
		// The spell duration (10 min); a table-less fixture falls back to it.
		if d := h.Data.DurationMS(info); d > 0 {
			hd.MaxDuration, hd.Duration = d, d
		} else {
			hd.MaxDuration, hd.Duration = 10*60*1000, 10*60*1000
		}
	}
	s.sp().auras.Add(hd)
	h.syncAuraFields(s)
	_ = s.send(codec.UpdateAuraDuration(hd.Slot, uint32(hd.Duration)),
		codec.SetExtraAuraInfo(uint64(p.GUID), hd.Slot, hd.SpellID, uint32(hd.MaxDuration), uint32(hd.Duration)))
	h.afterAuraChange(s)
}

// applyGhostStateAtLogin re-creates the ghost state of a player that logged
// out dead (the ghost flag and health 1 were saved). A saved health of 0
// without the ghost flag (the session dropped between death and release)
// also comes back as a ghost at the saved position.
func (h *Handler) applyGhostStateAtLogin(s *Session, p *character.Player) {
	if p.PlayerFlags&playerFlagsGhost == 0 && p.Health != 0 {
		return
	}
	p.Health = 1
	p.PlayerFlags |= playerFlagsGhost
	s.bytes1 = s.bytes1&^0xFF000000 | unitByte1AlwaysStand<<24
	h.applyGhostAuras(s)
}
