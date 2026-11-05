// Package mangosone8606 encodes the native MaNGOS One build-8606 persistence format.
package mangosone8606

import (
	"math"
	"strconv"
	"strings"
	"wowserver/internal/domain/character"
)

// Build-8606 ITEM_END is 60 words. This is the native MaNGOS item_instance
// format, not a network object update or a custom inventory representation.
func EncodeItem(guid, owner uint32, item character.InitialItem) string {
	v := make([]uint32, 60)
	v[0] = guid
	v[1] = 0x40000000
	v[2] = 3
	v[3] = item.Template.ItemID
	v[4] = math.Float32bits(1)
	v[6] = owner
	v[8] = owner
	v[14] = item.Count
	v[15] = uint32(item.Template.Duration)
	for i, sp := range item.Template.Spells {
		v[16+i] = uint32(sp.SpellCharges)
	}
	if item.Template.Bonding == 1 || item.Template.Bonding == 4 || (item.Template.Bonding == 2 && item.Slot < 19) {
		v[21] = 1
	}
	v[58] = item.Template.MaxDurability
	v[59] = item.Template.MaxDurability
	return uints(v)
}

func uints(values []uint32) string {
	var b strings.Builder
	for _, v := range values {
		b.WriteString(strconv.FormatUint(uint64(v), 10))
		b.WriteByte(' ')
	}
	return b.String()
}
