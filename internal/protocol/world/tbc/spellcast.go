package tbc

import (
	"context"
	"math/rand/v2"
	"time"

	"wowserver/internal/domain/aura"
	"wowserver/internal/domain/inventory"
	"wowserver/internal/domain/spell"
	"wowserver/internal/entity"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
	f "wowserver/internal/protocol/world/tbc/updatefields"
)

// manaUseDelay lives in regen.go; the casting state below ports the parts of
// Spell::prepare / cast / finish that a self-targeted player spell needs.

// castItem identifies the item a spell is cast from (Spell::m_CastItem).
type castItem struct {
	bag, slot uint8
	guid      uint32
	entry     uint32
	spell     entity.ItemSpell
}

// castState is the player's current Spell (m_currentSpells[CURRENT_GENERIC_SPELL]).
type castState struct {
	info      *gamedata.SpellInfo
	count     uint8
	targets   codec.SpellTargets
	item      *castItem
	triggered bool
	cat       uint32 // cooldown category in effect (item spells use the item's)
	deadline  time.Time
}

// spellState is the per-session casting/aura state (see Session.spells).
type spellState struct {
	cast         *castState
	cooldowns    map[uint32]time.Time // spell id -> end
	catCooldowns map[uint32]time.Time // category -> end
	gcd          map[uint32]time.Time // StartRecoveryCategory -> end
	auras        aura.Set
	auraAt       time.Time
	sentFields   map[int]uint32 // aura update fields the client already has
	warned       map[uint32]bool
	equipAuras   map[uint32]bool     // passive spell ids applied from equipment
	foes         map[uint32]*foeAuras // auras the player put on creatures, by spawn guid
	delayed      []*delayedSpell      // spells still travelling to their targets
	melee        *castState           // CURRENT_MELEE_SPELL (next swing ability)
	comboTarget  uint64               // combo point target (m_comboTargetGuid)
	comboPoints  uint8                // combo points on the target
}

func (s *Session) sp() *spellState {
	if s.spells == nil {
		s.spells = &spellState{cooldowns: map[uint32]time.Time{}, catCooldowns: map[uint32]time.Time{}, gcd: map[uint32]time.Time{}, sentFields: map[int]uint32{}, warned: map[uint32]bool{}}
	}
	return s.spells
}

func irand(lo, hi int32) int32 {
	if hi < lo {
		lo, hi = hi, lo
	}
	return lo + rand.Int32N(hi-lo+1)
}

func (h *Handler) registerSpellRoutes(add func(uint32, handlerFunc)) {
	add(op.CMSG_CAST_SPELL, h.castSpell)
	add(op.CMSG_USE_ITEM, h.useItem)
	add(op.CMSG_CANCEL_CAST, h.cancelCast)
	add(op.CMSG_CANCEL_AURA, h.cancelAura)
	add(op.CMSG_STANDSTATECHANGE, h.standStateChange)
	add(op.CMSG_REPOP_REQUEST, h.repopRequest)
	add(op.CMSG_RESURRECT_RESPONSE, h.resurrectResponse)
	add(op.CMSG_SPIRIT_HEALER_ACTIVATE, h.spiritHealerActivate)
}

// CMSG_CAST_SPELL: uint32 spell, uint8 cast count, SpellCastTargets.
func (h *Handler) castSpell(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	if s.player == nil || !s.player.Entered {
		return nil, nil
	}
	r := wire.NewReader(p.Payload)
	id, count := r.Uint32(), r.Uint8()
	tg := codec.ReadSpellTargets(r, uint64(s.player.GUID))
	if err := r.Finish(); err != nil {
		return nil, err
	}
	info := h.Data.Spell(id)
	if info == nil {
		return nil, nil // "unknown spell id": the reference only logs
	}
	if !knowsSpell(s, id) || info.Attributes&gamedata.SpellAttrPassive != 0 {
		return nil, nil // spell not in the spellbook, or passive: ignored
	}
	h.prepareSpell(s, time.Now(), info, count, tg, nil, false)
	return nil, nil
}

func knowsSpell(s *Session, id uint32) bool {
	for _, k := range s.player.Spells {
		if k == id {
			return true
		}
	}
	return false
}

