package characters

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"wowserver/internal/domain/character"
)

// DeleteResultCode maps a delete error to the SMSG_CHAR_DELETE response byte.
// Pure function: no I/O. 0x49 = CHAR_DELETE_SUCCESS, 0x3a = CHAR_DELETE_FAILED.
func DeleteResultCode(err error) byte {
	if err == nil {
		return 0x49
	}
	return 0x3a
}

// Delete removes a character owned by accountID. It performs a hard delete
// across all character-owned tables in one transaction. The character must
// not be online to avoid deleting a snapshot a live session still owns.
// realmcharacters counter is not decremented here; it lives in the login
// database, which this Store does not access. The count is best-effort and
// may drift by one until the normal logout path or a future admin command
// corrects it.
func (s *Store) Delete(ctx context.Context, accountID uint64, guid uint32) error {
	if accountID == 0 || guid == 0 {
		return character.ErrPlayerUnavailable
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var online uint8
	err = tx.QueryRowContext(ctx,
		"SELECT online FROM characters WHERE guid=? AND account=? AND (deleteDate IS NULL OR deleteDate=0) FOR UPDATE",
		guid, accountID).Scan(&online)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: character missing, deleted or not owned by account", character.ErrPlayerUnavailable)
	}
	if err != nil {
		return err
	}
	if online != 0 {
		return fmt.Errorf("%w: character is online", character.ErrPlayerUnavailable)
	}
	// Collect item GUIDs before deleting inventory so item_instance can be cleaned.
	itemGUIDs, err := collectItemGUIDs(ctx, tx, guid)
	if err != nil {
		return err
	}
	// Pet IDs (character_pet.id) are also referenced by pet_aura and pet_spell.
	petIDs, err := collectPetIDs(ctx, tx, guid)
	if err != nil {
		return err
	}
	// Delete child tables keyed on guid.
	for _, q := range []string{
		"DELETE FROM character_action WHERE guid=?",
		"DELETE FROM character_aura WHERE guid=?",
		"DELETE FROM character_homebind WHERE guid=?",
		"DELETE FROM character_inventory WHERE guid=?",
		"DELETE FROM character_queststatus WHERE guid=?",
		"DELETE FROM character_queststatus_daily WHERE guid=?",
		"DELETE FROM character_reputation WHERE guid=?",
		"DELETE FROM character_skills WHERE guid=?",
		"DELETE FROM character_spell WHERE guid=?",
		"DELETE FROM character_spell_cooldown WHERE guid=?",
		"DELETE FROM corpse WHERE player=?",
	} {
		if _, e := tx.ExecContext(ctx, q, guid); e != nil {
			return e
		}
	}
	// Delete pet child tables, then character_pet itself.
	for _, id := range petIDs {
		if _, e := tx.ExecContext(ctx, "DELETE FROM pet_aura WHERE guid=?", id); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, "DELETE FROM pet_spell WHERE guid=?", id); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, "DELETE FROM pet_spell_cooldown WHERE guid=?", id); e != nil {
			return e
		}
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM character_pet WHERE owner=?", guid); err != nil {
		return err
	}
	// Delete item_instance rows owned by this character.
	for _, id := range itemGUIDs {
		if _, e := tx.ExecContext(ctx, "DELETE FROM item_instance WHERE guid=?", id); e != nil {
			return e
		}
	}
	// Finally delete the character row.
	res, err := tx.ExecContext(ctx, "DELETE FROM characters WHERE guid=? AND account=?", guid, accountID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("%w: character row vanished during delete", character.ErrPlayerUnavailable)
	}
	return tx.Commit()
}

func collectItemGUIDs(ctx context.Context, tx *sql.Tx, guid uint32) ([]uint32, error) {
	rows, err := tx.QueryContext(ctx, "SELECT item FROM character_inventory WHERE guid=?", guid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uint32
	for rows.Next() {
		var id uint32
		if e := rows.Scan(&id); e != nil {
			return nil, e
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func collectPetIDs(ctx context.Context, tx *sql.Tx, guid uint32) ([]uint32, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id FROM character_pet WHERE owner=?", guid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uint32
	for rows.Next() {
		var id uint32
		if e := rows.Scan(&id); e != nil {
			return nil, e
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
