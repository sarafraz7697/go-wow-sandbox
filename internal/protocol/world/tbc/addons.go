package tbc

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"io"
	"wowserver/internal/protocol/wire"
)

var errAddons = errors.New("invalid addon data")

// The pinned TBC AddonHandler reads name, CRC, unknown uint32, unknown byte
// repeatedly (no count prefix). Limit decompression, record count and reply size.
func addonResponse(payload []byte) ([]byte, error) {
	if len(payload) < 4 {
		return nil, errAddons
	}
	size := int(binary.LittleEndian.Uint32(payload))
	if size == 0 {
		if len(payload) == 4 {
			return []byte{}, nil
		}
		return nil, errAddons
	}
	if size > 0xfffff {
		return nil, errAddons
	}
	source := bytes.NewReader(payload[4:])
	z, err := zlib.NewReader(source)
	if err != nil {
		return nil, errAddons
	}
	data, err := io.ReadAll(io.LimitReader(z, int64(size)+1))
	closeErr := z.Close()
	if err != nil || closeErr != nil || len(data) != size || source.Len() != 0 {
		return nil, errAddons
	}
	r := wire.NewReader(data)
	w := wire.NewWriter(MaxServerPayload)
	for count := 0; r.Remaining() > 0; count++ {
		if count >= 256 {
			return nil, errAddons
		}
		name := r.CString(255)
		crc := r.Uint32()
		r.Uint32()
		r.Uint8()
		if r.Err() != nil || name == "" {
			return nil, errAddons
		}
		w.Uint8(2)
		w.Uint8(1)
		if crc == 0x1c776d01 {
			w.Uint8(0)
		} else {
			w.Uint8(1)
			w.Bytes(addonPublicKey[:])
		}
		w.Uint32(0)
		w.Uint8(0)
	}
	return w.Result()
}
