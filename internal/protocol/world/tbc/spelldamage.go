package tbc

import (
	"math"
	"time"

	"wowserver/internal/domain/aura"
	"wowserver/internal/domain/combat"
	"wowserver/internal/domain/spell"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world/tbc/codec"
)

// Implicit unit targets that select the player's hostile target
// (TARGET_CHAIN_DAMAGE, TARGET_CURRENT_ENEMY_COORDINATES, TARGET_SINGLE_ENEMY).
const (
	targetChainDamage             = 6
	targetCurrentEnemyCoordinates = 53
	targetSingleEnemy             = 77

	mechanicBleed              = 15
	schoolMaskNormal           = 1
	spellAttrExBCantCrit       = 0x20000000 // SPELL_ATTR_EX2_CANT_CRIT
	spellAttrExCCantMiss       = 0x00040000 // SPELL_ATTR_EX3_CANT_MISS
	spellAttrNoDodgeParryBlock = 0x00020000 // SPELL_ATTR_IMPOSSIBLE_DODGE_PARRY_BLOCK (Attributes)
	facingFlagInFront          = 0x1
	spellRangeSelfOnly         = 1
	spellRangeCombat           = 2
	spellRangeAnywhere         = 13
	rangeLeeway                = 1.25 // Spell::CheckRange max range slack
	rangeLeewayCast            = 6.25 // not strict: SPELL_RANGE_LENIENCY
)

// effectTargetsEnemy reports whether effect i is applied to the hostile
// target of the cast instead of the caster.
func effectTargetsEnemy(info *gamedata.SpellInfo, i int) bool {
	switch info.TargetA[i] {
	case targetChainDamage, targetCurrentEnemyCoordinates, targetSingleEnemy:
		return info.Effect[i] != 0
	}
	return false
}

func spellHasEnemyEffect(info *gamedata.SpellInfo) bool {
	for i := 0; i < 3; i++ {
		if effectTargetsEnemy(info, i) {
			return true
		}
	}
	return false
}

// spellTarget is one entry of Spell::m_UniqueTargetInfo.
type spellTarget struct {
	guid       uint64
	self       bool
	effectMask uint32
	miss       combat.SpellMiss
	delay      time.Duration
	done       bool
	// delayed spells compute their damage at launch (HandleDelayedSpellLaunch)
	launched bool
	damage   uint32
	hitInfo  uint32
}

// delayedSpell is a cast with projectile travel time still in the air.
type delayedSpell struct {
	st      *castState
	targets []*spellTarget
	start   time.Time
}

// foeAuras are the auras the player applied to one creature.
type foeAuras struct {
	ref  gamedata.CreatureSpawnRef
	set  aura.Set
	at   time.Time
	sent map[int]uint32
}

// ---- target checks (Spell::CheckCast target part, CheckRange) ----

// enemyTarget resolves the hostile target of a cast.
func (h *Handler) enemyTarget(s *Session, st *castState, now time.Time) (creatureView, uint8) {
	var v creatureView
	if st.targets.Mask&(codec.TargetFlagUnit|codec.TargetFlagUnk2) == 0 {
		return v, spell.FailedBadTargets
	}
	v, ok := h.lookupCreature(s, st.targets.Unit, now)
	if !ok {
		return v, spell.FailedBadTargets
	}
	if !v.alive {
		return v, spell.FailedTargetsDead
	}
	if h.creatureFriendlyOrUnattackable(s, v) {
		if v.spawn.UnitFlags&(unitFlagNonAttack|unitFlagNotSelect) == 0 {
			return v, spell.FailedTargetFriendly
		}
		return v, spell.FailedBadTargets
	}
	return v, spell.CastOK
}

