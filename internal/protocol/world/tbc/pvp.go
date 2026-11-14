package tbc

import (
	"math"
	"sync"
	"time"

	"wowserver/internal/domain/aura"
	"wowserver/internal/domain/combat"
	"wowserver/internal/domain/spell"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
	f "wowserver/internal/protocol/world/tbc/updatefields"
)

// Player-versus-player combat: every in-world player publishes a live combat
// snapshot in a shared registry (the world simulation's presence only carries
// the login state); attackers resolve targets, roll hits and compute damage
// from the snapshot and push the result into the victim's event inbox. The
// owning session applies the events on its own event loop (authoritative
// health, auras and death), which keeps session-owned state single-threaded
// like the reference's map threads.

// pvpSnapshot is the attacker's view of one in-world player.
type pvpSnapshot struct {
	guid                         uint32
	name                         string
	team                         uint32
	level, race, class           uint8
	mapID                        uint32
	x, y, z, o                   float32
	health, maxHealth            uint32
	ghost                        bool
	defense                      uint32
	armor                        uint32
	resist                       [7]int32
	dodgePct, parryPct, blockPct float32
}

// pvpEventKind selects the payload of one inbox event.
type pvpEventKind uint8

const (
	pvpDamage pvpEventKind = iota // yellow or white damage landed
	pvpAura                       // an aura application (debuff or HoT)
	pvpInterrupt                  // SPELL_EFFECT_INTERRUPT_CAST on this player
)

// pvpEvent is one cross-session combat event for the owning session.
type pvpEvent struct {
	kind      pvpEventKind
	attacker  uint64 // attacker guid
	spell     uint32
	school    uint32
	damage    uint32
	blocked   uint32
	hitInfo   uint32
	lockoutMS uint32        // pvpInterrupt: school prohibition
	aura      *aura.Holder // pvpAura
}

// pvpVictimState is the registry entry of one in-world player.
type pvpVictimState struct {
	snap  pvpSnapshot
	inbox chan pvpEvent
}

// pvpRegistry maps player guids to their live combat state.
type pvpRegistry struct {
	mu sync.Mutex
	m  map[uint32]*pvpVictimState
}

// pvpLookup returns the current snapshot of an in-world player.
func (h *Handler) pvpLookup(guid uint32) (pvpSnapshot, bool) {
	h.pvp.mu.Lock()
	st, ok := h.pvp.m[guid]
	var snap pvpSnapshot
	if ok {
		snap = st.snap
	}
	h.pvp.mu.Unlock()
	return snap, ok
}

// pvpPush queues one event on the victim's inbox; full inboxes drop (the
// attacker's combat log already reached the clients either way).
func (h *Handler) pvpPush(guid uint32, ev pvpEvent) bool {
	h.pvp.mu.Lock()
	st, ok := h.pvp.m[guid]
	h.pvp.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case st.inbox <- ev:
		return true
	default:
		return false
	}
}

// pvpRegister publishes this session's player. Called with the world
// registration (Player::AddToMap / Register).
func (h *Handler) pvpRegister(s *Session) {
	p := s.player
	if p == nil {
		return
	}
	st := &pvpVictimState{inbox: make(chan pvpEvent, 128)}
	st.snap.guid = p.GUID
	st.snap.name = p.Name
	if race, ok := h.Data.Race(uint32(p.Race)); ok {
		st.snap.team = race.Team
	}
	st.snap.level, st.snap.race, st.snap.class = p.Level, p.Race, p.Class
	st.snap.defense = uint32(p.Level) * 5
	h.pvp.mu.Lock()
	if h.pvp.m == nil {
		h.pvp.m = map[uint32]*pvpVictimState{}
	}
	h.pvp.m[p.GUID] = st
	h.pvp.mu.Unlock()
	s.pvpInbox = st.inbox
	h.pvpUpdateSnapshot(s)
}

// pvpUnregister removes the player (logout).
func (h *Handler) pvpUnregister(s *Session) {
	if s.player == nil {
		return
	}
	h.pvp.mu.Lock()
	delete(h.pvp.m, s.player.GUID)
	h.pvp.mu.Unlock()
	s.pvpInbox = nil
}

