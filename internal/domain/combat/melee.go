// Package combat ports the melee rules of the reference (UnitCombat.cpp,
// UnitMeleeDamage.cpp, UnitDamage.cpp, PlayerRegen.cpp): outcome roll,
// damage and rage. It is pure: all randomness comes through Rand.
package combat

import "math"

// Rand supplies the reference's urand / rand_norm_f.
type Rand interface {
	// Urand returns a uniform integer in [lo, hi] (inclusive), like urand().
	Urand(lo, hi uint32) uint32
	// Norm returns a uniform float in [0, 1), like rand_norm_f().
	Norm() float32
}

// Outcome is MeleeHitOutcome.
type Outcome int

const (
	HitEvade Outcome = iota
	HitMiss
	HitDodge
	HitBlock
	HitCrit
	HitParry
	HitGlancing
	HitCrushing
	HitNormal
)

// HitInfo flags of SMSG_ATTACKERSTATEUPDATE.
const (
	HitInfoNormalSwing  = 0x0
	HitInfoNormalSwing2 = 0x2
	HitInfoMiss         = 0x10
	HitInfoAbsorb       = 0x20
	HitInfoResist       = 0x40
	HitInfoCritical     = 0x80
	HitInfoGlancing     = 0x4000
	HitInfoCrushing     = 0x8000
	HitInfoNoHitSound   = 0x80000
)

// VictimState values of the attacker state update.
const (
	VictimUnaffected = 0
	VictimNormal     = 1
	VictimDodge      = 2
	VictimParry      = 3
	VictimInterrupt  = 4
	VictimBlocks     = 5
	VictimEvades     = 6
	VictimIsImmune   = 7
)

// Side describes one fighter for the roll.
type Side struct {
	Level       uint32
	IsPlayer    bool
	Class       uint32
	WeaponSkill uint32 // attacker: weapon skill; victim: unused
	Defense     uint32 // victim defense skill
	MaxSkill    uint32 // level * 5 (GetMaxSkillValueForLevel)
	Armor       uint32
	ShieldBlock uint32
	NoParry     bool // CREATURE_FLAG_EXTRA_NO_PARRY
	NoBlock     bool // CREATURE_FLAG_EXTRA_NO_BLOCK
	NoCrush     bool // CREATURE_FLAG_EXTRA_NO_CRUSH
}

// Chances are hundredths of a percent, as RollMeleeOutcomeAgainst takes them.
type Chances struct{ Miss, Crit, Dodge, Parry, Block int32 }

// Roll ports RollMeleeOutcomeAgainst(…, SpellCasted=false) for a main-hand
// auto attack. fromBehind is !victim.HasInArc(attacker, PI).
func Roll(r Rand, att, vic Side, c Chances, fromBehind bool) Outcome {
	skillBonus := 4 * (int32(att.WeaponSkill) - int32(vic.MaxSkill))
	var sum int32
	roll := int32(r.Urand(0, 10000))
	tmp := c.Miss
	if tmp > 0 {
		sum += tmp
		if roll < sum {
			return HitMiss
		}
	}
	if !vic.IsPlayer || !fromBehind {
		tmp = c.Dodge
		if tmp > 0 {
			tmp -= skillBonus
			if tmp > 0 {
				sum += tmp
				if roll < sum {
					return HitDodge
				}
			}
		}
	}
	if !fromBehind {
		if c.Parry > 0 && (vic.IsPlayer || !vic.NoParry) {
			p := c.Parry - skillBonus
			if p > 0 {
				sum += p
				if roll < sum {
					return HitParry
				}
			}
		}
	}
	if att.IsPlayer && !vic.IsPlayer && att.Level < vic.Level {
		skill := min(att.WeaponSkill, att.MaxSkill)
		tmp = (10 + (int32(vic.Defense) - int32(skill))) * 100
		tmp = min(tmp, 2500)
		sum += tmp
		if roll < sum {
			return HitGlancing
		}
	}
	if !fromBehind {
		if vic.IsPlayer || !vic.NoBlock {
			tmp = c.Block
			if tmp > 0 {
				tmp -= skillBonus
				if tmp > 0 {
					sum += tmp
					if roll < sum {
						return HitBlock
					}
				}
			}
		}
	}
	tmp = c.Crit
	if tmp > 0 {
		sum += tmp
		if roll < sum {
			return HitCrit
		}
	}
	if !att.IsPlayer && !att.NoCrush {
		d := min(vic.Defense, vic.MaxSkill)
		t := int32(att.MaxSkill) - int32(d)
		if t >= 15 {
			t = t*200 - 1500
			sum += t
			if roll < sum {
				return HitCrushing
			}
		}
	}
	return HitNormal
}

// MissChance ports MeleeMissChanceCalc (no offhand, no hit mods/ratings).
func MissChance(attWeaponSkill, victimDefense uint32, victimIsPlayer bool) float32 {
	miss := float32(5)
	skillDiff := int32(attWeaponSkill) - int32(victimDefense)
	switch {
	case victimIsPlayer:
		miss -= float32(skillDiff) * 0.04
	case skillDiff < -10:
		miss -= float32(skillDiff+10)*0.4 - 2.0
	default:
		miss -= float32(skillDiff) * 0.1
	}
	return min(max(miss, 0), 60)
}

// CritChance ports the common tail of GetUnitCriticalChance: the base crit
// of the attacker plus the level difference term.
func CritChance(base float32, attackerMaxSkill, victimDefense uint32) float32 {
	crit := base + float32(int32(attackerMaxSkill)-int32(victimDefense))*0.04
	return max(crit, 0)
}

