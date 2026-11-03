// Package codec encodes and decodes build 8606 packets without I/O or storage.
package codec

import (
	"encoding/binary"
	"wowserver/internal/protocol/auth"
)

type LoginProof struct {
	Public [32]byte
	Digest [20]byte
}

func DecodeLoginProof(packet []byte) (LoginProof, error) {
	var p LoginProof
	if len(packet) != 75 {
		return p, auth.ErrMalformed
	}
	if packet[0] != 1 {
		return p, auth.ErrCommand
	}
	if packet[73] != 0 || packet[74] != 0 {
		return p, auth.ErrMalformed
	}
	copy(p.Public[:], packet[1:33])
	copy(p.Digest[:], packet[33:53])
	return p, nil
}
func LoginChallenge(public, salt, modulus [32]byte, nonce [16]byte) []byte {
	p := make([]byte, 119)
	copy(p[3:35], public[:])
	p[35] = 1
	p[36] = 7
	p[37] = 32
	copy(p[38:70], modulus[:])
	copy(p[70:102], salt[:])
	copy(p[102:118], nonce[:])
	return p
}
func LoginSuccess(proof [20]byte) []byte {
	p := make([]byte, 32)
	p[0] = 1
	copy(p[2:22], proof[:])
	binary.LittleEndian.PutUint32(p[22:26], 0x00800000)
	return p
}
func ChallengeFailure(command, code byte) []byte {
	if command == 2 {
		return []byte{2, code}
	}
	return []byte{0, 0, code}
}
func ProofFailure(code byte) []byte { return []byte{1, code, 3, 0} }

type ReconnectProof struct {
	Nonce  [16]byte
	Digest [20]byte
}

func DecodeReconnectProof(packet []byte) (ReconnectProof, error) {
	var p ReconnectProof
	if len(packet) != 58 {
		return p, auth.ErrMalformed
	}
	if packet[0] != 3 {
		return p, auth.ErrCommand
	}
	if packet[57] != 0 {
		return p, auth.ErrMalformed
	}
	copy(p.Nonce[:], packet[1:17])
	copy(p.Digest[:], packet[17:37])
	return p, nil
}
func ReconnectChallenge(nonce [16]byte) []byte {
	p := make([]byte, 34)
	p[0] = 2
	copy(p[2:18], nonce[:])
	return p
}
func ReconnectSuccess() []byte { return []byte{3, 0, 0, 0} }