// CMSG_USE_ITEM: uint8 bag, slot, spell count, cast count, uint64 item guid, targets.
// Ports HandleUseItemOpcode + Player::CastItemUseSpell.
func (h *Handler) useItem(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	if s.player == nil || !s.player.Entered {
		return nil, nil
	}
	r := wire.NewReader(p.Payload)
	bag, slot, _, count := r.Uint8(), r.Uint8(), r.Uint8(), r.Uint8()
	guid := r.Uint64()
	tg := codec.ReadSpellTargets(r, uint64(s.player.GUID))
	if err := r.Finish(); err != nil {
		return nil, err
	}
	inv := h.inventoryReady(s)
	if inv == nil {
		return nil, nil
	}
	it := inv.ItemByPos(bag, slot)
	if it == nil || codec.ItemGUID(it.GUID) != guid {
		_ = s.send(codec.InventoryChangeFailure(inventory.FailureOf(inventory.ErrItemNotFound)))
		return nil, nil
	}
	proto := it.Proto()
	// some item classes can be used only in equipped state
	if proto.InventoryType != 0 && !inventory.IsEquipmentPos(bag, slot) {
		_ = s.send(codec.InventoryChangeFailure(inventory.FailureOf(inventory.ErrItemNotFound)))
		return nil, nil
	}
	if res := inv.CanUseItem(it, true); res != inventory.ErrOK {
		_ = s.send(codec.InventoryChangeFailure(inventory.Failure{Code: res, Item1: it.GUID}))
		return nil, nil
	}
	now := time.Now()
	n := 0
	for _, is := range proto.Spells {
		if is.SpellID == 0 || is.SpellTrigger != 0 { // ITEM_SPELLTRIGGER_ON_USE
			continue
		}
		info := h.Data.Spell(is.SpellID)
		if info == nil {
			continue
		}
		h.prepareSpell(s, now, info, count, tg, &castItem{bag, slot, it.GUID, it.Entry, is}, n > 0)
		n++
	}
	return nil, nil
}

// CMSG_CANCEL_CAST: uint32 spell.
func (h *Handler) cancelCast(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	r := wire.NewReader(p.Payload)
	id := r.Uint32()
	if err := r.Finish(); err != nil {
		return nil, err
	}
	if s.player != nil && s.spells != nil && s.spells.melee != nil && s.spells.melee.info.ID == id {
		old := s.spells.melee
		s.spells.melee = nil
		caster := uint64(s.player.GUID)
		h.sendToSet(s, codec.SpellFailure(caster, id, 0))
		h.sendToSet(s, codec.SpellFailedOther(caster, id))
		_ = s.send(codec.CastFailed(id, spell.FailedInterrupted, old.count))
		return nil, nil
	}
	if s.player == nil || s.spells == nil || s.spells.cast == nil || s.spells.cast.info.ID != id {
		return nil, nil
	}
	h.cancelCurrent(s)
	return nil, nil
}

// cancelCurrent ports Spell::cancel for a spell still preparing.
func (h *Handler) cancelCurrent(s *Session) {
	st := s.spells.cast
	s.spells.cast = nil
	caster := uint64(s.player.GUID)
	if st.info.StartRecoveryCategory != 0 || st.info.StartRecoveryTime != 0 {
		delete(s.spells.gcd, st.info.StartRecoveryCategory) // CancelGlobalCooldown
	}
	h.sendToSet(s, codec.SpellFailure(caster, st.info.ID, 0))
	h.sendToSet(s, codec.SpellFailedOther(caster, st.info.ID))
	_ = s.send(codec.CastFailed(st.info.ID, spell.FailedInterrupted, st.count))
}

// CMSG_CANCEL_AURA: uint32 spell.
func (h *Handler) cancelAura(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	r := wire.NewReader(p.Payload)
	id := r.Uint32()
	if err := r.Finish(); err != nil {
		return nil, err
	}
	info := h.Data.Spell(id)
	if s.player == nil || info == nil || info.Attributes&(gamedata.SpellAttrCantCancel|gamedata.SpellAttrPassive) != 0 || s.spells == nil {
		return nil, nil
	}
	if hd := s.spells.auras.BySpell(id); hd != nil && hd.Positive {
		s.spells.auras.Remove(hd.Slot)
		h.afterAuraRemoved(s)
	}
	return nil, nil
}

