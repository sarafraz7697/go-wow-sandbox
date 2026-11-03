package realmd

import (
	"context"
	"errors"
	"strings"
	"wowserver/internal/account"
)

// WorldSession is read-only: worldd does not initialize SRP credentials or
// replace session keys. Login publication remains owned by realmd.
func (s *Store) WorldSession(ctx context.Context, name, ip string, realmID uint32) (account.WorldSession, error) {
	var a account.WorldSession
	var key, builds string
	err := s.DB.QueryRowContext(ctx, `SELECT account.id,username,COALESCE(sessionkey,''),LEAST(expansion,1),r.realmbuilds
 FROM account JOIN realmlist r ON r.id=? WHERE username=? AND (locked=0 OR last_ip=?)
 AND LEAST(gmlevel,3)>=LEAST(r.allowedSecurityLevel,3)`+accessSQL, realmID, name, ip, ip).Scan(&a.ID, &a.Name, &key, &a.Expansion, &builds)
	if err != nil {
		return a, err
	}
	if !containsBuild8606(builds) {
		return a, account.ErrChanged
	}
	b, err := decodeNumber(key, 40)
	if err != nil {
		return a, err
	}
	copy(a.Key[:], b)
	if a.Key == ([40]byte{}) {
		return a, account.ErrChanged
	}
	return a, nil
}
func containsBuild8606(builds string) bool {
	for _, b := range strings.Fields(builds) {
		if b == "8606" {
			return true
		}
	}
	return false
}
func (s *Store) CheckWorldRealm(ctx context.Context, id uint32) error {
	var builds string
	if err := s.DB.QueryRowContext(ctx, "SELECT realmbuilds FROM realmlist WHERE id=?", id).Scan(&builds); err != nil {
		return err
	}
	if !containsBuild8606(builds) {
		return errors.New("realm does not support build 8606")
	}
	return nil
}
