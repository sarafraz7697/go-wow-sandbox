package codec

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"testing"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

func compressedAddons(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zlib.NewWriter(&buf)
	if _, err := z.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(len(data)))
	return append(b, buf.Bytes()...)
}
func TestAddonReplyAndBounds(t *testing.T) {
	// TBC record: name\0, CRC, unknown32, unknown8.
	data := append([]byte("Blizzard\x00"), []byte{1, 0x6d, 0x77, 0x1c, 0, 0, 0, 0, 0}...)
	b, err := AddonResponse(compressedAddons(t, data))
	if err != nil || !bytes.Equal(b, []byte{2, 1, 0, 0, 0, 0, 0, 0}) {
		t.Fatal("standard addon reply", err)
	}
	data[9] = 0
	b, err = AddonResponse(compressedAddons(t, data))
	if err != nil || len(b) != 264 || !bytes.Equal(b[:3], []byte{2, 1, 1}) || !bytes.Equal(b[3:259], addonPublicKey[:]) {
		t.Fatal("custom addon reply", err)
	}
	if b, e := AddonResponse([]byte{0, 0, 0, 0}); e != nil || len(b) != 0 {
		t.Fatal("empty addon list")
	}
	valid := compressedAddons(t, data)
	badSize := append([]byte(nil), valid...)
	binary.LittleEndian.PutUint32(badSize, 1)
	for _, p := range [][]byte{nil, {0, 0, 0}, {0, 0, 0, 0, 1}, {0, 0, 0x10, 0}, valid[:len(valid)-1], append(append([]byte(nil), valid...), 1), badSize, compressedAddons(t, []byte("no terminator"))} {
		if _, e := AddonResponse(p); e == nil {
			t.Fatal("malformed addon accepted")
		}
	}
}
func TestCharacterScreenReplies(t *testing.T) {
	for _, tc := range []struct{ request, response world.Packet }{
		{world.Packet{Opcode: op.CMSG_CHAR_ENUM}, world.Packet{Opcode: op.SMSG_CHAR_ENUM, Payload: []byte{0}}},
		{world.Packet{Opcode: op.CMSG_PING, Payload: []byte{1, 2, 3, 4, 0, 0, 0, 0}}, world.Packet{Opcode: op.SMSG_PONG, Payload: []byte{1, 2, 3, 4}}},
		{world.Packet{Opcode: op.CMSG_CHAR_DELETE, Payload: make([]byte, 8)}, world.Packet{Opcode: op.SMSG_CHAR_DELETE, Payload: []byte{0x3c}}},
		{world.Packet{Opcode: op.CMSG_PLAYER_LOGIN, Payload: make([]byte, 8)}, world.Packet{Opcode: op.SMSG_CHARACTER_LOGIN_FAILED, Payload: []byte{0x45}}},
	} {
		p, e := CharacterScreenPacket(tc.request)
		if e != nil || p.Opcode != tc.response.Opcode || !bytes.Equal(p.Payload, tc.response.Payload) {
			t.Fatal("reply mismatch", e)
		}
	}
	for _, p := range []world.Packet{{Opcode: op.CMSG_PING, Payload: make([]byte, 7)}, {Opcode: op.CMSG_CHAR_ENUM, Payload: []byte{1}}, {Opcode: op.CMSG_AUTH_SESSION}, {Opcode: op.CMSG_CHAR_DELETE, Payload: make([]byte, 9)}} {
		if _, e := CharacterScreenPacket(p); e == nil {
			t.Fatal("invalid packet accepted")
		}
	}
	p, e := CharacterScreenPacket(world.Packet{Opcode: op.CMSG_REALM_SPLIT, Payload: []byte{255, 255, 255, 255}})
	if e != nil || !bytes.Equal(p.Payload, append([]byte{255, 255, 255, 255, 0, 0, 0, 0}, []byte("01/01/01\x00")...)) {
		t.Fatal("realm split", e)
	}
}
func FuzzAuthSessionDecode(f *testing.F) {
	f.Add([]byte{0, 0, 0, 0})
	f.Add(make([]byte, 64))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > MaxClientSize {
			return
		}
		DecodeAuthRequest(b)
		AddonResponse(b)
	})
}