// checkSpellTarget ports the target related part of Spell::CheckCast:
// hostility, range, facing and combo points. strict is the prepare-time call.
func (h *Handler) checkSpellTarget(s *Session, now time.Time, st *castState, strict bool) uint8 {
	info, p := st.info, s.player
	if info.Attributes&(gamedata.SpellAttrExReqTargetComboPoints) != 0 || info.AttributesEx&(gamedata.SpellAttrExReqTargetComboPoints|gamedata.SpellAttrExReqComboPoints) != 0 {
		return spell.FailedNoComboPoints // combo points are not simulated yet
	}
	if !spellHasEnemyEffect(info) {
		return spell.CastOK
	}
	v, res := h.enemyTarget(s, st, now)
	if res != spell.CastOK {
		return res
	}
	if info.EquippedItemClass == 2 && !h.weaponFits(s, info) {
		return spell.FailedEquippedItemClass
	}
	dx, dy, dz := float64(v.x-p.X), float64(v.y-p.Y), float64(v.z-p.Z)
	centres := float32(math.Sqrt(dx*dx + dy*dy + dz*dz))
	reach := float32(defaultCombatReach) + v.spawn.CombatReach + baseMeleeRangeOffset
	switch info.RangeIndex {
	case 0, spellRangeSelfOnly, spellRangeAnywhere:
	case spellRangeCombat:
		mod := float32(0)
		if !strict {
			mod = 5
		}
		r := reach + mod
		if r < attackDistance {
			r = attackDistance
		}
		if centres > r {
			return spell.FailedOutOfRange
		}
	default:
		rg, ok := h.Data.SpellRange(info.RangeIndex)
		if !ok {
			break
		}
		// CombatDistanceBetween (not for melee range): centres minus both reaches.
		dist := centres - reach
		if dist < 0 {
			dist = 0
		}
		maxRange := rg.Max
		if strict {
			maxRange += rangeLeeway
		} else {
			maxRange += rangeLeewayCast
		}
		if dist > maxRange {
			return spell.FailedOutOfRange
		}
		if rg.Min > 0 && dist < rg.Min {
			return spell.FailedTooClose
		}
	}
	if info.FacingCasterFlags&facingFlagInFront != 0 && !combat.HasInArc(p.X, p.Y, p.Orientation, v.x, v.y, math.Pi) {
		return spell.FailedUnitNotInfront
	}
	return spell.CastOK
}

// weaponFits ports Item::IsFitToSpellRequirements for the main hand weapon.
func (h *Handler) weaponFits(s *Session, info *gamedata.SpellInfo) bool {
	cls, sub, inv, ok := h.mainHandWeapon(s)
	if !ok || int32(cls) != info.EquippedItemClass {
		return false
	}
	if info.EquippedItemSubclass != 0 && info.EquippedItemSubclass&(1<<sub) == 0 {
		return false
	}
	if info.EquippedItemInvTypes != 0 && info.EquippedItemInvTypes&(1<<inv) == 0 {
		return false
	}
	return true
}

// ---- target list (Spell::FillTargetMap / AddUnitTarget) ----

// fillTargets builds the unique target list in effect order.
func (h *Handler) fillTargets(s *Session, now time.Time, st *castState) []*spellTarget {
	info := st.info
	caster := uint64(s.player.GUID)
	var list []*spellTarget
	var self, foe *spellTarget
	for i := 0; i < 3; i++ {
		if info.Effect[i] == 0 {
			continue
		}
		if effectTargetsEnemy(info, i) {
			if foe == nil {
				foe = &spellTarget{guid: st.targets.Unit}
				list = append(list, foe)
			}
			foe.effectMask |= 1 << i
			if info.TargetB[i] != 0 && info.TargetB[i] != targetChainDamage {
				h.warnOnce(s, info.ID<<2|uint32(i), "spell second target not implemented", info.ID, info.TargetB[i])
			}
			if info.EffectChainTargets[i] > 1 {
				h.warnOnce(s, info.ID<<2|uint32(i), "spell chain targets not implemented", info.ID, info.EffectChainTargets[i])
			}
			continue
		}
		if self == nil {
			self = &spellTarget{guid: caster, self: true}
			list = append(list, self)
		}
		self.effectMask |= 1 << i
	}
	return list
}

