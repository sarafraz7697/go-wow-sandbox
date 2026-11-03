package tbc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

type cryptVector struct {
	Name, Session, Derived                                     string
	ClientPlain, ClientEncrypted, ServerPlain, ServerEncrypted []string
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func vectors(t *testing.T) []cryptVector {
	t.Helper()
	b, e := os.ReadFile("testdata/crypt.json")
	if e != nil {
		t.Fatal(e)
	}
	var v []cryptVector
	if e = json.Unmarshal(b, &v); e != nil {
		t.Fatal(e)
	}
	if len(v) != 3 {
		t.Fatal("missing vectors")
	}
	return v
}
func TestIndependentCryptVectors(t *testing.T) {
	for _, v := range vectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			var session [40]byte
			copy(session[:], unhex(t, v.Session))
			key := deriveKey(session)
			if !bytes.Equal(key[:], unhex(t, v.Derived)) {
				t.Fatal("HMAC/minimal integer mismatch")
			}
			send, recv := headerCipher{key: key}, headerCipher{key: key}
			// Interleave directions to prove their counters do not share state; six
			// packets cross the 20-byte key boundary in both directions.
			for i := range v.ClientPlain {
				b := unhex(t, v.ClientEncrypted[i])
				recv.decrypt(b)
				if !bytes.Equal(b, unhex(t, v.ClientPlain[i])) {
					t.Fatalf("client header %d", i)
				}
				b = unhex(t, v.ServerPlain[i])
				send.encrypt(b)
				if !bytes.Equal(b, unhex(t, v.ServerEncrypted[i])) {
					t.Fatalf("server header %d", i)
				}
			}
		})
	}
}
