package tbc

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	"wowserver/internal/domain/aura"
	"wowserver/internal/domain/character"
	"wowserver/internal/domain/combat"
	domainworld "wowserver/internal/domain/world"
	"wowserver/internal/domain/spell"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
	f "wowserver/internal/protocol/world/tbc/updatefields"
)

// Player melee against creatures, ported from Unit::Attack / AttackStop /
// UpdateMeleeAttackingState / AttackerStateUpdate and WorldSession's
// HandleAttackSwing/StopOpcode. Creatures do not fight back yet (no AI,
// threat or chase), and kills give no XP or loot.

const (
	attackDisplayDelay   = 200 * time.Millisecond // ATTACK_DISPLAY_DELAY
	swingRetryDelay      = 100 * time.Millisecond // setAttackTimer(.., 100) on swing errors
	attackDistance       = 5.0                    // ATTACK_DISTANCE
	baseMeleeRangeOffset = 1.33                   // BASE_MELEERANGE_OFFSET
	defaultCombatReach   = 1.5                    // player UNIT_FIELD_COMBATREACH
	unitFlagNonAttack    = 0x00000002
	unitFlagNotSelect    = 0x02000000
	unitFlagInCombat     = 0x00080000 // UNIT_FLAG_IN_COMBAT (Unit.h)
	// creatureCombatTimer approximates how long a creature stays in combat
	// after the last damage it took. The reference sets the 5000 ms combat
	// timer in Unit::SetInCombatState and holds creature combat open through
	// its hostile references (threat); with no AI/threat in this server the
	// timer alone decides when combat ends.
	creatureCombatTimer = 5 * time.Second
	// creatureRegenInterval is REGEN_TIME_FULL (Unit.h): Creature::RegenerateAll
	// runs every 2 s.
	creatureRegenInterval = 2 * time.Second
	extraNoParry         = 0x00000004
	extraNoBlock         = 0x00000010
	extraNoCrush         = 0x00000020
	creatureTypeHumanoid = 7
	auraInterruptMelee   = 0x00001000 // AURA_INTERRUPT_FLAG_MELEE_ATTACK
	skillUnarmed         = 162
	arcFront             = 2 * math.Pi / 3
)

// weaponSkills is item_weapon_skills[] of Item::GetSkill by weapon subclass.
var weaponSkills = [21]uint32{44, 172, 45, 46, 54, 160, 229, 43, 55, 0, 136, 0, 0, skillUnarmed, 0, 173, 176, 253, 226, 228, 356}

// Team ids of ChrRaces.BaseLanguage (Player::TeamForRace).
const (
	teamAlliance = 7
	teamHorde    = 1
)

// pvpNotImplementedKey is the warnOnce key for rejected player-victim
// attacks (cross-session PvP combat is not implemented yet).
const pvpNotImplementedKey = 0x7FFFFFFF

// playerHostileToPlayer ports the player-vs-player branch of
// Unit::IsHostileTo: players of the same team are never hostile ("Green/
// Blue — can't attack"), players of opposing teams are hostile while both
// are flagged for PvP ("Red — can attack"). Every player of this server is
// spawned with UNIT_FLAG_PVP (PvP realm rules), so the flag condition is
// always true and the rule reduces to the team comparison. Duel, FFA and
// sanctuary states do not exist here.
func playerHostileToPlayer(attacker, victim uint32) bool {
	if attacker == victim || attacker == 0 || victim == 0 {
		return false // same team, or unknown team (TEAM_NONE): not attackable
	}
	return true
}

// playerTargetHostility resolves a player-victim attack target: found is
// true when guid names another in-world player near the session's player,
// and hostile then reports whether the attack is allowed by the team rules
// (the IsFriendlyTo/team part of WorldSession::HandleAttackSwingOpcode).
func (h *Handler) playerTargetHostility(s *Session, guid uint64) (hostile, found bool) {
	if h.Broadcast == nil || s.player == nil {
		return false, false
	}
	race, ok := h.Data.Race(uint32(s.player.Race))
	if !ok {
		return false, false
	}
	for _, np := range h.Broadcast.Nearby(s.player.GUID) {
		if guid != uint64(np.GUID) {
			continue
		}
		return playerHostileToPlayer(race.Team, np.Team), true
	}
	return false, false
}