// spellHitResult ports Unit::SpellHitResult for the player hitting a creature.
func (h *Handler) spellHitResult(s *Session, st *castState, v creatureView, foe *aura.Set) combat.SpellMiss {
	info, p := st.info, s.player
	level := uint32(p.Level)
	switch info.DefenseType {
	case gamedata.DamageClassMagic:
		hit := foe.Total(aura.ModAttackerSpellHitChance)
		bonus := s.sp().auras.Total(aura.ModSpellHitChance)
		return combat.MagicHit(rng{}, level, v.spawn.Level, false, bonus, float32(hit), info.AttributesExC&spellAttrExCCantMiss != 0)
	case gamedata.DamageClassMelee, gamedata.DamageClassRanged:
		in := h.meleeSpellInput(s, info, v, foe)
		return combat.MeleeSpellHit(rng{}, in)
	}
	return combat.SpellMissNone
}

func (h *Handler) meleeSpellInput(s *Session, info *gamedata.SpellInfo, v creatureView, foe *aura.Set) combat.MeleeSpellInput {
	p := s.player
	level := uint32(p.Level)
	skill := int32(level * 5)
	if info.EquippedItemClass == 2 {
		skill = int32(h.playerWeaponSkill(p))
	}
	vlevel := int32(v.spawn.Level)
	ranged := info.DefenseType == gamedata.DamageClassRanged
	mod := float32(s.sp().auras.Total(aura.ModHitChance))
	mod -= float32(foe.Total(aura.ModAttackerMeleeHitChance))
	if ranged {
		mod = float32(s.sp().auras.Total(aura.ModHitChance)) - float32(foe.Total(aura.ModAttackerRangedHitChance))
	}
	in := combat.MeleeSpellInput{
		AttackerSkill:  skill,
		VictimMaxSkill: vlevel * 5,
		VictimDefense:  vlevel * 5,
		ModHit:         mod,
		CantMiss:       info.AttributesExC&spellAttrExCCantMiss != 0,
		NoDodgeParry:   info.Attributes&spellAttrNoDodgeParryBlock != 0,
		Ranged:         ranged,
		FromBehind:     !combat.HasInArc(v.x, v.y, v.o, p.X, p.Y, math.Pi),
		VictimNoParry:  v.info.ExtraFlags&extraNoParry != 0,
		DodgePct:       5,
	}
	if v.info.Type == creatureTypeHumanoid {
		in.ParryPct = 5
	}
	return in
}

// ---- damage bonuses ----

// physicalAuraMods returns the player's flat and percent physical damage
// modifiers (SPELL_AURA_MOD_DAMAGE_DONE / MOD_DAMAGE_PERCENT_DONE for the
// physical school) that the reference folds into the weapon damage unit mods.
func (h *Handler) physicalAuraMods(s *Session) (flat, pct float32) {
	pct = 1
	if s.spells == nil {
		return 0, pct
	}
	s.spells.auras.Each(func(hd *aura.Holder, e *aura.Effect) {
		if uint32(e.Misc)&schoolMaskNormal == 0 {
			return
		}
		info := h.Data.Spell(hd.SpellID)
		if info == nil || info.EquippedItemClass != -1 && !h.weaponFits(s, info) {
			return
		}
		switch e.Name {
		case aura.ModDamageDone:
			flat += float32(e.Amount)
		case aura.ModDamagePercentDone:
			pct *= (100 + float32(e.Amount)) / 100
		}
	})
	return flat, pct
}

// spellDonePercent is the player's school percent modifier for spell damage
// (SpellDamageBonusDone, DoneTotalMod).
func (h *Handler) spellDonePercent(s *Session, schoolMask uint32) float32 {
	m := float32(1)
	if s.spells == nil {
		return m
	}
	s.spells.auras.Each(func(hd *aura.Holder, e *aura.Effect) {
		if e.Name != aura.ModDamagePercentDone || uint32(e.Misc)&schoolMask == 0 {
			return
		}
		if info := h.Data.Spell(hd.SpellID); info != nil && info.EquippedItemClass == -1 && info.EquippedItemInvTypes == 0 {
			m *= (100 + float32(e.Amount)) / 100
		}
	})
	return m
}

