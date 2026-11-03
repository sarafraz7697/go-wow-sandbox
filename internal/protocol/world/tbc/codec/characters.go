package codec

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
	"wowserver/internal/character"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/wire"
)

// Pinned Player::BuildEnumData: full player GUID, 19 equipment slots plus an
// empty first-bag slot, each display u32 / inventory u8 / enchant visual u32.
func EncodeCharacter(c character.Summary, data *gamedata.Catalog) ([]byte, error) {
	if c.GUID == 0 || c.Gender > 1 || c.Level == 0 {
		return nil, errors.New("invalid character identity")
	}
	if !utf8.ValidString(c.Name) || utf8.RuneCountInString(c.Name) > 12 || c.Name == "" || strings.ContainsRune(c.Name, 0) {
		return nil, errors.New("invalid character name encoding")
	}
	if _, ok := data.Start(uint32(c.Race), uint32(c.Class)); !ok {
		return nil, errors.New("unsupported race/class")
	}
	for _, v := range []float32{c.X, c.Y, c.Z} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return nil, errors.New("invalid character position")
		}
	}
	cache, err := equipmentWords(c.EquipmentCache)
	if err != nil {
		return nil, err
	}
	w := wire.NewWriter(512)
	w.Uint64(uint64(c.GUID)) // HIGHGUID_PLAYER is zero; not a packed GUID.
	w.CString(c.Name, 48)
	w.Uint8(c.Race)
	w.Uint8(c.Class)
	w.Uint8(c.Gender)
	for shift := uint(0); shift < 32; shift += 8 {
		w.Uint8(byte(c.PlayerBytes >> shift))
	}
	w.Uint8(byte(c.PlayerBytes2))
	w.Uint8(c.Level)
	w.Uint32(c.Zone)
	w.Uint32(c.Map)
	w.Float32(c.X)
	w.Float32(c.Y)
	w.Float32(c.Z)
	w.Uint32(c.Guild)
	flags := uint32(0x02000000) // declined names disabled, as reference default
	flags |= c.PlayerFlags & (0x400 | 0x800)
	if c.PlayerFlags&0x10 != 0 {
		flags |= 0x2000
	}
	if c.AtLogin&1 != 0 {
		flags |= 0x4000
	}
	w.Uint32(flags)
	if c.AtLogin&0x20 != 0 {
		w.Uint8(1)
	} else {
		w.Uint8(0)
	}
	petDisplay, petLevel, petFamily := uint32(0), uint32(0), uint32(0)
	if c.PlayerFlags&0x10 == 0 && (c.Class == 3 || c.Class == 9) {
		if pet, ok := data.Creature(c.PetEntry); ok {
			petDisplay, petLevel, petFamily = c.PetDisplay, c.PetLevel, pet.Family
		}
	}
	w.Uint32(petDisplay)
	w.Uint32(petLevel)
	w.Uint32(petFamily)
	for slot := 0; slot < 19; slot++ {
		item, ok := data.Item(cache[slot*2])
		if !ok {
			w.Uint32(0)
			w.Uint8(0)
			w.Uint32(0)
			continue
		}
		visual := uint32(0)
		enchants := cache[slot*2+1]
		for shift := uint(0); shift <= 16; shift += 16 {
			if id := (enchants >> shift) & 0xffff; id != 0 {
				if enchant, ok := data.Enchantment(id); ok {
					visual = enchant.Visual
					break
				}
			}
		}
		w.Uint32(item.Display)
		w.Uint8(byte(item.InventoryType))
		w.Uint32(visual)
	}
	w.Uint32(0)
	w.Uint8(0)
	w.Uint32(0)
	return w.Result()
}
func equipmentWords(value string) ([38]uint32, error) {
	var words [38]uint32
	if len(value) > 4096 {
		return words, errors.New("equipment cache exceeds limit")
	}
	// Short/empty caches represent missing equipment, as in the reference.
	fields := strings.Fields(value)
	for i := 0; i < len(fields) && i < len(words); i++ {
		n, err := strconv.ParseUint(fields[i], 10, 32)
		if err != nil {
			return words, errors.New("invalid equipment cache number")
		}
		words[i] = uint32(n)
	}
	return words, nil
}