// CMSG_STANDSTATECHANGE: uint32 state (HandleStandStateChangeOpcode -> SetStandState).
func (h *Handler) standStateChange(_ context.Context, s *Session, p world.Packet) (*world.Packet, error) {
	r := wire.NewReader(p.Payload)
	state := r.Uint32()
	if err := r.Finish(); err != nil {
		return nil, err
	}
	if s.player == nil || !s.player.Entered {
		return nil, nil
	}
	h.setStandState(s, uint8(state))
	return nil, nil
}

// setStandState ports Unit::SetStandState: byte 0 of UNIT_FIELD_BYTES_1, and
// standing up removes the auras that require sitting.
func (h *Handler) setStandState(s *Session, state uint8) {
	s.sitting = state != codec.StandStateStand
	s.bytes1 = s.bytes1&^0xFF | uint32(state)
	h.sendToSet(s, codec.ValuesUpdate(uint64(s.player.GUID), []codec.FieldChange{{Index: f.UNIT_FIELD_BYTES_1, Value: s.bytes1}}))
	if !s.sitting && s.spells != nil {
		h.removeAurasWithInterrupt(s, gamedata.AuraInterruptNotSeated)
	}
}

func (h *Handler) removeAurasWithInterrupt(s *Session, flags uint32) {
	if s.spells == nil {
		return
	}
	if len(s.spells.auras.RemoveWithInterrupt(flags)) > 0 {
		h.afterAuraRemoved(s)
	}
}

// prepareSpell ports Spell::prepare.
func (h *Handler) prepareSpell(s *Session, now time.Time, info *gamedata.SpellInfo, count uint8, tg codec.SpellTargets, it *castItem, triggered bool) {
	sp := s.sp()
	fail := func(res uint8) {
		if info.Attributes&gamedata.SpellAttrPassive != 0 {
			res = spell.FailedDontReport
		}
		_ = s.send(codec.CastFailed(info.ID, res, count))
	}
	// Prevent casting at cast another spell (server side check).
	if sp.cast != nil && count != 0 && !triggered {
		fail(spell.FailedSpellInProgress)
		return
	}
	st := &castState{info: info, count: count, targets: tg, item: it, triggered: triggered, cat: info.Category}
	if it != nil && (it.spell.SpellCooldown >= 0 || it.spell.SpellCategoryCooldown >= 0) {
		st.cat = it.spell.SpellCategory
	}
	if res := h.checkCast(s, now, st); res != spell.CastOK {
		fail(res)
		return
	}
	castMS := uint32(0)
	if !triggered {
		castMS = h.Data.CastTimeMS(info)
	}
	if info.IsNextMeleeSwingSpell() && !triggered {
		// Unit::SetCurrentCastedSpell(CURRENT_MELEE_SPELL): replaces the pending one.
		if old := sp.melee; old != nil {
			_ = s.send(codec.SpellFailure(uint64(s.player.GUID), old.info.ID, 0), codec.SpellFailedOther(uint64(s.player.GUID), old.info.ID))
			_ = s.send(codec.CastFailed(old.info.ID, spell.FailedInterrupted, old.count))
		}
		sp.melee = st
		caster := uint64(s.player.GUID)
		h.sendToSet(s, codec.SpellStart(caster, caster, info.ID, count, castMS, tg))
		if info.StartRecoveryTime > 0 {
			sp.gcd[info.StartRecoveryCategory] = now.Add(time.Duration(info.StartRecoveryTime) * time.Millisecond)
		}
		return
	}
	if !triggered {
		sp.cast = st
		caster := uint64(s.player.GUID)
		source := caster
		if it != nil {
			source = codec.ItemGUID(it.guid)
		}
		h.sendToSet(s, codec.SpellStart(source, caster, info.ID, count, castMS, tg))
		// TriggerGlobalCooldown
		if it == nil && info.StartRecoveryTime > 0 {
			sp.gcd[info.StartRecoveryCategory] = now.Add(time.Duration(info.StartRecoveryTime) * time.Millisecond)
		}
	}
	if castMS == 0 {
		h.castNow(s, now, st)
		return
	}
	st.deadline = now.Add(time.Duration(castMS) * time.Millisecond)
}