// ArmorReduced ports CalcArmorReducedDamage.
func ArmorReduced(attackerLevel uint32, victimArmor uint32, damage uint32) uint32 {
	armor := float32(victimArmor)
	levelMod := float32(attackerLevel)
	if levelMod > 59 {
		levelMod += 4.5 * (levelMod - 59)
	}
	t := 0.1 * armor / (8.5*levelMod + 40)
	t = t / (1 + t)
	t = min(max(t, 0), 0.75)
	n := uint32(float32(damage) - float32(damage)*t)
	return max(n, 1)
}

// Result is the CalcDamageInfo the attacker state update needs.
type Result struct {
	Outcome     Outcome
	HitInfo     uint32
	TargetState uint32
	Damage      uint32
	Absorb      uint32
	Resist      uint32
	Blocked     uint32
	CleanDamage uint32
}

// Calculate ports CalculateMeleeDamage for a main-hand swing of a player (or
// creature) against a target without absorb/resist auras.
func Calculate(r Rand, att, vic Side, c Chances, fromBehind bool, minDmg, maxDmg float32, attackerCaster bool, defenseDiff int32) Result {
	res := Result{HitInfo: HitInfoNormalSwing2, TargetState: VictimUnaffected}
	// CalculateDamage
	if minDmg > maxDmg {
		minDmg, maxDmg = maxDmg, minDmg
	}
	if maxDmg == 0 {
		maxDmg = 5
	}
	raw := r.Urand(uint32(minDmg), uint32(maxDmg))
	res.Damage = ArmorReduced(att.Level, vic.Armor, raw)
	res.CleanDamage = raw - res.Damage
	res.Outcome = Roll(r, att, vic, c, fromBehind)
	switch res.Outcome {
	case HitEvade:
		res.HitInfo |= HitInfoMiss | HitInfoNoHitSound
		res.TargetState = VictimEvades
		res.Damage, res.CleanDamage = 0, 0
		return res
	case HitMiss:
		res.HitInfo |= HitInfoMiss
		res.TargetState = VictimUnaffected
		res.Damage, res.CleanDamage = 0, 0
	case HitNormal:
		res.TargetState = VictimNormal
	case HitCrit:
		res.HitInfo |= HitInfoCritical
		res.TargetState = VictimNormal
		res.Damage += res.Damage
	case HitParry:
		res.TargetState = VictimParry
		res.CleanDamage += res.Damage
		res.Damage = 0
	case HitDodge:
		res.TargetState = VictimDodge
		res.CleanDamage += res.Damage
		res.Damage = 0
	case HitBlock:
		res.TargetState = VictimNormal
		res.Blocked = vic.ShieldBlock
		if res.Blocked >= res.Damage {
			res.TargetState = VictimBlocks
			res.Blocked = res.Damage
		}
		res.Damage -= res.Blocked
		res.CleanDamage += res.Blocked
	case HitGlancing:
		res.HitInfo |= HitInfoGlancing
		res.TargetState = VictimNormal
		baseLow, baseHigh := float32(1.3), float32(1.2)
		if attackerCaster {
			baseLow -= 0.7
			baseHigh -= 0.3
		}
		maxLow := float32(0.6)
		if att.Class == 1 || att.Class == 4 {
			maxLow = 0.91
		}
		low := baseLow - 0.05*float32(defenseDiff)
		high := baseHigh - 0.03*float32(defenseDiff)
		if low < 0.01 {
			low = 0.01
		} else if low > maxLow {
			low = maxLow
		}
		high = min(max(high, 0.2), 0.99)
		if low > high {
			low = high
		}
		reduce := low + r.Norm()*(high-low)
		red := uint32(reduce * float32(res.Damage))
		res.CleanDamage += res.Damage - red
		res.Damage = red
	case HitCrushing:
		res.HitInfo |= HitInfoCrushing
		res.TargetState = VictimNormal
		res.Damage += res.Damage / 2
	}
	return res
}

// RageReward ports Player::RewardRage for the attacker (returns rage x10).
func RageReward(level uint32, damage uint32, weaponSpeedHitFactor uint32, rate float32) uint32 {
	l := float64(level)
	conv := float32(0.0091107836*l*l+3.225598133*l) + 4.2652911
	add := (float32(damage)/conv*7.5 + float32(weaponSpeedHitFactor)) / 2
	add *= rate
	return uint32(add * 10)
}

// RageWeaponFactor is the weaponSpeedHitFactor of a main-hand hit.
func RageWeaponFactor(attackTimeMS uint32, crit bool) uint32 {
	if crit {
		return uint32(float32(attackTimeMS) / 1000 * 7)
	}
	return uint32(float32(attackTimeMS) / 1000 * 3.5)
}

// RelativeBearing returns the angle from facing o at (x,y) to (tx,ty) in
// [-pi, pi] (Placement::RelativeBearingTo).
func RelativeBearing(x, y, o, tx, ty float32) float32 {
	a := math.Atan2(float64(ty-y), float64(tx-x)) - float64(o)
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a < -math.Pi {
		a += 2 * math.Pi
	}
	return float32(a)
}

// HasInArc ports Placement::HasInArc on one map.
func HasInArc(x, y, o, tx, ty, arc float32) bool {
	if !(arc > 0) {
		return false
	}
	if arc >= 2*math.Pi {
		return true
	}
	b := RelativeBearing(x, y, o, tx, ty)
	return b >= -arc/2 && b <= arc/2
}
