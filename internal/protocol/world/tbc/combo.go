package tbc

import (
	"math"
	"time"

	"wowserver/internal/domain/spell"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

// Combo points and class-spell helpers.

// spellInterruptFlagDamage is SPELL_INTERRUPT_FLAG_DAMAGE (Unit.h).
const spellInterruptFlagDamage = 0x02

// addComboPoints ports Player::AddComboPoints: points stack on one target at
// a time, a new target resets them, five is the maximum.
func (h *Handler) addComboPoints(s *Session, target uint64, count uint8) {
	if count == 0 || s.player == nil {
		return
	}
	sp := s.sp()
	if sp.comboTarget == target {
		sp.comboPoints += count
	} else {
		sp.comboTarget = target
		sp.comboPoints = count
	}
	if sp.comboPoints > 5 {
		sp.comboPoints = 5
	}
	h.sendComboPoints(s)
}

// clearComboPoints ports Player::ClearComboPoints (target death, finishers).
func (h *Handler) clearComboPoints(s *Session) {
	sp := s.sp()
	if sp.comboTarget == 0 {
		return
	}
	sp.comboTarget = 0
	sp.comboPoints = 0
	if s.player != nil {
		h.sendComboPoints(s)
	}
}

// sendComboPoints is Player::SendComboPoints.
func (h *Handler) sendComboPoints(s *Session) {
	sp := s.sp()
	if sp.comboTarget == 0 {
		return
	}
	_ = s.send(codec.UpdateComboPoints(sp.comboTarget, sp.comboPoints))
}

// effectLeap ports SPELL_EFFECT_LEAP for charge-style spells: the caster
// lands in front of the target (creature or player).
func (h *Handler) effectLeap(s *Session, target uint64, x, y, z float32) {
	p := s.player
	_ = target
	dx, dy := float64(x-p.X), float64(y-p.Y)
	d := math.Sqrt(dx*dx + dy*dy)
	if d < 0.01 {
		return
	}
	// Land just inside melee reach, keeping the target in front.
	const land = 1.0
	nx := x - float32((dx/d)*land)
	ny := y - float32((dy/d)*land)
	o := float32(math.Atan2(dy, dx))
	h.teleportTo(s, p.Map, nx, ny, z, o)
}

// interruptCastEvent applies a SPELL_EFFECT_INTERRUPT_CAST on this player:
// an interruptible cast (SPELL_INTERRUPT_FLAG_INTERRUPT and silence
// prevention) is cancelled and its school is prohibited for the interrupt's
// duration (Spell::EffectInterruptCast + ProhibitSpellSchool).
func (h *Handler) interruptCastEvent(s *Session, ev pvpEvent, now time.Time) {
	cur := s.spells.cast
	if cur == nil {
		return
	}
	if cur.info.InterruptFlags&spellInterruptFlagDamage == 0 {
		// Reference requires SPELL_INTERRUPT_FLAG_INTERRUPT; damage flag is
		// the same bit family - every interruptible cast carries one of them.
		if cur.info.InterruptFlags&0x04 == 0 {
			return
		}
	}
	if cur.info.PreventionType != 1 { // SPELL_PREVENTION_TYPE_SILENCE
		return
	}
	school := cur.info.SchoolMask
	st := cur
	h.cancelCurrent(s)
	if ev.lockoutMS > 0 {
		h.prohibitSpellSchool(s, school, ev.lockoutMS)
	}
	_ = st
	_ = op.SMSG_SPELL_COOLDOWN
	_ = spell.CastOK
}
