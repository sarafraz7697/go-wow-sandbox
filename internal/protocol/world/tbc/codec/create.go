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
