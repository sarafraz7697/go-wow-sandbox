package codec

import (
	"time"
	"wowserver/internal/character"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

func LoginPackets(p *character.Player, data *gamedata.Catalog, now time.Time) ([]world.Packet, error) {
	var packets []world.Packet
	var firstErr error
	add := func(opcode uint32, fill func(*wire.Writer)) {
		if firstErr != nil {
			return
		}
		w := wire.NewWriter(MaxServerPayload)
		fill(w)
		b, e := w.Result()
		if e != nil {
			firstErr = e
			return
		}
		packets = append(packets, world.Packet{Opcode: opcode, Payload: b})
	}
	add(op.MSG_SET_DUNGEON_DIFFICULTY, func(w *wire.Writer) { w.Uint32(0); w.Uint32(1); w.Uint32(0) })
	add(op.SMSG_LOGIN_VERIFY_WORLD, func(w *wire.Writer) {
		w.Uint32(p.Map)
		w.Float32(p.X)
		w.Float32(p.Y)
		w.Float32(p.Z)
		w.Float32(p.Orientation)
	})
	add(op.SMSG_ACCOUNT_DATA_TIMES, func(w *wire.Writer) { w.Bytes(make([]byte, 128)) })
	add(op.SMSG_FEATURE_SYSTEM_STATUS, func(w *wire.Writer) { w.Bytes([]byte{2, 0}) })
	add(op.SMSG_MOTD, func(w *wire.Writer) { w.Uint32(1); w.CString("Welcome.", 255) })
	add(op.SMSG_SET_REST_START, func(w *wire.Writer) { w.Uint32(0) })
	add(op.SMSG_BINDPOINTUPDATE, func(w *wire.Writer) {
		w.Float32(p.HomeX)
		w.Float32(p.HomeY)
		w.Float32(p.HomeZ)
		w.Uint32(p.HomeMap)
		w.Uint32(p.HomeZone)
	})
	add(op.SMSG_TUTORIAL_FLAGS, func(w *wire.Writer) {
		for _, v := range p.Tutorials {
			w.Uint32(v)
		}
	})
	add(op.SMSG_INITIAL_SPELLS, func(w *wire.Writer) {
		w.Uint8(0)
		w.Uint16(uint16(len(p.Spells)))
		for _, id := range p.Spells {
			w.Uint16(uint16(id))
			w.Uint16(0)
		}
		w.Uint16(0)
	})
	add(op.SMSG_SEND_UNLEARN_SPELLS, func(w *wire.Writer) { w.Uint32(0) })
	add(op.SMSG_ACTION_BUTTONS, func(w *wire.Writer) {
		for _, v := range p.Actions {
			w.Uint32(v)
		}
	})
	add(op.SMSG_INITIALIZE_FACTIONS, func(w *wire.Writer) {
		var flags [128]uint8
		var standing [128]int32
		for _, r := range p.Reputations {
			if i, ok := data.FactionIndex(r.Faction); ok {
				flags[i] = uint8(r.Flags)
				standing[i] = r.Standing
			}
		}
		w.Uint32(128)
		for i := range flags {
			w.Uint8(flags[i])
			w.Uint32(uint32(standing[i]))
		}
	})
	add(op.SMSG_LOGIN_SETTIMESPEED, func(w *wire.Writer) {
		n := now.Local()
		packed := uint32(n.Year()-2000)<<24 | uint32(n.Month()-1)<<20 | uint32(n.Day()-1)<<14 | uint32(n.Weekday())<<11 | uint32(n.Hour())<<6 | uint32(n.Minute())
		w.Uint32(packed)
		w.Float32(0.01666667)
	})
	add(op.SMSG_CLIENT_CONTROL_UPDATE, func(w *wire.Writer) { w.PackedGUID(uint64(p.GUID)); w.Uint8(1) })
	if firstErr != nil {
		return nil, firstErr
	}
	update, err := InitialSelfUpdate(p, data, uint32(now.UnixMilli()))
	if err != nil {
		return nil, err
	}
	packets = append(packets, update)
	add(op.SMSG_INIT_WORLD_STATES, func(w *wire.Writer) { w.Uint32(p.Map); w.Uint32(p.Zone); w.Uint32(0); w.Uint16(0) })
	add(op.SMSG_FORCE_MOVE_ROOT, func(w *wire.Writer) { w.PackedGUID(uint64(p.GUID)); w.Uint32(0) })
	return packets, firstErr
}
func NameQuery(p *character.Player) (world.Packet, error) {
	w := wire.NewWriter(128)
	w.Uint64(uint64(p.GUID))
	w.CString(p.Name, 48)
	w.Uint8(0)
	w.Uint32(uint32(p.Race))
	w.Uint32(uint32(p.Gender))
	w.Uint32(uint32(p.Class))
	w.Uint8(0)
	b, e := w.Result()
	return world.Packet{Opcode: op.SMSG_NAME_QUERY_RESPONSE, Payload: b}, e
}