// pvpUpdateSnapshot refreshes the live fields of this session's entry.
func (h *Handler) pvpUpdateSnapshot(s *Session) {
	if s.player == nil || s.pvpInbox == nil {
		return
	}
	p := s.player
	h.pvp.mu.Lock()
	st := h.pvp.m[p.GUID]
	if st != nil {
		st.snap.mapID, st.snap.x, st.snap.y, st.snap.z, st.snap.o = p.Map, p.X, p.Y, p.Z, p.Orientation
		st.snap.health = p.Health
		st.snap.ghost = p.PlayerFlags&playerFlagsGhost != 0
		if p.Stats != nil {
			st.snap.maxHealth = p.Stats.MaxHealth
			st.snap.armor = p.Stats.Resistances[0]
			for i := range p.Stats.Resistances {
				st.snap.resist[i] = int32(p.Stats.Resistances[i])
			}
			st.snap.dodgePct, st.snap.parryPct, st.snap.blockPct = p.Stats.DodgePct, p.Stats.ParryPct, p.Stats.BlockPct
		}
	}
	h.pvp.mu.Unlock()
}

// drainPvpEvents applies the queued combat events on the owning loop.
func (h *Handler) drainPvpEvents(s *Session, now time.Time) {
	if s.pvpInbox == nil {
		return
	}
	for {
		select {
		case ev := <-s.pvpInbox:
			h.applyPvpEvent(s, ev, now)
		default:
			return
		}
	}
}

// applyPvpEvent applies one attacker event: damage changes the authoritative
// health (and can kill, crediting the attacker), auras join this player's
// aura set and tick here.
func (h *Handler) applyPvpEvent(s *Session, ev pvpEvent, now time.Time) {
	p := s.player
	if p == nil || !p.Entered || p.Health == 0 || p.PlayerFlags&playerFlagsGhost != 0 {
		return
	}
	switch ev.kind {
	case pvpDamage:
		h.unitInCombat(s, now)
		// SPELL_INTERRUPT_FLAG_DAMAGE: damage cancels interruptible casts.
		if s.spells != nil {
			if cur := s.spells.cast; cur != nil && cur.info.InterruptFlags&spellInterruptFlagDamage != 0 {
				h.cancelCurrent(s)
			}
		}
		p.Health = sub(p.Health, ev.damage)
		h.sendFields(s, codec.FieldChange{Index: f.UNIT_FIELD_HEALTH, Value: p.Health})
		h.checkPlayerDeath(s, ev.attacker, now)
	case pvpInterrupt:
		h.interruptCastEvent(s, ev, now)
	case pvpAura:
		if ev.aura == nil {
			return
		}
		h.unitInCombat(s, now)
		sp := s.sp()
		sp.auraAt = nowIfZero(sp.auraAt, now)
		if _, ok := sp.auras.Add(ev.aura); !ok {
			return
		}
		h.syncAuraFields(s)
		_ = s.send(codec.UpdateAuraDuration(ev.aura.Slot, uint32(ev.aura.Duration)),
			codec.SetExtraAuraInfo(uint64(p.GUID), ev.aura.Slot, ev.aura.SpellID, uint32(ev.aura.MaxDuration), uint32(ev.aura.Duration)))
		h.afterAuraChange(s)
	}
}

// unitInCombat opens the combat window (UNIT_FLAG_IN_COMBAT,
// Unit::SetInCombatState) for a player that dealt or took damage.
func (h *Handler) unitInCombat(s *Session, now time.Time) {
	s.combatUntil = now.Add(creatureCombatTimer)
	h.combatFlagTick(s, now)
}

// combatFlagTick syncs the cached UNIT_FIELD_IN_COMBAT bit with the combat
// window and broadcasts changes.
func (h *Handler) combatFlagTick(s *Session, now time.Time) {
	if s.player == nil {
		return
	}
	want := uint32(codec.PlayerUnitFlags)
	if s.combat.active || now.Before(s.combatUntil) {
		want |= unitFlagInCombat
	}
	if want == s.unitFlags {
		return
	}
	s.unitFlags = want
	h.sendToSet(s, codec.ValuesUpdate(uint64(s.player.GUID), []codec.FieldChange{{Index: f.UNIT_FIELD_FLAGS, Value: want}}))
}