// checkCast is the subset of Spell::CheckCast the supported spells need.
func (h *Handler) checkCast(s *Session, now time.Time, st *castState) uint8 {
	info, sp, p := st.info, s.sp(), s.player
	if p.Health == 0 || p.PlayerFlags&playerFlagsGhost != 0 {
		return spell.FailedCasterDead
	}
	if info.Attributes&gamedata.SpellAttrRanged != 0 {
		return spell.FailedSpellUnavailable // ranged casts (ammo) are not supported yet
	}
	if end, ok := sp.cooldowns[info.ID]; ok && now.Before(end) {
		if st.triggered {
			return spell.FailedDontReport
		}
		return spell.FailedNotReady
	}
	if st.cat != 0 {
		if end, ok := sp.catCooldowns[st.cat]; ok && now.Before(end) && !st.triggered {
			return spell.FailedNotReady
		}
	}
	if !st.triggered && info.StartRecoveryTime > 0 && st.item == nil {
		if end, ok := sp.gcd[info.StartRecoveryCategory]; ok && now.Before(end) {
			return spell.FailedNotReady
		}
	}
	// Unit targets other than the caster are only creatures hit by hostile effects.
	if st.targets.Mask&(codec.TargetFlagUnit|codec.TargetFlagUnk2) != 0 && st.targets.Unit != uint64(p.GUID) && !spellHasEnemyEffect(info) {
		return spell.FailedBadTargets
	}
	if res := h.checkSpellTarget(s, now, st, true); res != spell.CastOK {
		return res
	}
	return h.checkPower(s, st)
}

func (h *Handler) powerCost(s *Session, st *castState) uint32 {
	if st.item != nil || st.triggered {
		return 0
	}
	rs := h.regenState(s, time.Now())
	return spell.PowerCost(st.info, s.player.Health, s.player.Stats.MaxHealth, s.player.Stats.MaxMana, rs.Power, rs.MaxPower)
}

// checkPower ports Spell::CheckPower.
func (h *Handler) checkPower(s *Session, st *castState) uint8 {
	if st.item != nil || st.triggered {
		return spell.CastOK
	}
	cost := h.powerCost(s, st)
	if st.info.PowerType == spell.PowerHealth {
		if s.player.Health <= cost {
			return spell.FailedCasterAuraState
		}
		return spell.CastOK
	}
	if st.info.PowerType >= 5 {
		return spell.FailedError
	}
	rs := h.regenState(s, time.Now())
	if rs.Power[st.info.PowerType] < cost {
		return spell.FailedNoPower
	}
	return spell.CastOK
}

// spellTick advances a pending cast and the aura set.
func (h *Handler) spellTick(s *Session, now time.Time) {
	if s.spells == nil || s.player == nil {
		return
	}
	sp := s.spells
	if sp.cast != nil && !now.Before(sp.cast.deadline) {
		h.castNow(s, now, sp.cast)
	}
	// Expired cooldowns leave the maps (cooldown housekeeping).
	for id, end := range sp.cooldowns {
		if !now.Before(end) {
			delete(sp.cooldowns, id)
		}
	}
	for cat, end := range sp.catCooldowns {
		if !now.Before(end) {
			delete(sp.catCooldowns, cat)
		}
	}
	for cat, end := range sp.gcd {
		if !now.Before(end) {
			delete(sp.gcd, cat)
		}
	}
	h.auraTick(s, now)
	h.foeTick(s, now)
	h.delayedTick(s, now)
}

// clearCooldown removes one spell cooldown and informs the client
// (SpellCooldownMgr::RemoveSpellCooldown with update).
func (h *Handler) clearCooldown(s *Session, id uint32) {
	delete(s.sp().cooldowns, id)
	_ = s.send(codec.ClearCooldownPacket(id, uint64(s.player.GUID)))
}

// prohibitSpellSchool ports Player::ProhibitSpellSchool: every known spell of
// the school is locked for the duration and one SMSG_SPELL_COOLDOWN informs
// the client (the spell school lockout of interrupt effects).
func (h *Handler) prohibitSpellSchool(s *Session, schoolMask uint32, ms uint32) {
	now := time.Now()
	end := now.Add(time.Duration(ms) * time.Millisecond)
	sp := s.sp()
	var entries []codec.CooldownEntry
	for _, id := range s.player.Spells {
		info := h.Data.Spell(id)
		if info == nil || info.Attributes&gamedata.SpellAttrDisabledWhileActive != 0 {
			continue
		}
		if info.SchoolMask&schoolMask == 0 {
			continue
		}
		if cur, ok := sp.cooldowns[id]; ok && cur.After(end) {
			continue
		}
		sp.cooldowns[id] = end
		entries = append(entries, codec.CooldownEntry{Spell: id, Remain: ms})
	}
	if len(entries) > 0 {
		_ = s.send(codec.SpellCooldownPacket(uint64(s.player.GUID), entries))
	}
}

