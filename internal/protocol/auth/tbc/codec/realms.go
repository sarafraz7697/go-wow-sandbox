package codec

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"unicode/utf8"
	"wowserver/internal/realm"
)

// EncodeRealms follows realmd LoadRealmlist's 8606 branch: u16 count,
// byte type/lock/flags, NUL strings, float32 population, 0x2c and 0x0010.
func EncodeRealms(realms []realm.Realm) ([]byte, error) {
	if len(realms) > 1024 {
		return nil, errors.New("too many realms")
	}
	body := make([]byte, 6)
	binary.LittleEndian.PutUint16(body[4:], uint16(len(realms)))
	for _, r := range realms {
		if r.Build != 8606 || r.Name == "" || len(r.Name) > 128 || !utf8.ValidString(r.Name) || strings.ContainsRune(r.Name, 0) ||
			r.Host == "" || len(r.Host) > 253 || strings.ContainsAny(r.Host, "\x00: /\\\r\n\t") || r.Port == 0 ||
			math.IsNaN(float64(r.Population)) || math.IsInf(float64(r.Population), 0) || r.Population < 0 {
			return nil, fmt.Errorf("invalid realm metadata for id %d", r.ID)
		}
		locked := byte(0)
		if r.Locked {
			locked = 1
		}
		body = append(body, r.Type, locked, r.Flags)
		body = append(body, []byte(r.Name)...)
		body = append(body, 0)
		body = append(body, []byte(net.JoinHostPort(r.Host, strconv.Itoa(int(r.Port))))...)
		body = append(body, 0)
		body = binary.LittleEndian.AppendUint32(body, math.Float32bits(r.Population))
		body = append(body, r.Characters, r.Category, 0x2c)
		if r.Flags&0x04 != 0 {
			body = append(body, 2, 4, 3)
			body = binary.LittleEndian.AppendUint16(body, 8606)
		}
		if len(body)+2 > 65535 {
			return nil, errors.New("realm list exceeds wire length")
		}
	}
	body = append(body, 0x10, 0)
	packet := []byte{0x10}
	packet = binary.LittleEndian.AppendUint16(packet, uint16(len(body)))
	return append(packet, body...), nil
}