// attackPlayer ports Unit::Attack for a player victim.
func (h *Handler) attackPlayer(s *Session, guid uint64, now time.Time) {
	c := &s.combat
	p := s.player
	if c.active {
		if c.victim == guid {
			return // already meleeing this player
		}
		h.attackStop(s, true)
	}
	c.active, c.victim, c.spawn = true, guid, 0
	if c.nextSwing.IsZero() {
		c.nextSwing = now
	}
	c.swingErr = 0
	h.setTarget(s, guid)
	h.sendToSet(s, attackStartPacket(uint64(p.GUID), guid))
}

// combatTickPlayer is the player-victim branch of UpdateMeleeAttackingState.
func (h *Handler) combatTickPlayer(s *Session, now time.Time) {
	c := &s.combat
	p := s.player
	if !c.active || p == nil || p.Stats == nil {
		return
	}
	snap, ok := h.pvpLookup(uint32(c.victim))
	if !ok || snap.ghost || snap.health == 0 {
		h.combatStopVictimGone(s)
		return
	}
	h.unitInCombat(s, now)
	if s.nonMeleeCasting() || now.Before(c.nextSwing) {
		return
	}
	var swingErr uint8
	reach := float32(defaultCombatReach) + 1.5 + baseMeleeRangeOffset // both players: combat reach 1.5
	if reach < attackDistance {
		reach = attackDistance
	}
	dx, dy, dz := float64(snap.x-p.X), float64(snap.y-p.Y), float64(snap.z-p.Z)
	switch {
	case dx*dx+dy*dy+dz*dz > float64(reach)*float64(reach):
		c.nextSwing = now.Add(swingRetryDelay)
		swingErr = 1
	case !combat.HasInArc(p.X, p.Y, p.Orientation, snap.x, snap.y, arcFront):
		c.nextSwing = now.Add(swingRetryDelay)
		swingErr = 2
	default:
		if ms := s.sp().melee; ms != nil {
			s.sp().melee = nil
			h.castNow(s, now, ms)
		} else {
			h.meleeSwingAtPlayer(s, snap, now)
		}
		if c.active {
			c.nextSwing = now.Add(time.Duration(p.Stats.AttackTime[0]) * time.Millisecond)
		}
	}
	if swingErr != c.swingErr {
		switch swingErr {
		case 1:
			_ = s.send(world.Packet{Opcode: op.SMSG_ATTACKSWING_NOTINRANGE})
		case 2:
			_ = s.send(world.Packet{Opcode: op.SMSG_ATTACKSWING_BADFACING})
		}
		c.swingErr = swingErr
	}
}