// castNow ports Spell::cast for a spell whose timer ran out.
func (h *Handler) castNow(s *Session, now time.Time, st *castState) {
	sp := s.sp()
	done := func() {
		if sp.cast == st {
			sp.cast = nil
		}
	}
	if res := h.checkSpellTarget(s, now, st, false); res != spell.CastOK {
		_ = s.send(codec.CastFailed(st.info.ID, res, st.count))
		done()
		return
	}
	if res := h.checkPower(s, st); res != spell.CastOK {
		_ = s.send(codec.CastFailed(st.info.ID, res, st.count))
		done()
		return
	}
	h.startCooldowns(s, now, st)
	h.takePower(s, now, st)
	if st.info.Attributes&gamedata.SpellAttrExReqTargetComboPoints != 0 || st.info.AttributesEx&(gamedata.SpellAttrExReqTargetComboPoints|gamedata.SpellAttrExReqComboPoints) != 0 {
		h.clearComboPoints(s) // finishers spend every point on the target
	}
	caster := uint64(s.player.GUID)
	source := caster
	if st.item != nil {
		source = codec.ItemGUID(st.item.guid)
	}
	targets := h.fillTargets(s, now, st)
	hit, miss := h.resolveHits(s, now, st, targets)
	_ = s.send(codec.ClearExtraAuraInfo(caster, st.info.ID))
	h.sendToSet(s, codec.SpellGoMiss(source, caster, st.info.ID, uint32(now.UnixMilli()), hit, miss, st.targets))
	if st.info.Speed > 0 {
		for _, t := range targets {
			if !t.self {
				h.launch(s, now, st, t)
			}
		}
	}
	if h.processTargets(s, now, st, targets, now) {
		sp.delayed = append(sp.delayed, &delayedSpell{st: st, targets: targets, start: now})
	}
	h.takeCastItem(s, st)
	done()
}

// delayedTick lands the spells whose travel time elapsed.
func (h *Handler) delayedTick(s *Session, now time.Time) {
	sp := s.spells
	keep := sp.delayed[:0]
	for _, d := range sp.delayed {
		if h.processTargets(s, now, d.st, d.targets, d.start) {
			keep = append(keep, d)
		}
	}
	sp.delayed = keep
}

// noUnitTargets reports whether FillTargetMap would produce no unit target:
// every used effect targets nothing (implicit target A/B both 0 and no aura/heal).
func (h *Handler) noUnitTargets(info *gamedata.SpellInfo) bool {
	for i := 0; i < 3; i++ {
		if info.Effect[i] != 0 && (info.TargetA[i] != 0 || info.TargetB[i] != 0) {
			return false
		}
	}
	for i := 0; i < 3; i++ {
		switch info.Effect[i] {
		case gamedata.EffectApplyAura, gamedata.EffectHeal, gamedata.EffectEnergize:
			return false
		}
	}
	return true
}

// startCooldowns ports AddSpellAndCategoryCooldowns (no spell mods).
func (h *Handler) startCooldowns(s *Session, now time.Time, st *castState) {
	info := st.info
	if info.Attributes&(gamedata.SpellAttrDisabledWhileActive|gamedata.SpellAttrPassive) != 0 {
		return
	}
	cat, rec, catrec := info.Category, int32(info.Recovery), int32(info.CategoryRecovery)
	if st.item != nil {
		is := st.item.spell
		if is.SpellCooldown >= 0 || is.SpellCategoryCooldown >= 0 {
			cat, rec, catrec = is.SpellCategory, is.SpellCooldown, is.SpellCategoryCooldown
		}
	}
	if rec < 0 {
		rec = 0
	}
	if catrec < 0 {
		catrec = 0
	}
	if rec == 0 && catrec == 0 {
		return
	}
	ms := func(v int32) time.Duration { return time.Duration(v) * time.Millisecond }
	var catEnd, recEnd time.Time
	if catrec > 0 {
		catEnd = now.Add(ms(catrec))
	}
	recEnd = catEnd
	if rec > 0 {
		recEnd = now.Add(ms(rec))
	}
	sp := s.sp()
	if !recEnd.IsZero() {
		sp.cooldowns[info.ID] = recEnd
	}
	if cat != 0 && catrec > 0 {
		sp.catCooldowns[cat] = catEnd
	}
}