// combatState is the per-session melee state (Unit::m_attacking and the
// BASE_ATTACK timer).
type combatState struct {
	active    bool
	victim    uint64
	spawn     uint32
	nextSwing time.Time
	swingErr  uint8
}

// creatureLife holds the mutable state of one creature spawn: damage taken,
// death and the respawn schedule (Creature::m_corpseRemoveTime/m_respawnTime),
// plus the combat window (Unit::SetInCombatState m_CombatTimer) and the
// out-of-combat health regeneration of Creature::RegenerateAll.
type creatureLife struct {
	spawn     gamedata.CreatureSpawnRef
	entry     uint32 // template entry (creature GUID rebuilding)
	unitFlags uint32 // template UNIT_FIELD_FLAGS; flag updates are absolute
	health    uint32
	maxHealth uint32
	dead      bool
	hidden    bool // corpse already removed from the world
	// combat is the client-visible UNIT_FLAG_IN_COMBAT state; combatUntil is
	// when the combat window closes (last damage + creatureCombatTimer).
	combat      bool
	combatUntil time.Time
	regenAt     time.Time // next Creature::RegenerateAll run
	decayAt     time.Time
	respawnAt   time.Time
}

// lifeEvent is a lock-free snapshot for broadcasting a life-state change.
type lifeEvent struct {
	ref       gamedata.CreatureSpawnRef
	entry     uint32
	unitFlags uint32
	health    uint32
}

type lifeRegistry struct {
	mu       sync.Mutex
	m        map[uint32]*creatureLife
	lastTick time.Time
}

// corpseDecay is Corpse.Decay.* of mangosd.conf by creature rank.
func corpseDecay(rank uint32) time.Duration {
	switch rank {
	case 1:
		return 600 * time.Second // ELITE
	case 2:
		return 1200 * time.Second // RAREELITE
	case 3:
		return 3600 * time.Second // WORLDBOSS
	case 4:
		return 900 * time.Second // RARE
	}
	return 300 * time.Second // NORMAL
}

type rng struct{}

func (rng) Urand(lo, hi uint32) uint32 {
	if hi <= lo {
		return lo
	}
	return lo + uint32(rand.Int63n(int64(hi-lo)+1))
}
func (rng) Norm() float32 { return rand.Float32() }

func (h *Handler) registerCombatRoutes(add func(uint32, handlerFunc)) {
	add(op.CMSG_ATTACKSWING, h.attackSwing)
	add(op.CMSG_ATTACKSTOP, h.attackStopHandler)
}

// creatureView is a resolved, live creature as combat sees it.
type creatureView struct {
	spawn   gamedata.ResolvedCreatureSpawn
	ref     gamedata.CreatureSpawnRef
	info    entityCreature
	x, y, z float32
	o       float32
	alive   bool
	health  uint32
}

type entityCreature struct {
	Armor, ExtraFlags, Type, Rank uint32
	Resist                        [7]int32 // holy, fire, nature, frost, shadow, arcane at 1..6
}

// lookupCreature resolves a creature guid on the player's map.
func (h *Handler) lookupCreature(s *Session, guid uint64, now time.Time) (creatureView, bool) {
	var v creatureView
	entry, counter, ok := codec.DecodeCreatureGUID(guid)
	if !ok || s.player == nil {
		return v, false
	}
	ref, ok := h.Data.CreatureSpawnByGUID(counter)
	if !ok || ref.Data.ID != entry || ref.Data.MapID != s.player.Map {
		return v, false
	}
	spawn, ok := h.Data.ResolveCreatureSpawn(ref)
	if !ok {
		return v, false
	}
	v.spawn, v.ref = spawn, ref
	if t, ok := h.Data.CreatureTemplate(entry); ok {
		v.info = entityCreature{Armor: t.Armor, ExtraFlags: t.ExtraFlags, Type: t.CreatureType, Rank: t.Rank,
			Resist: [7]int32{0, t.ResistanceHoly, t.ResistanceFire, t.ResistanceNature, t.ResistanceFrost, t.ResistanceShadow, t.ResistanceArcane}}
	}
	v.x, v.y, v.z, v.o = spawn.X, spawn.Y, spawn.Z, spawn.Orientation
	if cx, cy, cz, co, ok := h.currentCreaturePosition(counter, now); ok {
		v.x, v.y, v.z, v.o = cx, cy, cz, co
	}
	v.health, v.alive = h.creatureHealth(ref, spawn, now)
	return v, true
}