// meleeSwingAtPlayer ports AttackerStateUpdate for a player main-hand swing
// at another player.
func (h *Handler) meleeSwingAtPlayer(s *Session, v pvpSnapshot, now time.Time) {
	p := s.player
	h.removeAurasWithInterrupt(s, auraInterruptMelee)
	level := uint32(p.Level)
	weaponSkill := h.playerWeaponSkill(p)
	att := combat.Side{Level: level, IsPlayer: true, Class: uint32(p.Class), WeaponSkill: weaponSkill, MaxSkill: level * 5}
	vic := combat.Side{Level: uint32(v.level), IsPlayer: true, Class: uint32(v.class), Defense: v.defense, MaxSkill: v.defense, Armor: v.armor}
	crit := h.Data.MeleeCritFromAgility(uint32(p.Class), level, p.Stats.Attributes[1]) + float32(s.sp().auras.Total(aura.ModCritPercent))
	rb := func(cr uint32) float32 { return h.Data.RatingBonus(cr, level, p.Stats.Ratings[cr]) }
	miss := combat.MissChance(weaponSkill, v.defense, true) - rb(gamedata.CRHitMelee)
	chances := combat.Chances{
		Miss:  int32(max(miss, 0) * 100),
		Crit:  int32(combat.CritChance(crit+rb(gamedata.CRCritMelee), att.MaxSkill, v.defense) * 100),
		Dodge: int32(v.dodgePct * 100),
		Parry: int32(v.parryPct * 100),
		Block: int32(v.blockPct * 100),
	}
	fromBehind := !combat.HasInArc(v.x, v.y, v.o, p.X, p.Y, math.Pi)
	caster := false
	switch p.Class {
	case 5, 7, 8, 9, 11:
		caster = true
	}
	flat, pct := h.physicalAuraMods(s)
	res := combat.Calculate(rng{}, att, vic, chances, fromBehind, (p.Stats.DamageMin[0]+flat)*pct, (p.Stats.DamageMax[0]+flat)*pct, caster, int32(v.defense)-int32(weaponSkill))
	if res.Absorb > 0 {
		res.HitInfo |= combat.HitInfoAbsorb
	}
	h.sendToSet(s, attackerStateUpdatePacket(uint64(p.GUID), s.combat.victim, res))
	if res.Damage > 0 {
		h.pvpPush(uint32(s.combat.victim), pvpEvent{kind: pvpDamage, attacker: uint64(p.GUID), school: schoolMaskNormal, damage: res.Damage, blocked: res.Blocked, hitInfo: res.HitInfo})
		if p.Class == 1 && s.combat.active {
			factor := combat.RageWeaponFactor(p.Stats.AttackTime[0], res.Outcome == combat.HitCrit)
			add := combat.RageReward(level, res.Damage, factor, 1)
			if add > 0 {
				p.Rage = min(p.Rage+add, 1000)
				_ = s.send(codec.ValuesUpdate(uint64(p.GUID), []codec.FieldChange{{Index: f.UNIT_FIELD_POWER2, Value: p.Rage}}))
			}
		}
	}
}

// spellHitResultPlayer ports Unit::SpellHitResult for a player victim.
func (h *Handler) spellHitResultPlayer(s *Session, st *castState, v pvpSnapshot) combat.SpellMiss {
	info, p := st.info, s.player
	level := uint32(p.Level)
	switch info.DefenseType {
	case gamedata.DamageClassMagic:
		bonus := s.sp().auras.Total(aura.ModSpellHitChance) + int32(h.Data.RatingBonus(gamedata.CRHitSpell, level, p.Stats.Ratings[gamedata.CRHitSpell]))
		return combat.MagicHit(rng{}, level, uint32(v.level), true, bonus, 0, info.AttributesExC&spellAttrExCCantMiss != 0)
	case gamedata.DamageClassMelee, gamedata.DamageClassRanged:
		in := h.meleeSpellPlayerInput(s, info, v)
		return combat.MeleeSpellHit(rng{}, in)
	}
	return combat.SpellMissNone
}

// meleeSpellPlayerInput builds the yellow-hit roll input against a player.
func (h *Handler) meleeSpellPlayerInput(s *Session, info *gamedata.SpellInfo, v pvpSnapshot) combat.MeleeSpellInput {
	p := s.player
	level := uint32(p.Level)
	skill := int32(level * 5)
	if info.EquippedItemClass == 2 {
		skill = int32(h.playerWeaponSkill(p))
	}
	ranged := info.DefenseType == gamedata.DamageClassRanged
	mod := float32(s.sp().auras.Total(aura.ModHitChance))
	if ranged {
		mod += h.Data.RatingBonus(gamedata.CRHitRanged, level, p.Stats.Ratings[gamedata.CRHitRanged])
	} else {
		mod += h.Data.RatingBonus(gamedata.CRHitMelee, level, p.Stats.Ratings[gamedata.CRHitMelee])
	}
	exp := h.Data.RatingBonus(gamedata.CRExpertise, level, p.Stats.Ratings[gamedata.CRExpertise])
	return combat.MeleeSpellInput{
		AttackerSkill:  skill,
		VictimMaxSkill: int32(v.defense),
		VictimDefense:  int32(v.defense),
		VictimIsPlayer: true,
		BothPlayers:    true,
		ModHit:         mod,
		CantMiss:       info.AttributesExC&spellAttrExCCantMiss != 0,
		NoDodgeParry:   info.Attributes&spellAttrNoDodgeParryBlock != 0,
		Ranged:         ranged,
		FromBehind:     !combat.HasInArc(v.x, v.y, v.o, p.X, p.Y, math.Pi),
		DodgePct:       v.dodgePct - exp,
		ParryPct:       v.parryPct - exp,
	}
}