// takePower ports Spell::TakePower.
func (h *Handler) takePower(s *Session, now time.Time, st *castState) {
	if st.item != nil || st.triggered {
		return
	}
	cost := h.powerCost(s, st)
	p := s.player
	if st.info.PowerType == spell.PowerHealth {
		p.Health = sub(p.Health, cost)
		h.sendFields(s, codec.FieldChange{Index: f.UNIT_FIELD_HEALTH, Value: p.Health})
		h.checkPlayerDeath(s, uint64(p.GUID), now)
		return
	}
	if st.info.PowerType >= 5 || cost == 0 {
		return
	}
	pw := h.getPower(s, int(st.info.PowerType))
	h.setPower(s, int(st.info.PowerType), sub(pw, cost))
	if st.info.PowerType == spell.PowerMana {
		s.manaUseUntil = now.Add(manaUseDelay)
	}
}

func sub(a, b uint32) uint32 {
	if b > a {
		return 0
	}
	return a - b
}

func (h *Handler) getPower(s *Session, i int) uint32 {
	p := s.player
	return [5]uint32{p.Mana, p.Rage, p.Focus, p.Energy, p.Happiness}[i]
}

// setPower stores a power value and sends UNIT_FIELD_POWERi.
func (h *Handler) setPower(s *Session, i int, v uint32) {
	p := s.player
	switch i {
	case 0:
		p.Mana = v
	case 1:
		p.Rage = v
	case 2:
		p.Focus = v
	case 3:
		p.Energy = v
	case 4:
		p.Happiness = v
	}
	h.sendFields(s, codec.FieldChange{Index: f.UNIT_FIELD_POWER1 + i, Value: v})
}

// sendFields routes value changes like the reference update pipeline: unit
// (public) fields are broadcast to every player that sees this one (UpdateData
// udata1), player fields go to the owning client only (udata2).
func (h *Handler) sendFields(s *Session, ch ...codec.FieldChange) {
	var pub, own []codec.FieldChange
	for _, c := range ch {
		if int(c.Index) < f.PLAYER_QUEST_LOG_1_1 {
			pub = append(pub, c)
		} else {
			own = append(own, c)
		}
	}
	if len(pub) > 0 {
		h.sendToSet(s, codec.ValuesUpdate(uint64(s.player.GUID), pub))
	}
	if len(own) > 0 {
		_ = s.send(codec.ValuesUpdate(uint64(s.player.GUID), own))
	}
}

// applyEffects runs the effects of a cast on the caster (the only unit target).
func (h *Handler) applyEffects(s *Session, now time.Time, st *castState, mask uint32) {
	info, p := st.info, s.player
	caster := uint64(p.GUID)
	var effects [3]*aura.Effect
	hasAura := false
	for i := 0; i < 3; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		switch info.Effect[i] {
		case 0:
		case gamedata.EffectHeal:
			v := spell.EffectValue(info, i, uint32(p.Level), irand)
			if v < 0 {
				v = 0
			}
			h.sendToSet(s, codec.SpellHealLog(caster, caster, info.ID, uint32(v), false))
			if np := min(p.Health+uint32(v), p.Stats.MaxHealth); np != p.Health {
				p.Health = np
				h.sendFields(s, codec.FieldChange{Index: f.UNIT_FIELD_HEALTH, Value: np})
			}
		case gamedata.EffectEnergize:
			v := spell.EffectValue(info, i, uint32(p.Level), irand)
			pw := info.MiscValue[i]
			rs := h.regenState(s, now)
			if v <= 0 || pw < 0 || pw >= 5 || rs.MaxPower[pw] == 0 {
				continue
			}
			h.sendToSet(s, codec.SpellEnergizeLog(caster, caster, info.ID, uint32(pw), uint32(v)))
			h.setPower(s, int(pw), min(rs.Power[pw]+uint32(v), rs.MaxPower[pw]))
		case gamedata.EffectApplyAura:
			e := &aura.Effect{Name: info.Aura[i], Amount: spell.EffectValue(info, i, uint32(p.Level), irand), Misc: info.MiscValue[i]}
			if isPeriodic(e.Name) {
				e.Period = int32(info.AuraPeriod[i])
			}
			effects[i] = e
			hasAura = true
		case gamedata.EffectTeleportUnits:
			h.effectTeleportUnits(s, info, i)
		case gamedata.EffectHealPct:
			pct := spell.EffectValue(info, i, uint32(p.Level), irand)
			if pct > 0 {
				v := p.Stats.MaxHealth * uint32(pct) / 100
				_ = s.send(codec.SpellHealLog(caster, caster, info.ID, v, false))
				if np := min(p.Health+v, p.Stats.MaxHealth); np != p.Health {
					p.Health = np
					h.sendFields(s, codec.FieldChange{Index: f.UNIT_FIELD_HEALTH, Value: np})
				}
			}
		case gamedata.EffectEnergizePct:
			pct := spell.EffectValue(info, i, uint32(p.Level), irand)
			pw := info.MiscValue[i]
			rs := h.regenState(s, now)
			if pct > 0 && pw >= 0 && pw < 5 && rs.MaxPower[pw] > 0 {
				v := rs.MaxPower[pw] * uint32(pct) / 100
				h.sendToSet(s, codec.SpellEnergizeLog(caster, caster, info.ID, uint32(pw), v))
				h.setPower(s, int(pw), min(rs.Power[pw]+v, rs.MaxPower[pw]))
			}
		case gamedata.EffectDummy:
			h.warnOnce(s, info.ID<<2|uint32(i), "dummy effect not simulated", info.ID, 0)
		case gamedata.EffectScriptEffect:
			// Spell::EffectScriptEffect has no case for these spells (e.g. 8690).
		default:
			h.warnOnce(s, info.ID<<2|uint32(i), "spell effect not implemented", info.ID, info.Effect[i])
		}
	}
	if hasAura {
		h.addAura(s, now, st, effects)
	}
}

