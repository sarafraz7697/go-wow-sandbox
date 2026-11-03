// Package frame implements build 8606 world packet framing, not a World
// server.
//
// DOMAIN_WORLD_SPLIT_PATCH: moved from internal/protocol/world/tbc/frame.go.
// The stream Reader/Writer moved to stream.go in the same package.
package frame

import (
	"encoding/binary"
	"errors"

	"wowserver/internal/protocol/world"
)

const ClientHeaderSize = 6
const ServerHeaderSize = 4

// The size field includes opcode bytes, but not the two size bytes.
const MaxClientSize = 10240
const MaxServerPayload = 65535 - 2

var ErrHeader = errors.New("invalid TBC packet header")
var ErrSize = errors.New("TBC packet size exceeds bounds")
var ErrOpcode = errors.New("TBC opcode exceeds framing bounds")

// DecodeClientHeader operates on a complete plaintext header. Opcode existence
// and session permissions are checked separately after framing, as in MaNGOS.
func DecodeClientHeader(header []byte) (opcode uint32, payloadSize int, err error) {
	if len(header) != ClientHeaderSize {
		return 0, 0, ErrHeader
	}
	size := int(binary.BigEndian.Uint16(header[:2]))
	if size < 4 || size > MaxClientSize {
		return 0, 0, ErrSize
	}
	opcode = binary.LittleEndian.Uint32(header[2:])
	if opcode > MaxClientSize {
		return 0, 0, ErrOpcode
	}
	return opcode, size - 4, nil
}

// EncodeServer never uses the WotLK five-byte header, even above 0x7fff.
func EncodeServer(packet world.Packet) ([]byte, error) {
	if len(packet.Payload) > MaxServerPayload {
		return nil, ErrSize
	}
	if packet.Opcode > 65535 {
		return nil, ErrOpcode
	}
	frame := make([]byte, ServerHeaderSize+len(packet.Payload))
	binary.BigEndian.PutUint16(frame[:2], uint16(len(packet.Payload)+2))
	binary.LittleEndian.PutUint16(frame[2:4], uint16(packet.Opcode))
	copy(frame[4:], packet.Payload)
	return frame, nil
}