// spellCritChancePlayer is the crit chance of the caster against a player
// victim (the victim's attacker-crit auras are not visible cross-session).
func (h *Handler) spellCritChancePlayer(s *Session, info *gamedata.SpellInfo, v pvpSnapshot) float32 {
	p := s.player
	var chance float32
	auras := &s.sp().auras
	switch info.DefenseType {
	case gamedata.DamageClassMagic:
		if info.SchoolMask&schoolMaskNormal != 0 {
			return 0
		}
		chance = h.Data.SpellCritFromIntellect(uint32(p.Class), uint32(p.Level), p.Stats.Attributes[3])
		chance += h.Data.RatingBonus(gamedata.CRCritSpell, uint32(p.Level), p.Stats.Ratings[gamedata.CRCritSpell])
		chance += float32(auras.Total(aura.ModSpellCritChance))
		chance += float32(auras.TotalByMisc(aura.ModSpellCritChanceSchool, info.SchoolMask))
	case gamedata.DamageClassMelee, gamedata.DamageClassRanged:
		chance = h.meleeCritChanceAgainstPlayer(s, v)
		chance += float32(auras.TotalByMisc(aura.ModSpellCritChanceSchool, info.SchoolMask))
	default:
		return 0
	}
	return max(chance, 0)
}

// meleeCritChanceAgainstPlayer is GetUnitCriticalChance against a player.
func (h *Handler) meleeCritChanceAgainstPlayer(s *Session, v pvpSnapshot) float32 {
	p := s.player
	level := uint32(p.Level)
	crit := h.Data.MeleeCritFromAgility(uint32(p.Class), level, p.Stats.Attributes[1]) + h.Data.RatingBonus(gamedata.CRCritMelee, level, p.Stats.Ratings[gamedata.CRCritMelee])
	if s.spells != nil {
		crit += float32(s.spells.auras.Total(aura.ModCritPercent))
	}
	return combat.CritChance(crit, level*5, v.defense)
}

// calcSpellDamagePlayer ports Unit::CalculateSpellDamage for a player victim.
func (h *Handler) calcSpellDamagePlayer(s *Session, info *gamedata.SpellInfo, v pvpSnapshot, damage int32, weaponBased bool) spellDamage {
	var out spellDamage
	phys := info.DefenseType == gamedata.DamageClassMelee || info.DefenseType == gamedata.DamageClassRanged
	crit := info.AttributesExB&spellAttrExBCantCrit == 0 && rollChance(h.spellCritChancePlayer(s, info, v))
	damage = h.bonusDone(s, info, damage, false, weaponBased)
	if damage < 0 {
		damage = 0
	}
	d := uint32(damage)
	if crit {
		out.hitInfo |= combat.SpellHitCrit
		d = combat.CritDamage(d, phys, 1)
	}
	if d > 0 && info.SchoolMask&schoolMaskNormal != 0 {
		d = combat.ArmorReduced(uint32(s.player.Level), v.armor, d)
	}
	out.damage = d
	return out
}

// absorbResistBlockPlayer is CalculateAbsorbResistBlock for a player victim
// (no shields/absorbs are tracked cross-session; the resist roll uses the
// victim's published resistances).
func (h *Handler) absorbResistBlockPlayer(s *Session, info *gamedata.SpellInfo, v pvpSnapshot, d *spellDamage, dot bool) {
	if info.SchoolMask&schoolMaskNormal == 0 && d.damage > 0 {
		res := int32(0)
		for school := 1; school < 7; school++ {
			if info.SchoolMask&(1<<school) != 0 {
				res = v.resist[school]
				break
			}
		}
		d.resist = combat.ResistedDamage(rng{}, float32(res), uint32(v.level), d.damage, dot)
		d.damage -= d.resist
	}
}

