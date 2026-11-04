package characters

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"wowserver/internal/domain/character"
)

// Delete removes a character owned by accountID. It performs a hard delete
// across all character-owned tables in one transaction. The character must
// not be online to avoid deleting a snapshot a live session still owns.
// realmcharacters.numchars is decremented inside the same transaction that
// deletes the character row, using the login schema prefix stashed by
// NewCreator. When loginSchema is empty (tests that bypass NewCreator, or
// a Store that was never configured for creation) the counter is left
// untouched, matching the previous behavior.
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
	// Reference guards (HandleCharDeleteOpcode): a guild leader or an arena
	// team captain is refused with a dedicated code before any row is touched.
	var leader bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM guild WHERE leaderguid=?)", guid).Scan(&leader); err != nil {
		return err
	}
	if leader {
		return character.ErrGuildLeader
	}
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM arena_team WHERE captainguid=?)", guid).Scan(&leader); err != nil {
		return err
	}
	if leader {
		return character.ErrArenaCaptain
	}
	// Pet IDs (character_pet.id) are also referenced by pet_aura and pet_spell.
	petIDs, err := collectPetIDs(ctx, tx, guid)
	if err != nil {
		return err
	}
	// Delete child tables. The list follows the reference hard delete
	// (Player::DeleteFromDB case 0 plus the guild/group/arena/petition
	// membership removal that happens in memory there). Some statements
	// reference the guid twice (social both directions, guild eventlog,
	// petition signs); the argument list is built from the placeholder count.
	for _, q := range []string{
		"DELETE FROM arena_team_member WHERE guid=?",
		"DELETE FROM character_action WHERE guid=?",
		"DELETE FROM character_aura WHERE guid=?",
		"DELETE FROM character_battleground_data WHERE guid=?",
		"DELETE FROM character_declinedname WHERE guid=?",
		"DELETE FROM character_gifts WHERE guid=?",
		"DELETE FROM character_homebind WHERE guid=?",
		"DELETE FROM character_instance WHERE guid=?",
		"DELETE FROM character_inventory WHERE guid=?",
		"DELETE FROM character_pet_declinedname WHERE owner=?",
		"DELETE FROM character_queststatus WHERE guid=?",
		"DELETE FROM character_queststatus_daily WHERE guid=?",
		"DELETE FROM character_reputation WHERE guid=?",
		"DELETE FROM character_skills WHERE guid=?",
		"DELETE FROM character_social WHERE guid=? OR friend=?",
		"DELETE FROM character_spell WHERE guid=?",
		"DELETE FROM character_spell_cooldown WHERE guid=?",
		"DELETE FROM character_ticket WHERE guid=?",
		"DELETE FROM corpse WHERE player=?",
		"DELETE FROM group_instance WHERE leaderGuid=?",
		"DELETE FROM group_member WHERE memberGuid=?",
		"DELETE FROM guild_bank_eventlog WHERE PlayerGuid=?",
		"DELETE FROM guild_eventlog WHERE PlayerGuid1=? OR PlayerGuid2=?",
		"DELETE FROM guild_member WHERE guid=?",
		"DELETE FROM mail WHERE receiver=?",
		"DELETE FROM mail_items WHERE receiver=?",
		"DELETE FROM petition WHERE ownerguid=?",
		"DELETE FROM petition_sign WHERE playerguid=? OR ownerguid=?",
	} {
		args := make([]any, strings.Count(q, "?"))
		for i := range args {
			args[i] = guid
		}
		if _, e := tx.ExecContext(ctx, q, args...); e != nil {
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
	// Delete item_instance rows by owner, like the reference. This covers
	// items referenced from mail_items that never appear in
	// character_inventory, not just equipped/carried items.
	if _, err = tx.ExecContext(ctx, "DELETE FROM item_instance WHERE owner_guid=?", guid); err != nil {
		return err
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
	// Decrement the account-wide counter on the same transaction so a crash
	// between the two writes cannot leave them inconsistent. GREATEST guards
	// against underflow if the counter was already out of sync. A missing row
	// (account never created on this realm) is a no-op: UPDATE affects 0 rows.
	if s.loginSchema != "" && s.realmID != 0 {
		if _, e := tx.ExecContext(ctx,
			"UPDATE "+s.loginSchema+"realmcharacters SET numchars=GREATEST(numchars-1,0) WHERE realmid=? AND acctid=?",
			s.realmID, accountID); e != nil {
			return e
		}
	}
	return tx.Commit()
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