// donePercent dispatches on the damage class like CalculateSpellDamage.
func (h *Handler) bonusDone(s *Session, info *gamedata.SpellInfo, damage int32, dot, weaponBased bool) int32 {
	school := info.SchoolMask
	switch info.DefenseType {
	case gamedata.DamageClassMelee, gamedata.DamageClassRanged:
		if weaponBased {
			return damage
		}
		flat, pct := h.physicalAuraMods(s)
		if school&schoolMaskNormal == 0 {
			pct = h.spellDonePercent(s, school)
			flat = 0
		}
		if flat != 0 {
			h.warnOnce(s, info.ID<<2|3, "spell power coefficient not implemented", info.ID, 0)
		}
		return int32(float32(damage) * pct)
	}
	if s.sp().auras.TotalByMisc(aura.ModDamageDone, school) != 0 {
		h.warnOnce(s, info.ID<<2|3, "spell power coefficient not implemented", info.ID, 0)
	}
	return int32(float32(damage) * h.spellDonePercent(s, school))
}

// bonusTaken ports the victim side (Unit::MeleeDamageBonusTaken /
// SpellDamageBonusTaken) for the auras the player put on the creature.
func (h *Handler) bonusTaken(foe *aura.Set, info *gamedata.SpellInfo, damage int32) int32 {
	school := info.SchoolMask
	mult := float32(1)
	foe.Each(func(_ *aura.Holder, e *aura.Effect) {
		if e.Name == aura.ModDamagePercentTaken && uint32(e.Misc)&school != 0 {
			mult *= (100 + float32(e.Amount)) / 100
		}
	})
	flat := foe.TotalByMisc(aura.ModDamageTaken, school)
	return int32(float32(damage+flat) * mult)
}

// ---- crit ----

func rollChance(chance float32) bool {
	return chance >= 100 || rng{}.Norm()*100 < chance
}

// spellCritChance ports Unit::IsSpellCrit chance selection for a player caster.
func (h *Handler) spellCritChance(s *Session, info *gamedata.SpellInfo, v creatureView, foe *aura.Set) float32 {
	p := s.player
	var chance float32
	auras := &s.sp().auras
	switch info.DefenseType {
	case gamedata.DamageClassMagic:
		if info.SchoolMask&schoolMaskNormal != 0 {
			return 0
		}
		chance = h.Data.SpellCritFromIntellect(uint32(p.Class), uint32(p.Level), p.Stats.Attributes[3])
		chance += float32(auras.Total(aura.ModSpellCritChance))
		chance += float32(auras.TotalByMisc(aura.ModSpellCritChanceSchool, info.SchoolMask))
		chance += float32(foe.Total(aura.ModAttackerSpellCritChance))
	case gamedata.DamageClassMelee, gamedata.DamageClassRanged:
		chance = h.meleeCritChance(s, v, foe)
		chance += float32(auras.TotalByMisc(aura.ModSpellCritChanceSchool, info.SchoolMask))
	default:
		return 0
	}
	return max(chance, 0)
}

// meleeCritChance ports Unit::GetUnitCriticalChance for the player main hand.
func (h *Handler) meleeCritChance(s *Session, v creatureView, foe *aura.Set) float32 {
	p := s.player
	level := uint32(p.Level)
	crit := h.Data.MeleeCritFromAgility(uint32(p.Class), level, p.Stats.Attributes[1])
	if s.spells != nil {
		crit += float32(s.spells.auras.Total(aura.ModCritPercent))
	}
	crit += float32(foe.Total(aura.ModAttackerMeleeCritChance) + foe.Total(aura.ModAttackerSpellWeaponCritChance))
	return combat.CritChance(crit, level*5, v.spawn.Level*5)
}

