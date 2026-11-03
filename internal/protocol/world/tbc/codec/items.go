package codec

import (
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

func ItemQuery(id uint32, data *gamedata.Catalog) (world.Packet, error) {
	w := wire.NewWriter(8192)
	item, ok := data.Item(id)
	v, details := data.ItemDetails(id)
	if !ok || !details {
		w.Uint32(id | 0x80000000)
	} else {
		for _, n := range []uint32{id, item.Class, item.Subclass, 0xffffffff} {
			w.Uint32(n)
		}
		w.CString(v.Name, 255)
		w.Bytes([]byte{0, 0, 0})
		for _, n := range []uint32{item.Display, v.Quality, v.Flags, v.BuyPrice, v.SellPrice, item.InventoryType, uint32(v.AllowableClass), uint32(v.AllowableRace), v.ItemLevel, v.RequiredLevel, v.RequiredSkill, v.RequiredSkillRank, v.RequiredSpell, v.RequiredHonorRank, v.RequiredCityRank, v.RequiredReputationFaction, v.RequiredReputationRank, v.MaxCount, uint32(item.Stackable), v.ContainerSlots} {
			w.Uint32(n)
		}
		for _, x := range v.Stats {
			w.Uint32(x.Type)
			w.Uint32(uint32(x.Value))
		}
		for _, x := range v.Damage {
			w.Float32(x.Min)
			w.Float32(x.Max)
			w.Uint32(x.Type)
		}
		for _, x := range v.Resistances {
			w.Uint32(x)
		}
		w.Uint32(v.Delay)
		w.Uint32(v.AmmoType)
		w.Float32(v.RangedModRange)
		for _, x := range v.Spells {
			spell, known := data.SpellCooldown(x.ID)
			if !known {
				for _, n := range []uint32{0, 0, 0, 0xffffffff, 0, 0xffffffff} {
					w.Uint32(n)
				}
				continue
			}
			charges := x.Charges
			if charges > 0 {
				charges = -charges
			}
			w.Uint32(x.ID)
			w.Uint32(x.Trigger)
			w.Uint32(uint32(charges))
			if x.Cooldown >= 0 || x.CategoryCooldown >= 0 {
				w.Uint32(uint32(x.Cooldown))
				w.Uint32(x.Category)
				w.Uint32(uint32(x.CategoryCooldown))
			} else {
				w.Uint32(spell.Recovery)
				w.Uint32(spell.Category)
				w.Uint32(spell.CategoryRecovery)
			}
		}
		w.Uint32(item.Bonding)
		w.CString(v.Description, 4096)
		for _, n := range []uint32{v.PageText, v.Language, v.PageMaterial, v.StartQuest, v.Lock, uint32(v.Material), v.Sheath, v.RandomProperty, v.RandomSuffix, v.Block, v.ItemSet, item.MaxDurability, v.Area, uint32(v.Map), v.BagFamily, v.TotemCategory} {
			w.Uint32(n)
		}
		for _, x := range v.Sockets {
			w.Uint32(x.Color)
			w.Uint32(x.Content)
		}
		w.Uint32(v.SocketBonus)
		w.Uint32(v.GemProperties)
		w.Uint32(uint32(v.DisenchantSkill))
		w.Float32(v.ArmorDamageModifier)
		w.Uint32(item.Duration)
	}
	b, err := w.Result()
	return world.Packet{Opcode: op.SMSG_ITEM_QUERY_SINGLE_RESPONSE, Payload: b}, err
}
