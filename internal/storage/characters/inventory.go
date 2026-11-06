package characters

import (
	"context"
	"database/sql"
	"strings"
	"sync"

	"wowserver/internal/domain/character"
	"wowserver/internal/storage/characters/mangosone8606"
)

var itemGUIDMu sync.Mutex

// AllocateItemGUID reserves the next item guid (ObjectMgr::GenerateLowGuid
// (HIGHGUID_ITEM)). The guid is reserved by inserting a placeholder
// item_instance row owned by the player, so a concurrent character creation
// (which takes MAX+1) cannot hand out the same value.
func (s *Store) AllocateItemGUID(ctx context.Context, owner uint32) (uint32, error) {
	itemGUIDMu.Lock()
	defer itemGUIDMu.Unlock()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var next uint32
	if err = tx.QueryRowContext(ctx, "SELECT GREATEST((SELECT COALESCE(MAX(guid),0) FROM item_instance),(SELECT COALESCE(MAX(item),0) FROM character_inventory),(SELECT COALESCE(MAX(item_guid),0) FROM mail_items))+1").Scan(&next); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO item_instance (guid,owner_guid,data,`text`) VALUES (?,?,'','')", next, owner); err != nil {
		return 0, err
	}
	return next, tx.Commit()
}

// syncInventory rewrites the character's inventory rows from the in-world
// state: item_instance is upserted (keeping the `text` column), rows for
// destroyed items are deleted, and character_inventory is replaced.
func syncInventory(ctx context.Context, tx *sql.Tx, p *character.Player) error {
	if len(p.RemovedItems) > 0 {
		marks := strings.TrimSuffix(strings.Repeat("?,", len(p.RemovedItems)), ",")
		args := make([]any, 0, len(p.RemovedItems)+1)
		args = append(args, p.GUID)
		for _, g := range p.RemovedItems {
			args = append(args, g)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM item_instance WHERE owner_guid=? AND guid IN ("+marks+")", args...); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid=?", p.GUID); err != nil {
		return err
	}
	for _, it := range p.Items {
		if _, err := tx.ExecContext(ctx, "INSERT INTO item_instance (guid,owner_guid,data,`text`) VALUES (?,?,?,'') ON DUPLICATE KEY UPDATE owner_guid=VALUES(owner_guid),data=VALUES(data)", it.GUID, p.GUID, mangosone8606.EncodeFields(it.Fields)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO character_inventory (guid,bag,slot,item,item_template) VALUES (?,?,?,?,?)", p.GUID, it.Bag, it.Slot, it.GUID, it.Entry); err != nil {
			return err
		}
	}
	return nil
}
