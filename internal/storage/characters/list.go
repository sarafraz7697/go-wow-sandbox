package characters

import (
	"context"
	"errors"
	"wowserver/internal/domain/character"
)

// Realm isolation is by configured CharacterDSN, as in MaNGOS (there is no
// realmid on characters). Account identity comes only from the verified session.
// A deterministic pet subquery avoids duplicate characters if old data contains
// multiple pets in current slot 0. Deleted characters are never exposed.
func (s *Store) List(ctx context.Context, accountID uint64) ([]character.Summary, error) {
	if accountID == 0 {
		return nil, errors.New("invalid character account")
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT c.guid,c.name,c.race,c.class,c.gender,c.level,c.playerBytes,c.playerBytes2,
 c.zone,c.map,c.position_x,c.position_y,c.position_z,COALESCE(g.guildid,0),c.playerFlags,c.at_login,
 COALESCE(p.entry,0),COALESCE(p.modelid,0),COALESCE(p.level,0),LEFT(COALESCE(c.equipmentCache,''),4097)
 FROM characters c
 LEFT JOIN guild_member g ON g.guid=c.guid
 LEFT JOIN character_pet p ON p.id=(SELECT MIN(p2.id) FROM character_pet p2 WHERE p2.owner=c.guid AND p2.slot=0)
 WHERE c.account=? AND (c.deleteDate IS NULL OR c.deleteDate=0)
 ORDER BY c.guid LIMIT 256`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []character.Summary
	for rows.Next() {
		var v character.Summary
		if err = rows.Scan(&v.GUID, &v.Name, &v.Race, &v.Class, &v.Gender, &v.Level, &v.PlayerBytes, &v.PlayerBytes2, &v.Zone, &v.Map, &v.X, &v.Y, &v.Z, &v.Guild, &v.PlayerFlags, &v.AtLogin, &v.PetEntry, &v.PetDisplay, &v.PetLevel, &v.EquipmentCache); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(result) > 255 {
		return nil, errors.New("character count exceeds TBC packet capacity")
	}
	return result, nil
}
