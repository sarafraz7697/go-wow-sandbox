package codec

import (
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

// ItemQuery builds SMSG_ITEM_QUERY_SINGLE_RESPONSE from the full
// item_template entity. Field order mirrors the reference
// BuildItemQueryResponse packet.
func ItemQuery(id uint32, data *gamedata.Catalog) (world.Packet, error) {
	w := wire.NewWriter(8192)
	v, ok := data.ItemPrototype(id)
	if !ok {
		w.Uint32(id | 0x80000000)
	} else {
		for _, n := range []uint32{id, v.Class, v.SubClass, 0xffffffff} {
			w.Uint32(n)
		}
		w.CString(v.Name1, 255)
		w.Bytes([]byte{0, 0, 0})
		for _, n := range []uint32{v.DisplayInfoID, v.Quality, v.Flags, v.BuyPrice, v.SellPrice, v.InventoryType, uint32(v.AllowableClass), uint32(v.AllowableRace), v.ItemLevel, v.RequiredLevel, v.RequiredSkill, v.RequiredSkillRank, v.RequiredSpell, v.RequiredHonorRank, v.RequiredCityRank, v.RequiredReputationFaction, v.RequiredReputationRank, v.MaxCount, uint32(v.Stackable), v.ContainerSlots} {
			w.Uint32(n)
		}
		for _, x := range v.ItemStat {
			w.Uint32(x.ItemType)

			w.Uint32(uint32(x.Value))
		}
		for _, x := range v.Damage {
			w.Float32(x.DamageMin)
			w.Float32(x.DamageMax)
			w.Uint32(x.DamageType)
		}
		for _, x := range v.ResistanceValues() {
			w.Uint32(x)
		}
		w.Uint32(v.Delay)
		w.Uint32(v.AmmoType)
		w.Float32(v.RangedModRange)
		for _, x := range v.Spells {
			spell, known := data.SpellCooldown(x.SpellID)
			if !known {
				for _, n := range []uint32{0, 0, 0, 0xffffffff, 0, 0xffffffff} {
					w.Uint32(n)
				}
				continue
			}
			charges := x.SpellCharges
			if charges > 0 {
				charges = -charges
			}
			w.Uint32(x.SpellID)
			w.Uint32(x.SpellTrigger)
			w.Uint32(uint32(charges))
			if x.SpellCooldown >= 0 || x.SpellCategoryCooldown >= 0 {
				w.Uint32(uint32(x.SpellCooldown))
				w.Uint32(x.SpellCategory)
				w.Uint32(uint32(x.SpellCategoryCooldown))
			} else {
				w.Uint32(spell.Recovery)
				w.Uint32(spell.Category)
				w.Uint32(spell.CategoryRecovery)
			}
		}
		w.Uint32(v.Bonding)
		w.CString(v.Description, 4096)
		for _, n := range []uint32{v.PageText, v.LanguageID, v.PageMaterial, v.StartQuest, v.LockID, uint32(v.Material), v.Sheath, v.RandomProperty, v.RandomSuffix, v.Block, v.ItemSet, v.MaxDurability, v.Area, uint32(v.Map), v.BagFamily, v.TotemCategory} {
			w.Uint32(n)
		}
		for _, x := range v.Socket {
			w.Uint32(x.SocketColor)
			w.Uint32(x.SocketContent)
		}
		w.Uint32(v.SocketBonus)
		w.Uint32(v.GemProperties)
		w.Uint32(uint32(v.RequiredDisenchantSkill))
		w.Float32(v.ArmorDamageModifier)
		w.Uint32(v.Duration)
	}
	b, err := w.Result()
	return world.Packet{Opcode: op.SMSG_ITEM_QUERY_SINGLE_RESPONSE, Payload: b}, err
}
