// Package inventory ports the player-inventory logic of the MaNGOS One 2.4.3
// reference (PlayerItem*.cpp, ItemHandler.cpp, Item.cpp): item positions,
// CanStoreItem / CanEquipItem / CanUnequipItem, Swap/Split/Destroy/AutoEquip.
//
// The package is pure domain logic: every mutation records ordered Events that
// mirror what the reference does toward the client, so the protocol layer can
// emit the same packets in the same order.
package inventory

import (
	"sort"

	"wowserver/internal/domain/character"
	"wowserver/internal/entity"
)

// Slot layout (Player.h).
const (
	NullBag  uint8 = 0
	NullSlot uint8 = 255
	Bag0     uint8 = 255 // INVENTORY_SLOT_BAG_0: the player itself

	EquipmentStart = 0
	EquipmentEnd   = 19
	BagStart       = 19
	BagEnd         = 23
	ItemStart      = 23
	ItemEnd        = 39
	BankItemStart  = 39
	BankItemEnd    = 67
	BankBagStart   = 67
	BankBagEnd     = 74
	BuybackStart   = 74
	BuybackEnd     = 86
	KeyringStart   = 86
	KeyringEnd     = 118
	PlayerSlotEnd  = 118

	EquipMainhand = 15
	EquipOffhand  = 16
	EquipRanged   = 17
	maxBagSize    = 36
)

// Item field offsets inside character.PlayerItem.Fields (ITEM_FIELD_*).
const (
	fCount       = 14
	fFlags       = 21
	fContained   = 8
	fOwner       = 6
	dynBinded    = 0x00000001 // ITEM_DYNFLAG_BINDED
	dynWrapped   = 0x00000008
	guidHighItem = 0x40000000
)

// Item flags / bag families / classes used by the checks.
const (
	flagIndestructible = 0x00000020
	flagUniqueEquipped = 0x00080000

	bagFamilyArrows      = 0x00000001
	bagFamilyBullets     = 0x00000002
	bagFamilySoulShards  = 0x00000004
	bagFamilyLeather     = 0x00000008
	bagFamilyHerbs       = 0x00000020
	bagFamilyEnchanting  = 0x00000040
	bagFamilyEngineering = 0x00000080
	bagFamilyKeys        = 0x00000100
	bagFamilyGems        = 0x00000200
	bagFamilyMining      = 0x00000400

	classContainer  = 1
	classWeapon     = 2
	classArmor      = 4
	classProjectile = 6
	classQuiver     = 11

	subContainer         = 0
	subSoulContainer     = 1
	subHerbContainer     = 2
	subEnchantContainer  = 3
	subEngineerContainer = 4
	subGemContainer      = 5
	subMiningContainer   = 6
	subLeatherContainer  = 7
	subQuiver            = 2
	subAmmoPouch         = 3
)

// Inventory types (INVTYPE_*).
const (
	invHead        = 1
	invNeck        = 2
	invShoulders   = 3
	invBody        = 4
	invChest       = 5
	invWaist       = 6
	invLegs        = 7
	invFeet        = 8
	invWrists      = 9
	invHands       = 10
	invFinger      = 11
	invTrinket     = 12
	invWeapon      = 13
	invShield      = 14
	invRanged      = 15
	invCloak       = 16
	inv2HWeapon    = 17
	invBag         = 18
	invTabard      = 19
	invRobe        = 20
	invWeaponMain  = 21
	invWeaponOff   = 22
	invHoldable    = 23
	invThrown      = 25
	invRangedRight = 26
	invRelic       = 28
)

// Classes (CLASS_*) used by FindEquipSlot relic rules.
const (
	classWarlock = 9
	classPaladin = 2
	classShaman  = 7
	classDruid   = 11
)

// ProtoFunc resolves an item_template row.
type ProtoFunc func(entry uint32) (entity.ItemPrototype, bool)

// PlayerState carries the player facts the reference functions read.
type PlayerState struct {
	Level                uint8
	Race                 uint8
	Class                uint8
	Alive                bool
	InCombat             bool // Unit::IsInCombat
	Stunned              bool // hasUnitState(UNIT_STAT_STUNNED)
	LoggingOut           bool // GetSession()->isLogingOut()
	Casting              bool // IsNonMeleeSpellCasted(false)
	CanDualWield         bool // Player::CanDualWield
	WeaponSwitchCooldown bool // m_weaponChangeTimer != 0
	SkillValue           func(skill uint32) uint16
	HasSpell             func(spell uint32) bool
	ReputationRank       func(faction uint32) uint32
}

func (s PlayerState) skill(id uint32) uint16 {
	if s.SkillValue == nil {
		return 0
	}
	return s.SkillValue(id)
}
func (s PlayerState) hasSpell(id uint32) bool { return s.HasSpell != nil && s.HasSpell(id) }