// ---- damage calculation (Unit::CalculateSpellDamage) ----

type spellDamage struct {
	damage, absorb, resist, blocked, hitInfo uint32
}

func (h *Handler) victimArmor(v creatureView, foe *aura.Set) uint32 {
	armor := int64(v.info.Armor)
	if foe != nil {
		armor += int64(foe.TotalByMisc(aura.ModResistance, schoolMaskNormal))
	}
	return uint32(max(armor, 0))
}

// calcSpellDamage ports Unit::CalculateSpellDamage for a player caster.
func (h *Handler) calcSpellDamage(s *Session, info *gamedata.SpellInfo, v creatureView, foe *aura.Set, damage int32, weaponBased bool) spellDamage {
	var out spellDamage
	phys := info.DefenseType == gamedata.DamageClassMelee || info.DefenseType == gamedata.DamageClassRanged
	crit := info.AttributesExB&spellAttrExBCantCrit == 0 && rollChance(h.spellCritChance(s, info, v, foe))
	damage = h.bonusDone(s, info, damage, false, weaponBased)
	damage = h.bonusTaken(foe, info, damage)
	if damage < 0 {
		damage = 0
	}
	d := uint32(damage)
	if crit {
		out.hitInfo |= combat.SpellHitCrit
		// SpellCriticalDamageBonus: critPctDamageMod starts from the "multiplier"
		// of the (empty) victim aura list, which the reference adds as 1.
		d = combat.CritDamage(d, phys, 1)
	}
	if d > 0 && info.SchoolMask&schoolMaskNormal != 0 {
		d = combat.ArmorReduced(uint32(s.player.Level), h.victimArmor(v, foe), d)
	}
	out.damage = d
	return out
}

// absorbResistBlock ports Unit::CalculateAbsorbResistBlock for a creature
// victim (no absorb auras exist on creatures yet).
func (h *Handler) absorbResistBlock(s *Session, info *gamedata.SpellInfo, v creatureView, d *spellDamage, dot bool) {
	p := s.player
	if info.DefenseType == gamedata.DamageClassMelee || info.DefenseType == gamedata.DamageClassRanged {
		if info.Attributes&spellAttrNoDodgeParryBlock == 0 && v.info.ExtraFlags&extraNoBlock == 0 &&
			combat.HasInArc(v.x, v.y, v.o, p.X, p.Y, math.Pi) {
			skill := int32(uint32(p.Level) * 5)
			if info.EquippedItemClass == 2 {
				skill = int32(h.playerWeaponSkill(p))
			}
			chance := float32(5) + float32(skill-int32(v.spawn.Level*5))*0.04
			if chance > 0 && rollChance(chance) {
				d.blocked = min(v.spawn.Level/2, d.damage)
				d.damage -= d.blocked
			}
		}
	}
	if info.SchoolMask&schoolMaskNormal == 0 && d.damage > 0 {
		res := creatureResistance(v, info.SchoolMask)
		d.resist = combat.ResistedDamage(rng{}, float32(res), v.spawn.Level, d.damage, dot)
		d.damage -= d.resist
	}
}

// creatureResistance is Unit::GetResistance of the first school in the mask.
func creatureResistance(v creatureView, mask uint32) int32 {
	for school := 1; school < 7; school++ {
		if mask&(1<<school) != 0 {
			return v.info.Resist[school]
		}
	}
	return 0
}

// ---- weapon damage (Spell::EffectWeaponDmg) ----

// weaponDamageRange returns the main-hand min/max damage with the
// attack-power part recomputed for the normalized weapon speed.
func (h *Handler) weaponDamageRange(s *Session, normalized bool) (lo, hi float32) {
	p := s.player
	lo, hi = p.Stats.DamageMin[0], p.Stats.DamageMax[0]
	if normalized {
		ap := float32(max(p.Stats.AttackPower, 0)) / 14
		speed := float32(p.Stats.AttackTime[0]) / 1000
		norm := h.normalizedWeaponSpeed(s) / 1000
		lo += ap * (norm - speed)
		hi += ap * (norm - speed)
	}
	return lo, hi
}

