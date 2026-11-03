package srp

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

type vector struct{ Label, Name, Password, Salt, Verifier, Secret, A, B, Key, Proof, ServerProof string }

func decode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestIndependentClientVectors(t *testing.T) {
	data, err := os.ReadFile("testdata/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []vector
	if err = json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors {
		t.Run(v.Label, func(t *testing.T) {
			s, err := newServer(v.Name, [32]byte(decode(t, v.Salt)), [32]byte(decode(t, v.Verifier)), bytes.NewReader(decode(t, v.Secret)))
			if err != nil {
				t.Fatal(err)
			}
			if hex.EncodeToString(s.Public[:]) != v.B {
				t.Fatal("public value mismatch")
			}
			key, proof, err := s.Verify([32]byte(decode(t, v.A)), [20]byte(decode(t, v.Proof)))
			if err != nil {
				t.Fatal(err)
			}
			if hex.EncodeToString(key[:]) != v.Key || hex.EncodeToString(proof[:]) != v.ServerProof {
				t.Fatal("proof or key mismatch")
			}
			if _, _, err = s.Verify([32]byte(decode(t, v.A)), [20]byte(decode(t, v.Proof))); err == nil {
				t.Fatal("replay accepted")
			}
		})
	}
}
func TestRejectInvalidPublicAndProof(t *testing.T) {
	salt, v, err := Credentials("TEST", "PASSWORD")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range [][32]byte{{}, Modulus(), {7}} {
		s, err := New("TEST", salt, v)
		if err != nil {
			t.Fatal(err)
		}
		k, m, err := s.Verify(a, [20]byte{})
		if err == nil || k != [40]byte{} || m != [20]byte{} {
			t.Fatal("invalid proof exposed key")
		}
	}
	if _, err := New("TEST", salt, [32]byte{}); err == nil {
		t.Fatal("zero verifier accepted")
	}
}