// Result is InventoryResult (EQUIP_ERR_*), values exactly as in Item.h.
type Result uint8

const (
	ErrOK                        Result = 0
	ErrCantEquipLevel            Result = 1
	ErrCantEquipSkill            Result = 2
	ErrItemDoesntGoToSlot        Result = 3
	ErrNonemptyBagOverOtherBag   Result = 5
	ErrNoRequiredProficiency     Result = 8
	ErrNoEquipmentSlotAvailable  Result = 9
	ErrYouCanNeverUseThatItem    Result = 10
	ErrCantEquipWithTwohanded    Result = 13
	ErrCantDualWield             Result = 14
	ErrItemDoesntGoIntoBag       Result = 15
	ErrCantCarryMoreOfThis       Result = 17
	ErrItemCantStack             Result = 19
	ErrItemCantBeEquipped        Result = 20
	ErrItemsCantBeSwapped        Result = 21
	ErrItemNotFound              Result = 23
	ErrCantDropSoulbound         Result = 24
	ErrTriedToSplitMoreThanCount Result = 26
	ErrCouldntSplitItems         Result = 27
	ErrCanOnlyDoWithEmptyBags    Result = 31
	ErrDontOwnThatItem           Result = 32
	ErrCanEquipOnly1Quiver       Result = 33
	ErrTooFarAwayFromBank        Result = 35
	ErrYouAreStunned             Result = 37
	ErrYouAreDead                Result = 38
	ErrCantDoRightNow            Result = 39
	ErrCanEquipOnly1AmmoPouch    Result = 42
	ErrAlreadyLooted             Result = 49
	ErrInventoryFull             Result = 50
	ErrNotInCombat               Result = 60
	ErrCantEquipReputation       Result = 64
	ErrItemUniqueEquipable       Result = 67
)

// Pos packs (bag<<8)|slot.
func Pos(bag, slot uint8) uint16 { return uint16(bag)<<8 | uint16(slot) }

// Item is one carried item. Bag is the INVENTORY slot of the containing bag
// (19..22 or 67..73), or Bag0 when the item sits directly on the player.
type Item struct {
	GUID, Entry uint32
	Bag, Slot   uint8
	Fields      [60]uint32
	proto       entity.ItemPrototype
	stash       []*Item // contents of a bag while the bag item is detached (moving)
}

func (i *Item) Count() uint32               { return i.Fields[fCount] }
func (i *Item) setCount(c uint32)           { i.Fields[fCount] = c }
func (i *Item) Pos() uint16                 { return Pos(i.Bag, i.Slot) }
func (i *Item) Proto() entity.ItemPrototype { return i.proto }
func (i *Item) IsBag() bool                 { return i.proto.InventoryType == invBag || i.proto.InventoryType == 27 }
func (i *Item) BagSize() uint32             { return i.proto.ContainerSlots }
func (i *Item) Binded() bool                { return i.Fields[fFlags]&dynBinded != 0 }

// EventKind says what the reference does toward the client.
type EventKind uint8

const (
	EvItemCreate  EventKind = iota + 1 // Item::SendCreateUpdateToPlayer (full create block of Snap)
	EvItemDestroy                      // Object::DestroyForPlayer
	EvInvSlot                          // PLAYER_FIELD_INV_SLOT_HEAD + slot*2 := Value (item guid low, 0 = cleared)
	EvBagSlot                          // CONTAINER_FIELD_SLOT_1 + slot*2 of bag item GUID := Value
	EvVisible                          // SetVisibleItemSlot(slot, entry); Value 0 clears
)

// Event is one ordered client/persistence consequence of an operation.
type Event struct {
	Kind  EventKind
	GUID  uint32 // item guid (for EvBagSlot: the bag's guid)
	Slot  uint8
	Value uint32
	Snap  Item     // EvItemCreate: item state at that moment
	Bag   []uint32 // EvItemCreate for a bag: item guid per bag slot (0 = empty)
}

// Failure carries what Player::SendEquipError serializes.
type Failure struct {
	Code          Result
	Item1, Item2  uint32 // item guid low, 0 = none
	ItemID        uint32 // proto lookup for ErrCantEquipLevel when Item1 == 0
	RequiredLevel uint32
}

// Hooks are called where the reference applies/removes item modifiers.
type Hooks interface {
	ApplyItemMods(item *Item, slot uint8, apply bool)
	RemoveItemDependentAuras(item *Item)
}

type noHooks struct{}

func (noHooks) ApplyItemMods(*Item, uint8, bool) {}
func (noHooks) RemoveItemDependentAuras(*Item)   {}

// Inventory is the in-memory container model of one player.
type Inventory struct {
	playerGUID uint32
	proto      ProtoFunc
	st         PlayerState
	newGUID    func() uint32
	hooks      Hooks
	pos        map[uint16]*Item
	guid       map[uint32]*Item
	ev         []Event
}