// normalizedWeaponSpeed is Unit::GetAttackTime normalization (2.4/3.3/1.7/2.4).
func (h *Handler) normalizedWeaponSpeed(s *Session) float32 {
	cls, sub, inv, ok := h.mainHandWeapon(s)
	if !ok || cls != 2 {
		return 2400 // fist: unarmed
	}
	switch {
	case inv == 15 || inv == 25 || inv == 26: // ranged weapons
		return 2800
	case inv == 17: // two-handed
		return 3300
	case sub == 15: // dagger
		return 1700
	}
	return 2400
}

// ---- hit processing (Spell::DoAllEffectOnTarget / DoSpellHitOnUnit) ----

func (h *Handler) foeFor(s *Session, v creatureView) *foeAuras {
	sp := s.sp()
	if sp.foes == nil {
		sp.foes = map[uint32]*foeAuras{}
	}
	fa := sp.foes[v.ref.GUID]
	if fa == nil {
		fa = &foeAuras{ref: v.ref, sent: map[int]uint32{}}
		sp.foes[v.ref.GUID] = fa
	}
	return fa
}

// doAllEffectOnTarget ports Spell::DoAllEffectOnTarget for the hostile target.
func (h *Handler) doAllEffectOnTarget(s *Session, now time.Time, st *castState, t *spellTarget) {
	info, p := st.info, s.player
	caster := uint64(p.GUID)
	v, ok := h.lookupCreature(s, t.guid, now)
	if !ok || !v.alive {
		return
	}
	fa := h.foeFor(s, v)
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
				damage += spell.EffectValue(info, i, uint32(p.Level), irand)
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
		h.addFoeAura(s, now, st, v, fa, effects)
	}
	if damage <= 0 {
		return
	}
	var d spellDamage
	if t.launched {
		d = spellDamage{damage: uint32(damage), hitInfo: t.hitInfo}
	} else {
		d = h.calcSpellDamage(s, info, v, &fa.set, damage, weaponBased)
	}
	h.absorbResistBlock(s, info, v, &d, false)
	_ = s.send(codec.SpellNonMeleeDamageLog(codec.CreatureGUID(v.ref.Data.ID, v.ref.GUID), caster, info.ID, d.damage, uint8(info.SchoolMask), d.absorb, d.resist, false, d.blocked, d.hitInfo))
	if d.damage > 0 {
		h.dealCreatureDamage(s, v, d.damage, now)
	}
}

// launch ports Spell::HandleDelayedSpellLaunch: damage is rolled when the
// spell leaves the caster, the hit lands after the travel time.
func (h *Handler) launch(s *Session, now time.Time, st *castState, t *spellTarget) {
	info, p := st.info, s.player
	v, ok := h.lookupCreature(s, t.guid, now)
	if !ok || t.miss != combat.SpellMissNone {
		return
	}
	fa := h.foeFor(s, v)
	var damage int32
	for i := 0; i < 3; i++ {
		if t.effectMask&(1<<i) != 0 && info.Effect[i] == gamedata.EffectSchoolDamage {
			damage += spell.EffectValue(info, i, uint32(p.Level), irand)
		}
	}
	d := h.calcSpellDamage(s, info, v, &fa.set, damage, false)
	t.launched, t.damage, t.hitInfo = true, d.damage, d.hitInfo
}

