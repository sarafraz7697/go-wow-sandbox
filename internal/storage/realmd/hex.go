package realmd

import (
	"encoding/hex"
	"errors"
	"strings"
)

func encodeNumber(little []byte) string {
	b := append([]byte(nil), little...)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return strings.ToUpper(hex.EncodeToString(b))
}
func decodeNumber(value string, width int) ([]byte, error) {
	if value == "" || len(value) > width*2 {
		return nil, errors.New("invalid stored integer width")
	}
	value = strings.Repeat("0", width*2-len(value)) + value
	b, err := hex.DecodeString(value)
	if err != nil {
		return nil, errors.New("invalid stored integer encoding")
	}
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return b, nil
}
