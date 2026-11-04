package codec

import (
	"crypto/sha1"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"wowserver/internal/domain/account"
	"wowserver/internal/protocol/wire"
	"wowserver/internal/protocol/world"
	op "wowserver/internal/protocol/world/tbc/opcodes"
)

const AuthOK byte = 0x0c
const AuthFailed byte = 0x0d
const AuthVersionMismatch byte = 0x14
const AuthAlreadyOnline byte = 0x1d

var ErrWorldAuth = errors.New("world authentication rejected")
var ErrWorldPacket = errors.New("malformed or out-of-state World packet")

// ErrFatalWorldPacket marks a protocol violation that must END the session
// (the reference disconnects the client, e.g. a second CMSG_AUTH_SESSION
// mid-session; ClientConnection rejects it once auth has started).
var ErrFatalWorldPacket = errors.New("fatal world protocol violation")

type AuthRequest struct {
	Build  uint32
	Name   string
	Seed   uint32
	Digest [20]byte
	Addons []byte
}

func DecodeAuthRequest(payload []byte) (AuthRequest, error) {
	r := wire.NewReader(payload)
	a := AuthRequest{Build: r.Uint32()}
	r.Uint32() // server id, unused in reference
	a.Name = r.CString(16)
	a.Seed = r.Uint32()
	copy(a.Digest[:], r.Bytes(20))
	a.Addons = r.Bytes(r.Remaining())
	if err := r.Finish(); err != nil {
		return a, err
	}
	if _, err := account.Normalize(a.Name); err != nil {
		return a, ErrWorldAuth
	}
	return a, nil
}
func VerifyWorldDigest(a AuthRequest, serverSeed uint32, key [40]byte) bool {
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
func AuthResponse(code, expansion byte) world.Packet {
	p := world.Packet{Opcode: op.SMSG_AUTH_RESPONSE, Payload: []byte{code}}
	if code == AuthOK {
		p.Payload = make([]byte, 11)
		p.Payload[0] = code
		p.Payload[10] = expansion
	}
	return p
}