// doAllEffectOnPlayerTarget ports Spell::DoAllEffectOnTarget for a player
// victim: the aura application and the damage become events on the victim's
// session; the combat log packet is broadcast from here.
func (h *Handler) doAllEffectOnPlayerTarget(s *Session, now time.Time, st *castState, t *spellTarget) {
	info, p := st.info, s.player
	caster := uint64(p.GUID)
	snap, ok := h.pvpLookup(uint32(t.guid))
	if !ok || snap.ghost || snap.health == 0 {
		return
	}
	if t.miss != combat.SpellMissNone {
		return
	}
	var effects [3]*aura.Effect
	hasAura := false
	var damage int32
	weaponBased := false
	weaponDone := false
	for i := 0; i < 3; i++ {
		if t.effectMask&(1<<i) == 0 {
			continue
		}
		switch info.Effect[i] {
		case gamedata.EffectSchoolDamage:
			if !t.launched {
				v := spell.EffectValue(info, i, uint32(p.Level), irand)
				v = spell.ComboScaled(info, i, v, int32(s.sp().comboPoints))
				damage += v
			}
		case gamedata.EffectAddComboPoints:
			if !t.launched {
				h.addComboPoints(s, t.guid, uint8(max(spell.EffectValue(info, i, uint32(p.Level), irand), 1)))
			}
		case gamedata.EffectInterruptCast:
			if !t.launched {
				lockout := uint32(max(h.Data.DurationMS(info), 0))
				h.pvpPush(uint32(t.guid), pvpEvent{kind: pvpInterrupt, attacker: caster, spell: info.ID, lockoutMS: lockout})
			}
		case gamedata.EffectLeap:
			if !t.launched {
				h.effectLeap(s, t.guid, snap.x, snap.y, snap.z)
			}
		case gamedata.EffectWeaponDamageNoSchool, gamedata.EffectWeaponDamage, gamedata.EffectNormalizedWeaponDmg, gamedata.EffectWeaponPercentDamage:
			weaponBased = true
			if !t.launched && !weaponDone {
				weaponDone = true
				damage += h.effectWeaponDamage(s, info, t.effectMask)
			}
		case gamedata.EffectApplyAura:
			if effectTargetsEnemy(info, i) {
				e := &aura.Effect{Name: info.Aura[i], Amount: spell.EffectValue(info, i, uint32(p.Level), irand), Misc: info.MiscValue[i]}
				if isPeriodic(e.Name) {
					e.Period = int32(info.AuraPeriod[i])
				}
				if e.Name == aura.PeriodicDamage {
					e.Amount = h.bonusDone(s, info, e.Amount, true, false)
				}
				effects[i] = e
				hasAura = true
			}
		default:
			h.warnOnce(s, info.ID<<2|uint32(i), "spell effect not implemented", info.ID, info.Effect[i])
		}
	}
	if t.launched {
		damage = int32(t.damage)
	}
	if hasAura {
		hd := aura.NewHolder(info.ID, caster, uint8(p.Level), h.Data.DurationMS(info), false, effects)
		hd.InterruptFlags = info.AuraInterruptFlags
		h.pvpPush(uint32(t.guid), pvpEvent{kind: pvpAura, attacker: caster, spell: info.ID, aura: hd})
	}
	if damage <= 0 {
		return
	}
	var d spellDamage
	if t.launched {
		d = spellDamage{damage: uint32(damage), hitInfo: t.hitInfo}
	} else {
		d = h.calcSpellDamagePlayer(s, info, snap, damage, weaponBased)
	}
	h.absorbResistBlockPlayer(s, info, snap, &d, false)
	h.sendToSet(s, codec.SpellNonMeleeDamageLog(t.guid, caster, info.ID, d.damage, uint8(info.SchoolMask), d.absorb, d.resist, false, d.blocked, d.hitInfo))
	if d.damage > 0 {
		h.pvpPush(uint32(t.guid), pvpEvent{kind: pvpDamage, attacker: caster, spell: info.ID, school: info.SchoolMask, damage: d.damage, blocked: d.blocked, hitInfo: d.hitInfo})
	}
}
