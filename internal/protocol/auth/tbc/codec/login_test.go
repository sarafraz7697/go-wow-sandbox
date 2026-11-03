package codec

import (
	"errors"
	"testing"
	"wowserver/internal/protocol/auth"
)

func TestProofBoundsAndExtensions(t *testing.T) {
	for n := 0; n < 80; n++ {
		p := make([]byte, n)
		if n > 0 {
			p[0] = 1
		}
		_, err := DecodeLoginProof(p)
		if (err == nil) != (n == 75) {
			t.Fatalf("login length %d: %v", n, err)
		}
		if n > 0 {
			p[0] = 3
		}
		_, err = DecodeReconnectProof(p)
		if (err == nil) != (n == 58) {
			t.Fatalf("reconnect length %d: %v", n, err)
		}
	}
	p := make([]byte, 75)
	p[0] = 1
	for _, offset := range []int{73, 74} {
		p[offset] = 1
		if _, err := DecodeLoginProof(p); !errors.Is(err, auth.ErrMalformed) {
			t.Fatal("accepted extension")
		}
		p[offset] = 0
	}
	p[0] = 16
	if _, err := DecodeLoginProof(p); !errors.Is(err, auth.ErrCommand) {
		t.Fatal("accepted realm request as proof")
	}
	r := make([]byte, 58)
	r[0] = 3
	r[57] = 1
	if _, err := DecodeReconnectProof(r); !errors.Is(err, auth.ErrMalformed) {
		t.Fatal("accepted reconnect extension")
	}
}
func FuzzProofDecoders(f *testing.F) {
	f.Add([]byte{1})
	f.Add(make([]byte, 75))
	f.Add(make([]byte, 58))
	f.Fuzz(func(t *testing.T, p []byte) { DecodeLoginProof(p); DecodeReconnectProof(p) })
}
