package mangosone8606

import (
	"fmt"
	"strconv"
	"strings"
)

// DecodeItem validates the native item_instance record, not a client packet.
func DecodeItem(data string, guid, entry, owner uint32) ([60]uint32, error) {
	var values [60]uint32
	if len(data) > 2048 {
		return values, fmt.Errorf("item record exceeds limit")
	}
	words := strings.Fields(data)
	if len(words) != len(values) {
		return values, fmt.Errorf("item record is not build 8606")
	}
	for i, w := range words {
		n, err := strconv.ParseUint(w, 10, 32)
		if err != nil {
			return values, fmt.Errorf("invalid item field %d", i)
		}
		values[i] = uint32(n)
	}
	if values[0] != guid || values[1] != 0x40000000 || values[2] != 3 || values[3] != entry || values[6] != owner || values[7] != 0 || values[14] == 0 {
		return values, fmt.Errorf("item identity, owner or count mismatch")
	}
	return values, nil
}
