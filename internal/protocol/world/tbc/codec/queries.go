package codec

import (
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

// CreatureQuery builds SMSG_CREATURE_QUERY_RESPONSE for a given creature
// template entry. Mirrors WorldSession::HandleCreatureQueryOpcode in the
// mangos-one 2.4.3 reference (game/WorldHandlers/QueryHandler.cpp).
//
// Wire layout (TBC 2.4.3, build 8606):
//
//	uint32  entry                          // creature entry
//	CString name                           // primary name
//	uint8   0, 0, 0                        // name2, name3, name4 (always empty)
//	CString subName                        // sub-name (Title)
//	CString iconPath                       // IconName ("Directions" for guards)
//	uint32  CreatureTypeFlags              // type flags
//	uint32  CreatureType                   // CreatureType.dbc
//	uint32  Family                         // CreatureFamily.dbc
//	uint32  Rank                           // elite/rare/normal
//	uint32  0                              // unknown (wdbField11)
//	uint32  PetSpellDataId                 // CreatureSpellData.dbc
//	uint32  ModelId[4]                     // MAX_CREATURE_MODEL alternative models
//	float   HealthMultiplier
//	float   PowerMultiplier
//	uint8   RacialLeader
//
// If the entry is not in the catalog, the "not found" response is sent: a
// single uint32 with bit 31 set (entry | 0x80000000), matching the reference.
func CreatureQuery(entry uint32, data *gamedata.Catalog) (world.Packet, error) {
	ci, ok := data.CreatureTemplate(entry)
	if !ok {
		w := wire.NewWriter(4)
		w.Uint32(entry | 0x80000000)
		b, err := w.Result()
		return world.Packet{Opcode: op.SMSG_CREATURE_QUERY_RESPONSE, Payload: b}, err
	}
	w := wire.NewWriter(256)
	w.Uint32(entry)
	w.CString(ci.Name, 256)
	w.Uint8(0)
	w.Uint8(0)
	w.Uint8(0)
	w.CString(ci.SubName, 256)
	w.CString(ci.IconName, 256)
	w.Uint32(ci.CreatureTypeFlags)
	w.Uint32(ci.CreatureType)
	w.Uint32(ci.Family)
	w.Uint32(ci.Rank)
	w.Uint32(0) // unknown wdbField11
	w.Uint32(ci.PetSpellDataId)
	for i := 0; i < gamedata.MAX_CREATURE_MODEL; i++ {
		w.Uint32(ci.ModelId[i])
	}
	w.Float32(ci.HealthMultiplier)
	w.Float32(ci.PowerMultiplier)
	if ci.RacialLeader {
		w.Uint8(1)
	} else {
		w.Uint8(0)
	}
	b, err := w.Result()
	return world.Packet{Opcode: op.SMSG_CREATURE_QUERY_RESPONSE, Payload: b}, err
}

// GameObjectQuery builds SMSG_GAMEOBJECT_QUERY_RESPONSE for a given
// gameobject template entry. Mirrors WorldSession::HandleGameObjectQueryOpcode
// in the mangos-one 2.4.3 reference (game/WorldHandlers/QueryHandler.cpp).
//
// Wire layout (TBC 2.4.3, build 8606):
//
//	uint32  entry                          // gameobject entry
//	uint32  type                           // GAMEOBJECT_TYPE_*
//	uint32  displayId
//	CString name
//	uint8   0, 0, 0                        // name2, name3, name4 (always empty)
//	CString IconName
//	CString CastBarCaption
//	uint8   0                              // 2.0.3 string placeholder (always empty)
//	[24 bytes raw data] = uint32 data[0..5] // first 6 type-specific slots
//	float   size
//
// If the entry is not in the catalog, the "not found" response is sent: a
// single uint32 with bit 31 set (entry | 0x80000000).
func GameObjectQuery(entry uint32, data *gamedata.Catalog) (world.Packet, error) {
	info, ok := data.GameObjectTemplate(entry)
	if !ok {
		w := wire.NewWriter(4)
		w.Uint32(entry | 0x80000000)
		b, err := w.Result()
		return world.Packet{Opcode: op.SMSG_GAMEOBJECT_QUERY_RESPONSE, Payload: b}, err
	}
	w := wire.NewWriter(256)
	w.Uint32(entry)
	w.Uint32(info.Type)
	w.Uint32(info.DisplayID)
	w.CString(info.Name, 256)
	w.Uint8(0)
	w.Uint8(0)
	w.Uint8(0)
	w.CString(info.IconName, 256)
	w.CString(info.CastBarCaption, 256)
	w.Uint8(0) // 2.0.3 unused string placeholder
	// Reference: data.append(info->raw.data, 24) — the first 24 bytes (6 uint32s)
	// of the type-specific data slots. data0..data5.
	for i := 0; i < 6; i++ {
		w.Uint32(info.Data[i])
	}
	w.Float32(info.Size)
	b, err := w.Result()
	return world.Packet{Opcode: op.SMSG_GAMEOBJECT_QUERY_RESPONSE, Payload: b}, err
}