// creatureHealth returns the current health and whether the creature is
// alive and in the world.
func (h *Handler) creatureHealth(ref gamedata.CreatureSpawnRef, spawn gamedata.ResolvedCreatureSpawn, now time.Time) (uint32, bool) {
	l := &h.life
	l.mu.Lock()
	defer l.mu.Unlock()
	if c, ok := l.m[ref.GUID]; ok {
		if c.dead {
			return 0, false
		}
		return c.health, true
	}
	return spawn.Health, spawn.Health > 0
}

func (h *Handler) attackSwing(_ context.Context, s *Session, pk world.Packet) (*world.Packet, error) {
	r := wire.NewReader(pk.Payload)
	guid := r.Uint64()
	if err := r.Finish(); err != nil {
		return nil, err
	}
	p := s.player
	if p == nil || !p.Entered {
		return nil, nil
	}
	now := time.Now()
	// Player victims (PvP): validate the team like the reference does, then
	// stop the attack — cross-session player combat (damage, death, release)
	// is not implemented yet. Player guids are low guids, so a zero high part
	// that no creature matches is another player.
	if guid>>32 == 0 {
		if hostile, found := h.playerTargetHostility(s, guid); found {
			if !hostile {
				s.log.Debug("attack on same-team player rejected", "target", guid)
			} else {
				h.warnOnce(s, pvpNotImplementedKey, "player-versus-player combat is not implemented; attack rejected", 0, 0)
			}
			_ = s.send(attackStopPacket(uint64(p.GUID), guid))
			return nil, nil
		}
	}
	v, ok := h.lookupCreature(s, guid, now)
	if !ok {
		// Enemy not found (or not a creature): stop the attack state at the client.
		_ = s.send(attackStopPacket(uint64(p.GUID), 0))
		return nil, nil
	}
	if h.creatureFriendlyOrUnattackable(s, v) || !v.alive {
		_ = s.send(attackStopPacket(uint64(p.GUID), guid))
		return nil, nil
	}
	h.attack(s, v, guid, now)
	return nil, nil
}

func (h *Handler) attackStopHandler(_ context.Context, s *Session, _ world.Packet) (*world.Packet, error) {
	h.attackStop(s, false)
	return nil, nil
}

// creatureFriendlyOrUnattackable: IsFriendlyTo || NON_ATTACKABLE|NOT_SELECTABLE.
func (h *Handler) creatureFriendlyOrUnattackable(s *Session, v creatureView) bool {
	if v.spawn.UnitFlags&(unitFlagNonAttack|unitFlagNotSelect) != 0 {
		return true
	}
	race, ok := h.Data.Race(uint32(s.player.Race))
	if !ok {
		return false
	}
	return h.Data.CreatureFriendlyToPlayer(v.spawn.Faction, race.Faction, h.playerReputation(s.player))
}

// attack ports Unit::Attack(victim, meleeAttack=true).
func (h *Handler) attack(s *Session, v creatureView, guid uint64, now time.Time) {
	c := &s.combat
	p := s.player
	if c.active {
		if c.victim == guid {
			return // already meleeing this target
		}
		h.attackStop(s, true)
	}
	c.active, c.victim, c.spawn = true, guid, v.ref.GUID
	if c.nextSwing.IsZero() {
		c.nextSwing = now
	}
	c.swingErr = 0
	h.setTarget(s, guid)
	h.sendToSet(s, attackStartPacket(uint64(p.GUID), guid))
}

// attackStop ports Unit::AttackStop.
func (h *Handler) attackStop(s *Session, targetSwitch bool) {
	c := &s.combat
	if !c.active || s.player == nil {
		return
	}
	victim := c.victim
	if old := s.sp().melee; old != nil { // InterruptSpell(CURRENT_MELEE_SPELL)
		s.sp().melee = nil
		caster := uint64(s.player.GUID)
		_ = s.send(codec.SpellFailure(caster, old.info.ID, 0), codec.SpellFailedOther(caster, old.info.ID))
		_ = s.send(codec.CastFailed(old.info.ID, spell.FailedInterrupted, old.count))
	}
	*c = combatState{nextSwing: c.nextSwing}
	h.setTarget(s, 0)
	h.sendToSet(s, attackStopPacket(uint64(s.player.GUID), victim))
}