// handleSpellTargets resolves misses, sends nothing yet and returns the
// hit and miss lists for SMSG_SPELL_GO.
func (h *Handler) resolveHits(s *Session, now time.Time, st *castState, list []*spellTarget) (hit []uint64, miss []codec.SpellMissEntry) {
	info := st.info
	for _, t := range list {
		if t.self {
			if !h.noUnitTargets(info) {
				hit = append(hit, t.guid)
			}
			continue
		}
		v, ok := h.lookupCreature(s, t.guid, now)
		if !ok {
			continue
		}
		fa := h.foeFor(s, v)
		t.miss = h.spellHitResult(s, st, v, &fa.set)
		if info.Speed > 0 {
			dx, dy, dz := float64(v.x-s.player.X), float64(v.y-s.player.Y), float64(v.z-s.player.Z)
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			t.delay = time.Duration(dist / float64(info.Speed) * float64(time.Second))
		}
		if t.miss == combat.SpellMissNone {
			hit = append(hit, t.guid)
		} else {
			miss = append(miss, codec.SpellMissEntry{Target: t.guid, Reason: uint8(t.miss)})
		}
	}
	return hit, miss
}

// processTargets runs the effects of every due target.
func (h *Handler) processTargets(s *Session, now time.Time, st *castState, list []*spellTarget, start time.Time) bool {
	pending := false
	for _, t := range list {
		if t.done {
			continue
		}
		if now.Sub(start) < t.delay {
			pending = true
			continue
		}
		t.done = true
		if t.self {
			h.applyEffects(s, now, st, t.effectMask)
			continue
		}
		h.doAllEffectOnTarget(s, now, st, t)
	}
	return pending
}

// ---- auras on creatures ----

// addFoeAura ports Unit::AddSpellAuraHolder for an aura the player cast on a creature.
func (h *Handler) addFoeAura(s *Session, now time.Time, st *castState, v creatureView, fa *foeAuras, effects [3]*aura.Effect) {
	info := st.info
	for _, e := range effects {
		if e != nil && !foeSimulated(e.Name) {
			h.warnOnce(s, info.ID<<2|uint32(e.Name&3), "aura on creature is not simulated", info.ID, e.Name)
		}
	}
	hd := aura.NewHolder(info.ID, uint64(s.player.GUID), uint8(s.player.Level), h.Data.DurationMS(info), false, effects)
	hd.InterruptFlags = info.AuraInterruptFlags
	if fa.at.IsZero() {
		fa.at = now
	}
	if _, ok := fa.set.Add(hd); !ok {
		return
	}
	h.syncFoeFields(s, v, fa)
	_ = s.send(codec.SetExtraAuraInfoNeedUpdate(codec.CreatureGUID(v.ref.Data.ID, v.ref.GUID), hd.Slot, hd.SpellID, uint32(hd.MaxDuration), uint32(hd.Duration)))
}

func foeSimulated(name uint32) bool {
	switch name {
	case aura.PeriodicDamage, aura.ModDamageTaken, aura.ModDamagePercentTaken, aura.ModAttackerMeleeHitChance,
		aura.ModAttackerRangedHitChance, aura.ModAttackerSpellHitChance, aura.ModAttackerSpellCritChance, aura.ModResistance:
		return true
	}
	return false
}

// syncFoeFields broadcasts the creature's changed UNIT_FIELD_AURA* words.
func (h *Handler) syncFoeFields(s *Session, v creatureView, fa *foeAuras) {
	want := h.auraWords(&fa.set)
	var ch []codec.FieldChange
	for idx, val := range want {
		if fa.sent[idx] != val {
			fa.sent[idx] = val
			ch = append(ch, codec.FieldChange{Index: idx, Value: val})
		}
	}
	if len(ch) == 0 {
		return
	}
	pkt := codec.ValuesUpdate(codec.CreatureGUID(v.ref.Data.ID, v.ref.GUID), ch)
	h.broadcastCreature(v.ref, pkt, time.Now())
}

