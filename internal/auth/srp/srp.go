// Package srp implements the legacy WoW SRP6 variant, not generic SRP-6a.
// Reference: mangos/realmd 095bc797730669f228d5126f4627b7d2a2e13a8f.
package srp

import (
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"errors"
	"io"
	"math/big"
)

var modulus, _ = new(big.Int).SetString("894B645E89E1535BBDAD5B8B290650530801B18EBFBF5E8FAB3C82872A3E9BB7", 16)
var generator = big.NewInt(7)

// Verification failures are distinguishable so the caller can mirror the
// reference AuthSocket behaviour: a bad public value closes the connection
// silently, only a genuine M1 mismatch is a wrong-password reply.
var (
	ErrProof         = errors.New("invalid SRP proof")
	ErrInvalidPublic = errors.New("invalid SRP public value")
	ErrReplay        = errors.New("SRP exchange already used")
)

func little(b []byte) *big.Int {
	r := append([]byte(nil), b...)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return new(big.Int).SetBytes(r)
}
func fixed(n *big.Int, size int) []byte {
	b := make([]byte, size)
	n.FillBytes(b)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return b
}
func hash(parts ...[]byte) []byte {
	h := sha1.New()
	for _, p := range parts {
		h.Write(p)
	}
	return h.Sum(nil)
}
func Modulus() [32]byte { return [32]byte(fixed(modulus, 32)) }

// Credentials expects already-normalized credentials; plaintext is never stored.
func Credentials(name, password string) (salt, verifier [32]byte, err error) {
	return CredentialsFromHash(sha1.Sum([]byte(name + ":" + password)))
}

func CredentialsFromHash(digest [20]byte) (salt, verifier [32]byte, err error) {
	if _, err = io.ReadFull(rand.Reader, salt[:]); err != nil {
		return
	}
	x := little(hash(salt[:], digest[:]))
	copy(verifier[:], fixed(new(big.Int).Exp(generator, x, modulus), 32))
	return
}

type Server struct {
	name        string
	salt        [32]byte
	verifier, b *big.Int
	Public      [32]byte
	used        bool
}

func New(name string, salt, verifier [32]byte) (*Server, error) {
	return newServer(name, salt, verifier, rand.Reader)
}
func newServer(name string, salt, verifier [32]byte, random io.Reader) (*Server, error) {
	v := little(verifier[:])
	if v.Sign() == 0 || v.Cmp(modulus) >= 0 {
		return nil, errors.New("invalid verifier")
	}
	secret := make([]byte, 19)
	if _, err := io.ReadFull(random, secret); err != nil {
		return nil, err
	}
	b := little(secret)
	if b.Sign() == 0 {
		return nil, errors.New("zero SRP secret")
	}
	B := new(big.Int).Exp(generator, b, modulus)
	B.Add(B, new(big.Int).Mul(big.NewInt(3), v))
	B.Mod(B, modulus)
	s := &Server{name: name, salt: salt, verifier: v, b: b}
	copy(s.Public[:], fixed(B, 32))
	return s, nil
}

// Verify is one-shot. No key is returned on a failed proof.
func (s *Server) Verify(public [32]byte, proof [20]byte) (key [40]byte, serverProof [20]byte, err error) {
	if s.used {
		return key, serverProof, ErrReplay
	}
	s.used = true
	A := little(public[:])
	if new(big.Int).Mod(A, modulus).Sign() == 0 {
		return key, serverProof, ErrInvalidPublic
	}
	u := little(hash(public[:], s.Public[:]))
	if u.Sign() == 0 {
		return key, serverProof, ErrInvalidPublic
	}
	S := new(big.Int).Exp(s.verifier, u, modulus)
	S.Mul(S, A)
	S.Exp(S, s.b, modulus)
	raw := fixed(S, 32)
	even, odd := make([]byte, 16), make([]byte, 16)
	for i := range even {
		even[i] = raw[2*i]
		odd[i] = raw[2*i+1]
	}
	h0, h1 := hash(even), hash(odd)
	var candidate [40]byte
	for i := 0; i < 20; i++ {
		candidate[2*i] = h0[i]
		candidate[2*i+1] = h1[i]
	}
	nHash, gHash := hash(fixed(modulus, 32)), hash([]byte{7})
	for i := range nHash {
		nHash[i] ^= gHash[i]
	}
	expected := hash(nHash, hash([]byte(s.name)), s.salt[:], public[:], s.Public[:], candidate[:])
	if subtle.ConstantTimeCompare(expected, proof[:]) != 1 {
		return key, serverProof, ErrProof
	}
	copy(serverProof[:], hash(public[:], proof[:], candidate[:]))
	return candidate, serverProof, nil
}
