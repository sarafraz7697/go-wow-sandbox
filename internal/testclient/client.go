// Package testclient supplies an independent CLIENT-side SRP calculation for
// integration tests. Production authentication never imports this package.
package testclient

import (
	"crypto/sha1"
	"encoding/binary"
	"math/big"
)

func digest(p ...[]byte) []byte {
	h := sha1.New()
	for _, b := range p {
		h.Write(b)
	}
	return h.Sum(nil)
}
func integer(b []byte) *big.Int {
	c := append([]byte(nil), b...)
	for i, j := 0, len(c)-1; i < j; i, j = i+1, j-1 {
		c[i], c[j] = c[j], c[i]
	}
	return new(big.Int).SetBytes(c)
}
func wire(n *big.Int) []byte {
	b := make([]byte, 32)
	n.FillBytes(b)
	for i := 0; i < 16; i++ {
		b[i], b[31-i] = b[31-i], b[i]
	}
	return b
}
func Challenge(name string, build uint16) []byte {
	b := make([]byte, 34+len(name))
	binary.LittleEndian.PutUint16(b[2:], uint16(len(b)-4))
	copy(b[4:], "WoW\x00")
	copy(b[8:], []byte{2, 4, 3})
	binary.LittleEndian.PutUint16(b[11:], build)
	copy(b[13:], "68x\x00")
	copy(b[17:], "niW\x00")
	copy(b[21:], "SUne")
	b[33] = byte(len(name))
	copy(b[34:], name)
	return b
}

// Proof uses a deterministic private exponent only for reproducible test clients.
func Proof(name, password string, challenge []byte) (packet []byte, key [40]byte, m2 [20]byte) {
	N := integer(challenge[38:70])
	g := big.NewInt(7)
	a := big.NewInt(0x123456789)
	A := wire(new(big.Int).Exp(g, a, N))
	B := integer(challenge[3:35])
	salt := challenge[70:102]
	x := integer(digest(salt, digest([]byte(name+":"+password))))
	u := integer(digest(A, challenge[3:35]))
	base := new(big.Int).Exp(g, x, N)
	base.Mul(base, big.NewInt(3))
	base.Sub(B, base)
	base.Mod(base, N)
	exp := new(big.Int).Mul(u, x)
	exp.Add(exp, a)
	S := wire(new(big.Int).Exp(base, exp, N))
	even, odd := make([]byte, 16), make([]byte, 16)
	for i := range even {
		even[i] = S[2*i]
		odd[i] = S[2*i+1]
	}
	h0, h1 := digest(even), digest(odd)
	for i := 0; i < 20; i++ {
		key[2*i] = h0[i]
		key[2*i+1] = h1[i]
	}
	hN, hG := digest(challenge[38:70]), digest([]byte{7})
	for i := range hN {
		hN[i] ^= hG[i]
	}
	m := digest(hN, digest([]byte(name)), salt, A, challenge[3:35], key[:])
	copy(m2[:], digest(A, m, key[:]))
	packet = make([]byte, 75)
	packet[0] = 1
	copy(packet[1:33], A)
	copy(packet[33:53], m)
	return
}