// foeTick ports Aura::PeriodicTick (SPELL_AURA_PERIODIC_DAMAGE) for the
// player's auras on creatures and drops auras of dead creatures.
func (h *Handler) foeTick(s *Session, now time.Time) {
	sp := s.spells
	for guid, fa := range sp.foes {
		v, ok := h.lookupCreature(s, codec.CreatureGUID(fa.ref.Data.ID, fa.ref.GUID), now)
		if !ok || !v.alive {
			delete(sp.foes, guid)
			continue
		}
		if len(fa.set.Holders()) == 0 {
			delete(sp.foes, guid)
			continue
		}
		diff := int32(now.Sub(fa.at).Milliseconds())
		if diff <= 0 {
			continue
		}
		fa.at = now
		expired := false
		for _, e := range fa.set.Update(diff) {
			if e.Kind == aura.Expired {
				expired = true
				continue
			}
			eff := e.Holder.Effects[e.Effect]
			if eff.Name != aura.PeriodicDamage {
				continue
			}
			h.periodicDamage(s, now, v, fa, e.Holder, eff)
			if cv, ok := h.lookupCreature(s, codec.CreatureGUID(fa.ref.Data.ID, fa.ref.GUID), now); !ok || !cv.alive {
				break
			}
		}
		if expired {
			h.syncFoeFields(s, v, fa)
		}
	}
}

func (h *Handler) periodicDamage(s *Session, now time.Time, v creatureView, fa *foeAuras, hd *aura.Holder, eff *aura.Effect) {
	info := h.Data.Spell(hd.SpellID)
	if info == nil {
		return
	}
	damage := h.bonusTaken(&fa.set, info, eff.Amount)
	if damage < 0 {
		damage = 0
	}
	d := spellDamage{damage: uint32(damage)}
	bleed := false
	for i := 0; i < 3; i++ {
		if info.Aura[i] == aura.PeriodicDamage && info.EffectMechanic[i] == mechanicBleed {
			bleed = true
		}
	}
	if d.damage > 0 && info.SchoolMask&schoolMaskNormal != 0 && !bleed {
		d.damage = combat.ArmorReduced(uint32(hd.Level), h.victimArmor(v, &fa.set), d.damage)
	}
	h.absorbResistBlock(s, &gamedata.SpellInfo{ID: info.ID, SchoolMask: info.SchoolMask, DefenseType: gamedata.DamageClassMagic}, v, &d, true)
	_ = s.send(codec.PeriodicDamageLog(codec.CreatureGUID(v.ref.Data.ID, v.ref.GUID), hd.Caster, info.ID, d.damage, info.SchoolMask, d.absorb, d.resist))
	if d.damage > 0 {
		h.dealCreatureDamage(s, v, d.damage, now)
	}
}


// effectWeaponDamage ports Spell::EffectWeaponDmg (no class special cases):
// fixed bonuses, the weapon percent chain, weapon_total_pct and the rolled
// main-hand weapon damage.
func (h *Handler) effectWeaponDamage(s *Session, info *gamedata.SpellInfo, mask uint32) int32 {
	p := s.player
	level := uint32(p.Level)
	weaponPct := float32(1)
	normalized := false
	var fixed int32
	for j := 0; j < 3; j++ {
		if mask&(1<<j) == 0 {
			continue
		}
		switch info.Effect[j] {
		case gamedata.EffectWeaponDamage, gamedata.EffectWeaponDamageNoSchool:
			fixed += spell.EffectValue(info, j, level, irand)
		case gamedata.EffectNormalizedWeaponDmg:
			fixed += spell.EffectValue(info, j, level, irand)
			normalized = true
		case gamedata.EffectWeaponPercentDamage:
			weaponPct *= float32(spell.EffectValue(info, j, level, irand)) / 100
			fixed = int32(float32(fixed) * weaponPct)
		}
	}
	flat, totalPct := h.physicalAuraMods(s)
	bonus := fixed
	if bonus != 0 {
		bonus = int32(float32(bonus) * totalPct)
	}
	lo, hi := h.weaponDamageRange(s, normalized)
	lo, hi = (lo+flat)*totalPct, (hi+flat)*totalPct
	if lo > hi {
		lo, hi = hi, lo
	}
	if hi == 0 {
		hi = 5
	}
	roll := rng{}.Urand(uint32(lo), uint32(hi))
	bonus += int32(float32(roll) * weaponPct)
	return max(bonus, 0)
}
