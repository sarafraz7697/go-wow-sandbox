package codec

import (
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

// Stateless screen responses. Handler intercepts CHAR_ENUM and replaces the
// empty baseline with the account's stored list and handles CREATE separately.
func CharacterScreenPacket(p world.Packet) (*world.Packet, error) {
	r := wire.NewReader(p.Payload)
	var response world.Packet
	switch p.Opcode {
	case op.CMSG_OPT_OUT_OF_LOOT, op.CMSG_SET_TAXI_BENCHMARK_MODE, op.CMSG_SET_ACTIVE_VOICE_CHANNEL:
		// MaNGOS consumes these preferences without a response. Loot opt-out
		// activation is explicitly unimplemented there as well.
		switch p.Opcode {
		case op.CMSG_OPT_OUT_OF_LOOT:
			r.Uint32()
		case op.CMSG_SET_TAXI_BENCHMARK_MODE:
			r.Uint8()
		case op.CMSG_SET_ACTIVE_VOICE_CHANNEL:
			r.Uint32()
			r.CString(255)
		}
		return nil, r.Finish()
	case op.CMSG_VOICE_SESSION_ENABLE:
		// Voice and microphone preferences are accepted even without a voice
		// service, matching MaNGOS. This command has no response.
		r.Uint8()
		r.Uint8()
		if err := r.Finish(); err != nil {
			return nil, err
		}
		return nil, nil
	case op.CMSG_CHAR_ENUM:
		response = world.Packet{Opcode: op.SMSG_CHAR_ENUM, Payload: []byte{0}}
	case op.CMSG_PING:
		sequence := r.Uint32()
		r.Uint32() // latency
		w := wire.NewWriter(4)
		w.Uint32(sequence)
		b, _ := w.Result()
		response = world.Packet{Opcode: op.SMSG_PONG, Payload: b}
	case op.CMSG_REALM_SPLIT:
		value := r.Uint32()
		w := wire.NewWriter(17)
		w.Uint32(value)
		w.Uint32(0)
		w.CString("01/01/01", 8)
		b, _ := w.Result()
		response = world.Packet{Opcode: op.SMSG_REALM_SPLIT, Payload: b}
	case op.CMSG_KEEP_ALIVE:
		if err := r.Finish(); err != nil {
			return nil, err
		}
		return nil, nil
	case op.CMSG_CHAR_CREATE:
		r.CString(12)
		r.Bytes(9)                                                                  // race/class/gender, appearance and outfit id
		response = world.Packet{Opcode: op.SMSG_CHAR_CREATE, Payload: []byte{0x33}} // disabled
	case op.CMSG_CHAR_DELETE:
		r.Uint64()
		response = world.Packet{Opcode: op.SMSG_CHAR_DELETE, Payload: []byte{0x3c}}
	case op.CMSG_PLAYER_LOGIN:
		r.Uint64()
		response = world.Packet{Opcode: op.SMSG_CHARACTER_LOGIN_FAILED, Payload: []byte{0x46}}
	default:
		return nil, ErrWorldPacket
	}
	if err := r.Finish(); err != nil {
		return nil, err
	}
	return &response, nil
}
