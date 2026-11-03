# Character login stats

Calculation lives in `application/character/stats`; the result is a domain
`character.Stats` snapshot. The protocol codec only writes that result into
8606 update fields. No derived values are written back to the database.

Source inspection: local MaNGOS `Object/StatSystem.cpp` (`UpdateAllStats`,
`UpdateMaxHealth`, `UpdateMaxPower`, `UpdateAttackPowerAndDamage`,
`CalculateMinMaxDamage`), `PlayerItemApply.cpp`, `PlayerItemEnchant.cpp` and
`PlayerStats.cpp`. Enchantment layout comes from `DBCStructure.h`.

Implemented for the ordinary unshifted base/equipment path:

- Race/class/level attributes plus equipped item bonuses, including negative
  bonuses; backpack and bank contents do not contribute. Broken equipment does
  not contribute bonuses or weapon damage.
- Direct item health/mana bonuses; stamina gives 1 HP per point for the first
  20 points, then 10; intellect similarly gives 1 mana then 15 (mana classes).
- Armor from equipment plus twice agility, school resistances, shield block.
- Class-specific base melee/ranged AP; weapon template damage and delay;
  AP contribution `max(AP,0)/14 * attack_seconds`; offhand multiplier 0.5;
  unarmed baseline 1–2 damage at 2 seconds.
- Unconditional direct enchantment/gem stat, resistance and weapon-damage
  effects, using SpellItemEnchantment.dbc. On-hit procs are not idle stat buffs.
- Current stored health and powers are clamped to the computed maxima for the
  outgoing update. They are not replaced with full health/mana on login.
- Positive/negative stat bonus fields, damage ranges, attack speeds, attack
  power, armor/resistance and shield block fields are explicitly serialized.

## Not yet full MaNGOS parity

`UpdateAllStats` depends on the spell/aura modifier system as well as item
templates. Passive/racial spells, talents, persisted buffs, forms, item equip
spells, set bonuses, conditional meta gems, random-suffix scaling, combat rating
conversion, crit/dodge/parry, mana regeneration, spell/healing power and ranged
ammo bonuses are not calculated here yet. Relevant unsupported equipment
effects and the missing aura engine are reported in the login warning.

Consequently a buffed/geared character can still differ from MaNGOS. This is an
explicit incomplete stat engine, not a claim of full parity. Remaining work
requires loading spell modifiers and rating/crit/regen DBC tables and evaluating
them through a domain modifier pipeline, including the same ordering as MaNGOS.

No tests were added/run, and no build or client execution was performed.
