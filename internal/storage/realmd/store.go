package realmd

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"errors"
	driver "github.com/go-sql-driver/mysql"
	"strings"
	"wowserver/internal/account"
	"wowserver/internal/auth/srp"
	"wowserver/internal/storage/sqlutil"
)

type Store struct{ DB *sql.DB }

func Open(ctx context.Context, dsn string) (*Store, error) {
	db, err := sqlutil.Open(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &Store{db}, nil
}
func (s *Store) Check(ctx context.Context) error {
	return sqlutil.Check(ctx, s.DB, map[string]string{
		"account":         "SELECT id,username,sha_pass_hash,gmlevel,sessionkey,v,s,joindate,last_ip,failed_logins,locked,last_login,expansion,locale,client_locale,os FROM account",
		"account_banned":  "SELECT id,bandate,unbandate,active FROM account_banned",
		"ip_banned":       "SELECT ip,bandate,unbandate FROM ip_banned",
		"realmlist":       "SELECT id,name,address,localAddress,localSubnetMask,port,icon,realmflags,timezone,allowedSecurityLevel,population,realmbuilds FROM realmlist",
		"realmcharacters": "SELECT realmid,acctid,numchars FROM realmcharacters",
	})
}
func (s *Store) Create(ctx context.Context, name, password string) error {
	var err error
	name, err = account.Normalize(name)
	if err != nil {
		return err
	}
	password, err = account.Normalize(password)
	if err != nil {
		return err
	}
	salt, v, err := srp.Credentials(name, password)
	if err != nil {
		return err
	}
	digest := sha1.Sum([]byte(name + ":" + password))
	_, err = s.DB.ExecContext(ctx, "INSERT INTO account (username,sha_pass_hash,s,v,joindate,expansion) VALUES (?,?,?,?,UTC_TIMESTAMP(),1)", name, strings.ToUpper(hex.EncodeToString(digest[:])), encodeNumber(salt[:]), encodeNumber(v[:]))
	var e *driver.MySQLError
	if errors.As(err, &e) && e.Number == 1062 {
		return account.ErrConflict
	}
	return err
}
func (s *Store) Find(ctx context.Context, name string) (account.Account, error) {
	var a account.Account
	var salt, v string
	var locked bool
	var lastIP string
	err := s.DB.QueryRowContext(ctx, `SELECT id,username,COALESCE(s,''),COALESCE(v,''),sha_pass_hash,gmlevel,locked,last_ip,
 EXISTS(SELECT 1 FROM account_banned b WHERE b.id=account.id AND b.active=1 AND b.unbandate=b.bandate),
 EXISTS(SELECT 1 FROM account_banned b WHERE b.id=account.id AND b.active=1 AND b.unbandate<>b.bandate AND b.unbandate>UNIX_TIMESTAMP())
 FROM account WHERE username=?`, name).Scan(&a.ID, &a.Name, &salt, &v, &a.CredentialHash, &a.Security, &locked, &lastIP, &a.Banned, &a.Suspended)
	if errors.Is(err, sql.ErrNoRows) {
		return a, account.ErrNotFound
	}
	if err != nil {
		return a, err
	}
	if locked {
		a.LockedIP = lastIP
	}
	if a.Banned || a.Suspended {
		return a, nil
	}
	// MaNGOS accounts may use the literal "0" until first login. The pinned
	// realmd regenerates credentials whenever either hex field is not 64 chars.
	if len(salt) != 64 || len(v) != 64 {
		hash, err := hex.DecodeString(a.CredentialHash)
		if err != nil || len(hash) != 20 {
			return a, errors.New("account has no valid password verifier or hash")
		}
		generatedSalt, generatedV, err := srp.CredentialsFromHash([20]byte(hash))
		if err != nil {
			return a, err
		}
		_, err = s.DB.ExecContext(ctx, "UPDATE account SET s=?,v=? WHERE id=? AND sha_pass_hash=? AND COALESCE(s,'')=? AND COALESCE(v,'')=?", encodeNumber(generatedSalt[:]), encodeNumber(generatedV[:]), a.ID, a.CredentialHash, salt, v)
		if err != nil {
			return a, err
		}
		// Re-read after a competing login may have initialized a different salt.
		if err = s.DB.QueryRowContext(ctx, "SELECT s,v FROM account WHERE id=? AND sha_pass_hash=?", a.ID, a.CredentialHash).Scan(&salt, &v); err != nil {
			return a, err
		}
	}
	sb, err := decodeNumber(salt, 32)
	if err != nil {
		return a, err
	}
	vb, err := decodeNumber(v, 32)
	if err != nil {
		return a, err
	}
	copy(a.Salt[:], sb)
	copy(a.Verifier[:], vb)
	return a, nil
}
func (s *Store) IPAllowed(ctx context.Context, ip string) (bool, error) {
	var banned bool
	err := s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM ip_banned WHERE ip=? AND (unbandate=bandate OR unbandate>UNIX_TIMESTAMP()))", ip).Scan(&banned)
	return !banned, err
}

const accessSQL = ` AND NOT EXISTS(SELECT 1 FROM account_banned b WHERE b.id=account.id AND b.active=1 AND (b.unbandate=b.bandate OR b.unbandate>UNIX_TIMESTAMP())) AND NOT EXISTS(SELECT 1 FROM ip_banned i WHERE i.ip=? AND (i.unbandate=i.bandate OR i.unbandate>UNIX_TIMESTAMP()))`

func (s *Store) Publish(ctx context.Context, a account.Account, key [40]byte, build uint16, ip string) error {
	if build != 8606 {
		return account.ErrChanged
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE account SET sessionkey=?,last_ip=?,last_login=UTC_TIMESTAMP(),failed_logins=0
 WHERE id=? AND sha_pass_hash=? AND UPPER(LPAD(s,64,'0'))=? AND UPPER(LPAD(v,64,'0'))=? AND (locked=0 OR last_ip=?)`+accessSQL,
		encodeNumber(key[:]), ip, a.ID, a.CredentialHash, encodeNumber(a.Salt[:]), encodeNumber(a.Verifier[:]), ip, ip)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return account.ErrChanged
	}
	return nil
}
func (s *Store) Session(ctx context.Context, id uint64, build uint16, ip string) ([40]byte, error) {
	var key [40]byte
	if build != 8606 {
		return key, account.ErrChanged
	}
	var value string
	err := s.DB.QueryRowContext(ctx, "SELECT COALESCE(sessionkey,'') FROM account WHERE id=? AND (locked=0 OR last_ip=?)"+accessSQL, id, ip, ip).Scan(&value)
	if err != nil {
		return key, err
	}
	b, err := decodeNumber(value, 40)
	if err != nil {
		return key, err
	}
	copy(key[:], b)
	if key == [40]byte{} {
		return key, account.ErrChanged
	}
	return key, nil
}
func (s *Store) Failed(ctx context.Context, id uint64) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE account SET failed_logins=LEAST(failed_logins+1,4294967295) WHERE id=?", id)
	return err
}
