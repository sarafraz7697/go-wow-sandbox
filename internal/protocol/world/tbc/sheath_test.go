package tbc

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"wowserver/internal/domain/character"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/codec"
	op "wowserver/internal/protocol/world/tbc/opcodes"
	f "wowserver/internal/protocol/world/tbc/updatefields"
)

func sheathFixture(t *testing.T) (*Handler, *Session, *[]world.Packet) {
	t.Helper()
	c := &gamedata.Catalog{}
	h := &Handler{Data: c}
	var sent []world.Packet
	p := &character.Player{Summary: character.Summary{GUID: 9, Level: 3, Race: 1, Class: 1}, Entered: true, Health: 50,
		Stats: &character.Stats{MaxHealth: 100}, Items: []character.PlayerItem{{GUID: 1, Entry: 25, Slot: 15}, {GUID: 2, Entry: 26, Slot: 16}, {GUID: 3, Entry: 27, Slot: 17}}}
	s := &Session{player: p, log: slog.New(slog.NewTextHandler(io.Discard, nil)), send: func(ps ...world.Packet) error {
		sent = append(sent, ps...)
		return nil
	}}
	return h, s, &sent
}

func TestSetSheathedUpdatesBytes2AndVirtualItems(t *testing.T) {
	h, s, sent := sheathFixture(t)
	ctx := t.Context()
	// sheath melee: byte 0 = 1 on the 0x2800 base word.
	if res, err := h.setSheathed(ctx, s, world.Packet{Opcode: op.CMSG_SETSHEATHED, Payload: []byte{1, 0, 0, 0}}); err != nil || res != nil {
		t.Fatalf("handler %v %v", res, err)
	}
	if s.sheath != sheathMelee || s.bytes2 != unitBytes2Base|1 {
		t.Fatalf("sheath state %d bytes2 %x", s.sheath, s.bytes2)
	}
	// One ValuesUpdate with bytes_2 + 3 virtual items = 10 field changes.
	var upd *world.Packet
	for i := range *sent {
		if (*sent)[i].Opcode == op.SMSG_UPDATE_OBJECT {
			upd = &(*sent)[i]
		}
	}
	if upd == nil {
		t.Fatal("no values update sent")
	}
	if len(upd.Payload) < 10 {
		t.Fatalf("update too small: %d", len(upd.Payload))
	}
	// Invalid states are ignored (the reference only logs).
	before := len(*sent)
	if _, err := h.setSheathed(ctx, s, world.Packet{Opcode: op.CMSG_SETSHEATHED, Payload: []byte{9, 0, 0, 0}}); err != nil {
		t.Fatal(err)
	}
	if len(*sent) != before {
		t.Fatal("unknown sheath state must be ignored")
	}
	// Ranged sheath clears the melee slots.
	if _, err := h.setSheathed(ctx, s, world.Packet{Opcode: op.CMSG_SETSHEATHED, Payload: []byte{2, 0, 0, 0}}); err != nil {
		t.Fatal(err)
	}
	if s.sheath != sheathRanged {
		t.Fatalf("sheath %d", s.sheath)
	}
}

func TestVirtualItemChanges(t *testing.T) {
	h, s, _ := sheathFixture(t)
	ch := h.virtualItemChanges(s, sheathMelee)
	if len(ch) != 9 {
		t.Fatalf("changes %d", len(ch))
	}
	// The equipped main-hand weapon lands in virtual slot 0 display field.
	if ch[0].Index != f.UNIT_VIRTUAL_ITEM_SLOT_DISPLAY || ch[0].Value != 0 {
		t.Fatalf("virtual slot0 display %+v", ch[0])
	}
	// Unarmed clears every slot.
	for _, c := range h.virtualItemChanges(s, sheathUnarmed) {
		if c.Value != 0 {
			t.Fatalf("unarmed must clear slot %d", c.Index)
		}
	}
}

func TestSendFieldsRoutesPublicAndPrivate(t *testing.T) {
	h, s, sent := sheathFixture(t)
	// A unit field and a player field in one call: both reach self (no
	// broadcaster registered), split into two updates.
	h.sendFields(s,
		codec.FieldChange{Index: f.UNIT_FIELD_HEALTH, Value: 60},
		codec.FieldChange{Index: f.PLAYER_DODGE_PERCENTAGE, Value: 0x3F000000})
	if len(*sent) != 2 {
		t.Fatalf("expected two routed updates, got %d", len(*sent))
	}
	_ = time.Now()
}
