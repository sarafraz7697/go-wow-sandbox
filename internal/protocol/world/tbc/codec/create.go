package codec

import (
	"wowserver/internal/domain/character"
	"wowserver/internal/protocol/wire"
)

func DecodeCreate(payload []byte) (character.CreateRequest, error) {
	r := wire.NewReader(payload)
	v := character.CreateRequest{Name: r.CString(48), Race: r.Uint8(), Class: r.Uint8(), Gender: r.Uint8(), Skin: r.Uint8(), Face: r.Uint8(), HairStyle: r.Uint8(), HairColor: r.Uint8(), FacialHair: r.Uint8(), Outfit: r.Uint8()}
	return v, r.Finish()
}

// DecodeDeleteRequest reads the 8-byte player GUID from CMSG_CHAR_DELETE.
// The GUID is a full uint64; HIGHGUID_PLAYER is zero in TBC, so the low 32
// bits are the character guid.
func DecodeDeleteRequest(payload []byte) (uint32, error) {
	r := wire.NewReader(payload)
	guid := r.Uint64()
	if err := r.Finish(); err != nil {
		return 0, err
	}
	if guid>>32 != 0 {
		return 0, ErrWorldPacket
	}
	return uint32(guid), nil
}