func (h *Handler) setTarget(s *Session, guid uint64) {
	ch := []codec.FieldChange{
		{Index: f.UNIT_FIELD_TARGET, Value: uint32(guid)},
		{Index: f.UNIT_FIELD_TARGET + 1, Value: uint32(guid >> 32)},
	}
	h.sendToSet(s, codec.ValuesUpdate(uint64(s.player.GUID), ch))
}

// sendToSet is SendMessageToSet(&data, true).
func (h *Handler) sendToSet(s *Session, pk world.Packet) {
	if h.Broadcast != nil {
		h.Broadcast.Broadcast(s.player.GUID, domainworld.Packet{Opcode: pk.Opcode, Payload: pk.Payload}, true)
		return
	}
	_ = s.send(pk)
}

func (s *Session) nonMeleeCasting() bool {
	return s.spells != nil && s.spells.cast != nil
}

// combatTick ports Unit::UpdateMeleeAttackingState for the main hand.
func (h *Handler) combatTick(s *Session, now time.Time) {
	c := &s.combat
	p := s.player
	if !c.active || p == nil || p.Stats == nil {
		return
	}
	v, ok := h.lookupCreature(s, c.victim, now)
	if !ok || !v.alive {
		h.combatStopVictimGone(s)
		return
	}
	if s.nonMeleeCasting() || now.Before(c.nextSwing) {
		return
	}
	var swingErr uint8
	reach := float32(defaultCombatReach) + v.spawn.CombatReach + baseMeleeRangeOffset
	if reach < attackDistance {
		reach = attackDistance
	}
	dx, dy, dz := float64(v.x-p.X), float64(v.y-p.Y), float64(v.z-p.Z)
	switch {
	case dx*dx+dy*dy+dz*dz > float64(reach)*float64(reach):
		c.nextSwing = now.Add(swingRetryDelay)
		swingErr = 1
	case !combat.HasInArc(p.X, p.Y, p.Orientation, v.x, v.y, arcFront):
		c.nextSwing = now.Add(swingRetryDelay)
		swingErr = 2
	default:
		if ms := s.sp().melee; ms != nil {
			s.sp().melee = nil
			h.castNow(s, now, ms)
		} else {
			h.meleeSwing(s, v, now)
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

// combatStopVictimGone is Unit::CombatStop for an attacker whose victim died
// or vanished: AttackStop plus SMSG_CANCEL_COMBAT.
func (h *Handler) combatStopVictimGone(s *Session) {
	h.attackStop(s, false)
	_ = s.send(world.Packet{Opcode: op.SMSG_CANCEL_COMBAT})
}

// meleeSwing ports AttackerStateUpdate for a player main-hand swing at a creature.
func (h *Handler) meleeSwing(s *Session, v creatureView, now time.Time) {
	p := s.player
	h.removeAurasWithInterrupt(s, auraInterruptMelee)
	level := uint32(p.Level)
	weaponSkill := h.playerWeaponSkill(p)
	vlevel := v.spawn.Level
	att := combat.Side{Level: level, IsPlayer: true, Class: uint32(p.Class), WeaponSkill: weaponSkill, MaxSkill: level * 5}
	vic := combat.Side{Level: vlevel, Defense: vlevel * 5, MaxSkill: vlevel * 5, Armor: h.victimArmor(v, h.foeSet(s, v)),
		NoParry: v.info.ExtraFlags&extraNoParry != 0, NoBlock: v.info.ExtraFlags&extraNoBlock != 0, NoCrush: v.info.ExtraFlags&extraNoCrush != 0}
	crit := h.Data.MeleeCritFromAgility(uint32(p.Class), level, p.Stats.Attributes[1]) + float32(s.sp().auras.Total(aura.ModCritPercent))
	chances := combat.Chances{
		Miss:  int32(combat.MissChance(weaponSkill, vic.Defense, false) * 100),
		Crit:  int32(combat.CritChance(crit, att.MaxSkill, vic.Defense) * 100),
		Dodge: 500,
		Block: 500,
	}
	if v.info.Type == creatureTypeHumanoid {
		chances.Parry = 500
	}
	fromBehind := !combat.HasInArc(v.x, v.y, v.o, p.X, p.Y, math.Pi)
	caster := false
	switch p.Class {
	case 5, 7, 8, 9, 11:
		caster = true
	}
	flat, pct := h.physicalAuraMods(s)
	res := combat.Calculate(rng{}, att, vic, chances, fromBehind, (p.Stats.DamageMin[0]+flat)*pct, (p.Stats.DamageMax[0]+flat)*pct, caster, int32(vic.Defense)-int32(weaponSkill))
	if res.Absorb > 0 {
		res.HitInfo |= combat.HitInfoAbsorb
	}
	h.sendToSet(s, attackerStateUpdatePacket(uint64(p.GUID), codec.CreatureGUID(v.ref.Data.ID, v.ref.GUID), res))
	if res.Damage > 0 {
		h.dealCreatureDamage(s, v, res.Damage, now)
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

// playerWeaponSkill is GetWeaponSkillValue(BASE_ATTACK) against a creature.
func (h *Handler) playerWeaponSkill(p *character.Player) uint32 {
	skill := uint32(skillUnarmed)
	for _, it := range p.Items {
		if it.Bag != 0 || it.Slot != 15 {
			continue
		}
		if d, ok := h.Data.ItemPrototype(it.Entry); ok && d.Class == 2 && int(d.SubClass) < len(weaponSkills) && weaponSkills[d.SubClass] != 0 {
			skill = weaponSkills[d.SubClass]
		}
	}
	for _, sk := range p.Skills {
		if sk.ID == skill {
			return sk.Value
		}
	}
	return 0
}

// dealCreatureDamage applies damage (Unit::DealDamage on a creature victim).
func (h *Handler) dealCreatureDamage(s *Session, v creatureView, damage uint32, now time.Time) {
	l := &h.life
	l.mu.Lock()
	if l.m == nil {
		l.m = map[uint32]*creatureLife{}
	}
	c, ok := l.m[v.ref.GUID]
	if !ok {
		c = &creatureLife{spawn: v.ref, entry: v.ref.Data.ID, unitFlags: v.spawn.UnitFlags,
			health: v.spawn.Health, maxHealth: v.spawn.Health}
		l.m[v.ref.GUID] = c
	}
	if c.dead {
		l.mu.Unlock()
		return
	}
	wasCombat := c.combat
	killed := damage >= c.health
	if killed {
		c.health = 0
		c.dead = true
		c.combat = false // Unit::SetDeathState: CombatStop clears the flag
		c.decayAt = now.Add(corpseDecay(v.info.Rank))
		c.respawnAt = now.Add(time.Duration(v.ref.Data.SpawnTimeSecs) * time.Second)
	} else {
		c.health -= damage
		// Unit::SetInCombatState: damage refreshes the combat window and sets
		// UNIT_FLAG_IN_COMBAT while the creature is alive.
		c.combatUntil = now.Add(creatureCombatTimer)
		c.combat = true
	}
	health, combat, unitFlags := c.health, c.combat, c.unitFlags
	l.mu.Unlock()
	guid := codec.CreatureGUID(v.ref.Data.ID, v.ref.GUID)
	if wasCombat != combat {
		// Flag updates are absolute: the template flags with or without the
		// combat bit, so every observer's UNIT_FIELD_FLAGS stays consistent.
		flags := unitFlags
		if combat {
			flags |= unitFlagInCombat
		}
		h.broadcastCreatureLife(v.ref, v.x, v.y, v.z,
			codec.ValuesUpdate(guid, []codec.FieldChange{{Index: f.UNIT_FIELD_FLAGS, Value: flags}}))
	}
	pkt := codec.ValuesUpdate(guid, []codec.FieldChange{{Index: f.UNIT_FIELD_HEALTH, Value: health}})
	if h.Broadcast != nil {
		h.Broadcast.BroadcastArea(v.ref.Data.MapID, v.x, v.y, v.z, motionBroadcast, domainworld.Packet{Opcode: pkt.Opcode, Payload: pkt.Payload})
	} else {
		_ = s.send(pkt)
	}
	if killed {
		h.stopCreatureMoving(v.ref.GUID)
		// Unit::RemoveAllAurasOnDeath: the corpse loses every aura this
		// player put on the creature (cleared fields are broadcast).
		h.clearFoeAuras(s, v)
		if s.combat.active && s.combat.spawn == v.ref.GUID {
			h.combatStopVictimGone(s)
		}
	}
}

// broadcastCreatureLife broadcasts a values update of one creature at a
// given position to everyone around it (SendMessageToSet).
func (h *Handler) broadcastCreatureLife(ref gamedata.CreatureSpawnRef, x, y, z float32, pkt world.Packet) {
	if h.Broadcast == nil {
		return
	}
	h.Broadcast.BroadcastArea(ref.Data.MapID, x, y, z, motionBroadcast, domainworld.Packet{Opcode: pkt.Opcode, Payload: pkt.Payload})
}

// creatureLivePosition is the position to broadcast a creature update at:
// the mover's current position, falling back to the spawn point.
func (h *Handler) creatureLivePosition(ref gamedata.CreatureSpawnRef, now time.Time) (x, y, z float32) {
	if lx, ly, lz, _, ok := h.currentCreaturePosition(ref.GUID, now); ok {
		return lx, ly, lz
	}
	return ref.Data.PosX, ref.Data.PosY, ref.Data.PosZ
}

// tickCreatureLife removes decayed corpses and respawns creatures
// (Creature::Update in the DEAD/CORPSE states). It is throttled to 1 s.
func (h *Handler) tickCreatureLife(now time.Time) {
	l := &h.life
	l.mu.Lock()
	if now.Sub(l.lastTick) < time.Second || len(l.m) == 0 {
		l.mu.Unlock()
		return
	}
	l.lastTick = now
	var gone, back []*creatureLife
	var regen, leftCombat []lifeEvent
	snapshot := func(c *creatureLife) lifeEvent {
		return lifeEvent{ref: c.spawn, entry: c.entry, unitFlags: c.unitFlags, health: c.health}
	}
	for id, c := range l.m {
		switch {
		case c.dead && !now.Before(c.respawnAt):
			back = append(back, c)
			delete(l.m, id)
		case c.dead && !c.hidden && !now.Before(c.decayAt):
			c.hidden = true
			gone = append(gone, c)
		case c.dead:
			// corpse still lying; no regeneration
		case c.combat && !now.Before(c.combatUntil):
			// Unit::ClearInCombat: the combat window closed without new damage.
			c.combat = false
			c.regenAt = now.Add(creatureRegenInterval)
			leftCombat = append(leftCombat, snapshot(c))
		case !c.combat && c.health < c.maxHealth && !now.Before(c.regenAt):
			// Creature::RegenerateAll/RegenerateHealth for a non-controlled
			// creature: addvalue = maxValue / 3 per tick, every 2 s. Mana never
			// drops in this server (creatures do not cast), so power regen is
			// a no-op and not simulated.
			c.health = min(c.health+c.maxHealth/3, c.maxHealth)
			c.regenAt = now.Add(creatureRegenInterval)
			regen = append(regen, snapshot(c))
		}
	}
	l.mu.Unlock()
	for _, e := range leftCombat {
		x, y, z := h.creatureLivePosition(e.ref, now)
		h.broadcastCreatureLife(e.ref, x, y, z, codec.ValuesUpdate(codec.CreatureGUID(e.entry, e.ref.GUID),
			[]codec.FieldChange{{Index: f.UNIT_FIELD_FLAGS, Value: e.unitFlags}}))
	}
	for _, e := range regen {
		x, y, z := h.creatureLivePosition(e.ref, now)
		h.broadcastCreatureLife(e.ref, x, y, z, codec.ValuesUpdate(codec.CreatureGUID(e.entry, e.ref.GUID),
			[]codec.FieldChange{{Index: f.UNIT_FIELD_HEALTH, Value: e.health}}))
	}
	for _, c := range gone {
		h.broadcastCreature(c.spawn, codec.DestroyObjectGUID(codec.CreatureGUID(c.spawn.Data.ID, c.spawn.GUID)), now)
	}
	for _, c := range back {
		guid := codec.CreatureGUID(c.spawn.Data.ID, c.spawn.GUID)
		if !c.hidden {
			h.broadcastCreature(c.spawn, codec.DestroyObjectGUID(guid), now)
		}
		if h.Data == nil {
			continue
		}
		if r, ok := h.Data.ResolveCreatureSpawn(c.spawn); ok {
			h.broadcastCreature(c.spawn, codec.CreateObjectForCreature(r, uint32(now.UnixMilli())), now)
		}
		// The respawned creature wanders from its spawn point again.
		h.resetMover(c.spawn.GUID, now)
	}
}

func (h *Handler) broadcastCreature(ref gamedata.CreatureSpawnRef, pk world.Packet, now time.Time) {
	if h.Broadcast == nil {
		return
	}
	h.Broadcast.BroadcastArea(ref.Data.MapID, ref.Data.PosX, ref.Data.PosY, ref.Data.PosZ, motionBroadcast, domainworld.Packet{Opcode: pk.Opcode, Payload: pk.Payload})
}

// deadOrHiddenSpawns snapshots the spawn guids that are currently dead
// (corpse lying or already removed). Their movers must not pick new hops:
// Unit::SetDeathState stops the movement and the corpse stays where the
// creature died.
func (h *Handler) deadOrHiddenSpawns() map[uint32]bool {
	l := &h.life
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.m) == 0 {
		return nil
	}
	out := make(map[uint32]bool, len(l.m))
	for id, c := range l.m {
		if c.dead {
			out[id] = true
		}
	}
	return out
}

// creatureVisibleState tells the visibility pass whether a creature is in
// the world and with which live health and unit flags it must be created.
// flags is 0 when the template flags apply unchanged (not in combat).
func (h *Handler) creatureVisibleState(spawnGUID uint32) (health, flags uint32, override, visible bool) {
	l := &h.life
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.m[spawnGUID]
	if !ok {
		return 0, 0, false, true
	}
	if c.hidden {
		return 0, 0, false, false
	}
	if c.combat {
		flags = c.unitFlags | unitFlagInCombat
	}
	return c.health, flags, true, true
}

func attackStartPacket(attacker, victim uint64) world.Packet {
	w := wire.NewWriter(16)
	w.Uint64(attacker)
	w.Uint64(victim)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_ATTACKSTART, Payload: b}
}

func attackStopPacket(attacker, victim uint64) world.Packet {
	w := wire.NewWriter(20)
	w.PackedGUID(attacker)
	w.PackedGUID(victim) // packed 0 is a single zero byte
	w.Uint32(0)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_ATTACKSTOP, Payload: b}
}

func attackerStateUpdatePacket(attacker, victim uint64, r combat.Result) world.Packet {
	w := wire.NewWriter(96)
	w.Uint32(r.HitInfo)
	w.PackedGUID(attacker)
	w.PackedGUID(victim)
	w.Uint32(r.Damage)
	w.Uint8(1)  // sub damage count
	w.Uint32(1) // physical school mask
	w.Float32(float32(r.Damage))
	w.Uint32(r.Damage)
	w.Uint32(r.Absorb)
	w.Uint32(r.Resist)
	w.Uint32(r.TargetState)
	w.Uint32(0)
	w.Uint32(0)
	w.Uint32(r.Blocked)
	b, _ := w.Result()
	return world.Packet{Opcode: op.SMSG_ATTACKERSTATEUPDATE, Payload: b}
}

// tickInterval is the session tick period: 50 ms while a melee swing may be
// due, the usual 250 ms otherwise.
func (s *Session) tickInterval() time.Duration {
	if s.combat.active {
		return 50 * time.Millisecond
	}
	return 250 * time.Millisecond
}

// foeSet is the aura set the player put on a creature (nil-safe).
func (h *Handler) foeSet(s *Session, v creatureView) *aura.Set {
	if s.spells != nil {
		if fa := s.spells.foes[v.ref.GUID]; fa != nil {
			return &fa.set
		}
	}
	return &aura.Set{}
}

// mainHandWeapon returns class, subclass and inventory type of the main hand item.
func (h *Handler) mainHandWeapon(s *Session) (class, sub, inv uint32, ok bool) {
	for _, it := range s.player.Items {
		if it.Bag != 0 || it.Slot != 15 {
			continue
		}
		if d, found := h.Data.ItemPrototype(it.Entry); found {
			return d.Class, d.SubClass, d.InventoryType, true
		}
	}
	return 0, 0, 0, false
}