// New builds the model from the stored items (character.PlayerItem.Bag holds
// the GUID of the containing bag item, 0 for the player).
func New(items []character.PlayerItem, playerGUID uint32, proto ProtoFunc, st PlayerState, newItemGUID func() uint32) *Inventory {
	inv := &Inventory{playerGUID: playerGUID, proto: proto, st: st, newGUID: newItemGUID, hooks: noHooks{},
		pos: make(map[uint16]*Item, len(items)), guid: make(map[uint32]*Item, len(items))}
	bagSlot := map[uint32]uint8{}
	for _, it := range items {
		if it.Bag == 0 {
			bagSlot[it.GUID] = it.Slot
		}
	}
	for _, it := range items {
		p, ok := proto(it.Entry)
		if !ok {
			continue
		}
		n := &Item{GUID: it.GUID, Entry: it.Entry, Slot: it.Slot, Fields: it.Fields, proto: p, Bag: Bag0}
		if it.Bag != 0 {
			bs, ok := bagSlot[it.Bag]
			if !ok {
				continue
			}
			n.Bag = bs
		}
		inv.pos[n.Pos()] = n
		inv.guid[n.GUID] = n
	}
	return inv
}

// SetHooks installs the stat-application hooks.
func (inv *Inventory) SetHooks(h Hooks) {
	if h == nil {
		h = noHooks{}
	}
	inv.hooks = h
}

// SetState refreshes the player facts (level, combat, ...).
func (inv *Inventory) SetState(st PlayerState) { inv.st = st }

// Items returns the current state in storage form.
func (inv *Inventory) Items() []character.PlayerItem {
	out := make([]character.PlayerItem, 0, len(inv.guid))
	for _, it := range inv.guid {
		var bag uint32
		if it.Bag != Bag0 {
			if b := inv.pos[Pos(Bag0, it.Bag)]; b != nil {
				bag = b.GUID
			}
		}
		out = append(out, character.PlayerItem{GUID: it.GUID, Entry: it.Entry, Bag: bag, Slot: it.Slot, Fields: it.Fields})
	}
	// Stable order: player-level items first (bags before their contents),
	// like the storage query's ORDER BY bag,slot.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Bag != out[j].Bag {
			return out[i].Bag < out[j].Bag
		}
		return out[i].Slot < out[j].Slot
	})
	return out
}

// ItemByPos ports Player::GetItemByPos(bag, slot).
func (inv *Inventory) ItemByPos(bag, slot uint8) *Item { return inv.pos[Pos(bag, slot)] }

// ItemByGUID finds an item by guid low.
func (inv *Inventory) ItemByGUID(g uint32) *Item { return inv.guid[g] }

// ---- position predicates (PlayerItem.cpp) ----

func IsInventoryPos(bag, slot uint8) bool {
	if bag == Bag0 && slot == NullSlot {
		return true
	}
	if bag == Bag0 && slot >= ItemStart && slot < ItemEnd {
		return true
	}
	if bag >= BagStart && bag < BagEnd {
		return true
	}
	if bag == Bag0 && slot >= KeyringStart && slot < KeyringEnd {
		return true
	}
	return false
}

func IsEquipmentPos(bag, slot uint8) bool {
	if bag == Bag0 && slot < EquipmentEnd {
		return true
	}
	return bag == Bag0 && slot >= BagStart && slot < BagEnd
}

func IsBankPos(bag, slot uint8) bool {
	if bag == Bag0 && slot >= BankItemStart && slot < BankItemEnd {
		return true
	}
	if bag == Bag0 && slot >= BankBagStart && slot < BankBagEnd {
		return true
	}
	return bag >= BankBagStart && bag < BankBagEnd
}

func IsBagPos(p uint16) bool {
	bag, slot := uint8(p>>8), uint8(p&255)
	if bag == Bag0 && slot >= BagStart && slot < BagEnd {
		return true
	}
	return bag == Bag0 && slot >= BankBagStart && slot < BankBagEnd
}

func posParts(p uint16) (uint8, uint8) { return uint8(p >> 8), uint8(p & 255) }

// IsValidPos ports Player::IsValidPos.
func (inv *Inventory) IsValidPos(bag, slot uint8, explicit bool) bool {
	if bag == NullBag && !explicit {
		return true
	}
	if bag == Bag0 {
		if slot == NullSlot && !explicit {
			return true
		}
		switch {
		case slot < EquipmentEnd, slot >= BagStart && slot < BagEnd, slot >= ItemStart && slot < ItemEnd,
			slot >= KeyringStart && slot < KeyringEnd, slot >= BankItemStart && slot < BankItemEnd,
			slot >= BankBagStart && slot < BankBagEnd:
			return true
		}
		return false
	}
	if (bag >= BagStart && bag < BagEnd) || (bag >= BankBagStart && bag < BankBagEnd) {
		b := inv.ItemByPos(Bag0, bag)
		if b == nil {
			return false
		}
		if slot == NullSlot && !explicit {
			return true
		}
		return uint32(slot) < b.BagSize()
	}
	return false
}

