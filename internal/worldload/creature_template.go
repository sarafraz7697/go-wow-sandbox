package worldload

import (
	"context"

	"wowserver/internal/entity"
)

// creatureTemplateLoader ports ObjectMgr::LoadCreatureTemplates
// (src/game/Object/ObjectMgrCreatures.cpp) and the SQLStorage source
// format for sCreatureStorage. The column list is the full
// creature_template row in schema order — every field the reference
// holds in CreatureInfo is loaded, not only the ones the current
// simulator consumes.
//
// `Rank` is backticked (reserved word on MySQL 8) and the nullable
// SubName/IconName columns scan through str(), matching
// Field::GetCppString on NULL returning "".
func init() {
	Register(Loader{
		Name:       "creature_template",
		Order:      OrderCreatureTemplate,
		CountQuery: "SELECT COUNT(*) FROM `creature_template`",
		Load:       loadCreatureTemplates,
	})
}

func loadCreatureTemplates(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	const query = "SELECT `Entry`,`Name`,`SubName`,`IconName`,`MinLevel`,`MaxLevel`,`HeroicEntry`," +
		"`ModelId1`,`ModelId2`,`ModelId3`,`ModelId4`,`FactionAlliance`,`FactionHorde`,`Scale`," +
		"`Family`,`CreatureType`,`InhabitType`,`RegenerateStats`,`RacialLeader`," +
		"`NpcFlags`,`UnitFlags`,`DynamicFlags`,`ExtraFlags`,`CreatureTypeFlags`," +
		"`SpeedWalk`,`SpeedRun`,`UnitClass`,`Rank`,`Expansion`," +
		"`HealthMultiplier`,`PowerMultiplier`,`DamageMultiplier`,`DamageVariance`,`ArmorMultiplier`,`ExperienceMultiplier`," +
		"`MinLevelHealth`,`MaxLevelHealth`,`MinLevelMana`,`MaxLevelMana`," +
		"`MinMeleeDmg`,`MaxMeleeDmg`,`MinRangedDmg`,`MaxRangedDmg`," +
		"`Armor`,`MeleeAttackPower`,`RangedAttackPower`,`MeleeBaseAttackTime`,`RangedBaseAttackTime`,`DamageSchool`," +
		"`MinLootGold`,`MaxLootGold`,`LootId`,`PickpocketLootId`,`SkinningLootId`," +
		"`KillCredit1`,`KillCredit2`,`MechanicImmuneMask`,`SchoolImmuneMask`," +
		"`ResistanceHoly`,`ResistanceFire`,`ResistanceNature`,`ResistanceFrost`,`ResistanceShadow`,`ResistanceArcane`," +
		"`PetSpellDataId`,`MovementType`," +
		"`TrainerType`,`TrainerSpell`,`TrainerClass`,`TrainerRace`,`TrainerTemplateId`,`VendorTemplateId`,`EquipmentTemplateId`,`GossipMenuId`," +
		"`AIName` FROM `creature_template` ORDER BY `Entry`"

	return scanTable(ctx, q, t, query, 2000, func(r scanner) error {
		var v entity.CreatureInfo
		err := r.Scan(
			u32(&v.Entry), str(&v.Name), str(&v.SubName), str(&v.IconName), u32(&v.MinLevel), u32(&v.MaxLevel), u32(&v.HeroicEntry),
			u32(&v.ModelId[0]), u32(&v.ModelId[1]), u32(&v.ModelId[2]), u32(&v.ModelId[3]), u32(&v.FactionAlliance), u32(&v.FactionHorde), f32(&v.Scale),
			u32(&v.Family), u32(&v.CreatureType), u32(&v.InhabitType), u32(&v.RegenerateStats), boolean(&v.RacialLeader),
			u32(&v.NpcFlags), u32(&v.UnitFlags), u32(&v.DynamicFlags), u32(&v.ExtraFlags), u32(&v.CreatureTypeFlags),
			f32(&v.SpeedWalk), f32(&v.SpeedRun), u32(&v.UnitClass), u32(&v.Rank), i32(&v.Expansion),
			f32(&v.HealthMultiplier), f32(&v.PowerMultiplier), f32(&v.DamageMultiplier), f32(&v.DamageVariance), f32(&v.ArmorMultiplier), f32(&v.ExperienceMultiplier),
			u32(&v.MinLevelHealth), u32(&v.MaxLevelHealth), u32(&v.MinLevelMana), u32(&v.MaxLevelMana),
			f32(&v.MinMeleeDmg), f32(&v.MaxMeleeDmg), f32(&v.MinRangedDmg), f32(&v.MaxRangedDmg),
			u32(&v.Armor), u32(&v.MeleeAttackPower), u32(&v.RangedAttackPower), u32(&v.MeleeBaseAttackTime), u32(&v.RangedBaseAttackTime), u32(&v.DamageSchool),
			u32(&v.MinLootGold), u32(&v.MaxLootGold), u32(&v.LootId), u32(&v.PickpocketLootId), u32(&v.SkinningLootId),
			u32(&v.KillCredit[0]), u32(&v.KillCredit[1]), u32(&v.MechanicImmuneMask), u32(&v.SchoolImmuneMask),
			i32(&v.ResistanceHoly), i32(&v.ResistanceFire), i32(&v.ResistanceNature), i32(&v.ResistanceFrost), i32(&v.ResistanceShadow), i32(&v.ResistanceArcane),
			u32(&v.PetSpellDataId), u32(&v.MovementType),
			u32(&v.TrainerType), u32(&v.TrainerSpell), u32(&v.TrainerClass), u32(&v.TrainerRace), u32(&v.TrainerTemplateId), u32(&v.VendorTemplateId), u32(&v.EquipmentTemplateId), u32(&v.GossipMenuId),
			str(&v.AIName),
		)
		if err != nil {
			return err
		}
		// Reference validation (LoadCreatureTemplates): entry 0 is not a
		// usable template; minlevel 0 is forced to 1; maxlevel < minlevel
		// is clamped. Fixes stay counted as skips in the report like
		// outErrorDb lines, the row itself is still kept.
		if v.Entry == 0 {
			t.Skipf("creature_template entry 0 is not a valid template")
			return nil
		}
		if v.MinLevel == 0 {
			t.Skipf("Creature (Entry: %d) has invalid minlevel, set to 1", v.Entry)
			v.MinLevel = 1
		}
		if v.MaxLevel < v.MinLevel {
			t.Skipf("Creature (Entry: %d) has invalid maxlevel, set to minlevel", v.Entry)
			v.MaxLevel = v.MinLevel
		}
		if _, dup := dest.CreatureTemplates[v.Entry]; dup {
			t.Skipf("creature_template has duplicate entry %d", v.Entry)
			return nil
		}
		dest.CreatureTemplates[v.Entry] = v
		dest.CreatureTemplateOrder = append(dest.CreatureTemplateOrder, v.Entry)
		t.Stored(1)
		return nil
	})
}