func isPeriodic(name uint32) bool {
	return name == aura.PeriodicHeal || name == aura.PeriodicEnergize || name == aura.PeriodicDamage
}

func (h *Handler) warnOnce(s *Session, key uint32, msg string, spellID, effect uint32) {
	if s.sp().warned[key] {
		return
	}
	s.sp().warned[key] = true
	s.log.Warn(msg, "spell", spellID, "effect", effect)
}

// takeCastItem ports Spell::TakeCastItem.
func (h *Handler) takeCastItem(s *Session, st *castState) {
	if st.item == nil || st.triggered {
		return
	}
	inv := h.inventoryReady(s)
	if inv == nil {
		return
	}
	it := inv.ItemByGUID(st.item.guid)
	if it == nil {
		return
	}
	proto := it.Proto()
	expendable, without := false, false
	for i, is := range proto.Spells {
		if is.SpellID == 0 || is.SpellCharges == 0 {
			continue
		}
		if is.SpellCharges < 0 && proto.ExtraFlags&nonConsumableExtraFlag == 0 {
			expendable = true
		}
		charges := int32(it.Fields[f.ITEM_FIELD_SPELL_CHARGES+i])
		if charges != 0 {
			if charges > 0 {
				charges--
			} else {
				charges++
			}
			if proto.Stackable == 1 {
				it.Fields[f.ITEM_FIELD_SPELL_CHARGES+i] = uint32(charges)
				_ = s.send(codec.ValuesUpdate(codec.ItemGUID(it.GUID), []codec.FieldChange{{Index: f.ITEM_FIELD_SPELL_CHARGES + i, Value: uint32(charges)}}))
			}
			s.player.InventoryDirty = true
		}
		without = charges == 0
	}
	if expendable && without {
		h.applyInventoryEvents(s, inv, inv.ConsumeItem(it.Bag, it.Slot, 1))
	}
}

// ITEM_EXTRA_NON_CONSUMABLE (ItemPrototype.h).
const nonConsumableExtraFlag = 0x00000001

// effectTeleportUnits ports Spell::EffectTeleportUnits for the caster as the
// only target. Only the innkeeper (homebind) variant has data to run on:
// spell_target_position destinations are not loaded yet.
func (h *Handler) effectTeleportUnits(s *Session, info *gamedata.SpellInfo, i int) {
	target := info.TargetB[i]
	if target == 0 {
		target = info.TargetA[i]
	}
	switch target {
	case gamedata.TargetInnkeeperCoordinates:
		h.teleportToHomebind(s)
	default:
		h.warnOnce(s, info.ID<<2|uint32(i), "teleport target not implemented", info.ID, target)
	}
}