// ---- small helpers ----

func (inv *Inventory) emit(e Event) { inv.ev = append(inv.ev, e) }

func (inv *Inventory) bagContents(b *Item) []uint32 {
	out := make([]uint32, b.BagSize())
	for s := uint32(0); s < b.BagSize(); s++ {
		var it *Item
		if b.stash != nil {
			if int(s) < len(b.stash) {
				it = b.stash[s]
			}
		} else {
			it = inv.pos[Pos(b.Slot, uint8(s))]
		}
		if it != nil {
			out[s] = it.GUID
		}
	}
	return out
}

// detachBag lifts a bag's contents out of the position map while the bag
// item itself is moved (the reference's items belong to the Bag object).
func (inv *Inventory) detachBag(b *Item) {
	if !b.IsBag() || b.Bag != Bag0 {
		return
	}
	b.stash = make([]*Item, b.BagSize())
	for s := uint32(0); s < b.BagSize(); s++ {
		if x := inv.pos[Pos(b.Slot, uint8(s))]; x != nil {
			b.stash[s] = x
			delete(inv.pos, Pos(b.Slot, uint8(s)))
		}
	}
}

// attachBag puts a bag's contents back under the bag's (new) slot.
func (inv *Inventory) attachBag(b *Item) {
	for s, x := range b.stash {
		if x == nil {
			continue
		}
		x.Bag, x.Slot = b.Slot, uint8(s)
		inv.pos[x.Pos()] = x
	}
	b.stash = nil
}

// bagIsEmpty ports Bag::IsEmpty.
func (inv *Inventory) bagIsEmpty(b *Item) bool {
	for s := uint32(0); s < b.BagSize(); s++ {
		if inv.pos[Pos(b.Slot, uint8(s))] != nil {
			return false
		}
	}
	return true
}

func (inv *Inventory) snapCreate(it *Item) {
	e := Event{Kind: EvItemCreate, GUID: it.GUID, Snap: *it}
	if it.IsBag() && it.Bag == Bag0 || it.stash != nil {
		e.Bag = inv.bagContents(it)
	}
	inv.emit(e)
}

func maxStack(p entity.ItemPrototype) uint32 {
	if p.Stackable <= 0 {
		return 1
	}
	return uint32(p.Stackable)
}

// itemCanGoIntoBag ports ItemCanGoIntoBag (Item.cpp).
func itemCanGoIntoBag(p, bag entity.ItemPrototype) bool {
	switch bag.Class {
	case classContainer:
		switch bag.SubClass {
		case subContainer:
			return true
		case subSoulContainer:
			return p.BagFamily == bagFamilySoulShards
		case subHerbContainer:
			return p.BagFamily == bagFamilyHerbs
		case subEnchantContainer:
			return p.BagFamily == bagFamilyEnchanting
		case subMiningContainer:
			return p.BagFamily == bagFamilyMining
		case subEngineerContainer:
			return p.BagFamily == bagFamilyEngineering
		case subGemContainer:
			return p.BagFamily == bagFamilyGems
		case subLeatherContainer:
			return p.BagFamily == bagFamilyLeather
		}
		return false
	case classQuiver:
		switch bag.SubClass {
		case subQuiver:
			return p.BagFamily == bagFamilyArrows
		case subAmmoPouch:
			return p.BagFamily == bagFamilyBullets
		}
		return false
	}
	return false
}

// canChangeEquipStateInCombat ports ItemPrototype::CanChangeEquipStateInCombat.
func canChangeEquipStateInCombat(p entity.ItemPrototype) bool {
	switch p.InventoryType {
	case invRelic, invShield, invHoldable:
		return true
	}
	return p.Class == classWeapon || p.Class == classProjectile
}

// itemSkill ports Item::GetSkill.
func itemSkill(p entity.ItemPrototype) uint32 {
	weapon := [...]uint32{44, 172, 45, 46, 54, 160, 229, 43, 55, 0, 136, 0, 0, 162, 0, 173, 176, 253, 226, 228, 356}
	armor := [...]uint32{0, 415, 414, 413, 293, 0, 433, 0, 0, 0}
	switch p.Class {
	case classWeapon:
		if int(p.SubClass) < len(weapon) {
			return weapon[p.SubClass]
		}
	case classArmor:
		if int(p.SubClass) < len(armor) {
			return armor[p.SubClass]
		}
	}
	return 0
}
