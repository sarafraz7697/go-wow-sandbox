package tbc

import (
	"crypto/sha1"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"wowserver/internal/account"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

const authOK byte = 0x0c
const authFailed byte = 0x0d
const authVersionMismatch byte = 0x14
const authAlreadyOnline byte = 0x1d

var errWorldAuth = errors.New("world authentication rejected")
var errWorldPacket = errors.New("malformed or out-of-state World packet")

type authRequest struct {
	Build  uint32
	Name   string
	Seed   uint32
	Digest [20]byte
	Addons []byte
}

func decodeAuthRequest(payload []byte) (authRequest, error) {
	r := wire.NewReader(payload)
	a := authRequest{Build: r.Uint32()}
	r.Uint32() // server id, unused in reference
	a.Name = r.CString(16)
	a.Seed = r.Uint32()
	copy(a.Digest[:], r.Bytes(20))
	a.Addons = r.Bytes(r.Remaining())
	if err := r.Finish(); err != nil {
		return a, err
	}
	if _, err := account.Normalize(a.Name); err != nil {
		return a, errWorldAuth
	}
	return a, nil
}
func verifyWorldDigest(a authRequest, serverSeed uint32, key [40]byte) bool {
	h := sha1.New()
	h.Write([]byte(a.Name))
	h.Write([]byte{0, 0, 0, 0})
	var seeds [8]byte
	binary.LittleEndian.PutUint32(seeds[:4], a.Seed)
	binary.LittleEndian.PutUint32(seeds[4:], serverSeed)
	h.Write(seeds[:])
	n := len(key)
	for n > 0 && key[n-1] == 0 {
		n--
	}
	h.Write(key[:n])
	return subtle.ConstantTimeCompare(h.Sum(nil), a.Digest[:]) == 1
}
func authResponse(code, expansion byte) world.Packet {
	p := world.Packet{Opcode: op.SMSG_AUTH_RESPONSE, Payload: []byte{code}}
	if code == authOK {
		p.Payload = make([]byte, 11)
		p.Payload[0] = code
		p.Payload[10] = expansion
	}
	return p
}
