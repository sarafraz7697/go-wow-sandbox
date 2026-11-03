package realmd

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"strings"
	"testing"
	"time"
	"wowserver/internal/account"
	"wowserver/internal/protocol/auth/tbc"
	"wowserver/internal/testclient"
	"wowserver/internal/transport"
)

// Optional read-only compatibility check against existing MaNGOS accounts.
// Secret values stay in process memory; no account is authenticated or changed.
func TestExistingDatabaseReadOnly(t *testing.T) {
	dsn := os.Getenv("WOW_EXISTING_DSN")
	if dsn == "" {
		t.Skip("set WOW_EXISTING_DSN for read-only verifier compatibility check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	if err = s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT sha_pass_hash,s,v FROM account WHERE LENGTH(sha_pass_hash)=40 AND LENGTH(s)=64 AND LENGTH(v)=64")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	modulus, _ := new(big.Int).SetString("894B645E89E1535BBDAD5B8B290650530801B18EBFBF5E8FAB3C82872A3E9BB7", 16)
	count := 0
	for rows.Next() {
		var hash, salt, v string
		if err = rows.Scan(&hash, &salt, &v); err != nil {
			t.Fatal(err)
		}
		digest, e := hex.DecodeString(hash)
		if e != nil {
			t.Fatal("invalid existing credential hash")
		}
		sb, e := decodeNumber(salt, 32)
		if e != nil {
			t.Fatal(e)
		}
		vb, e := decodeNumber(v, 32)
		if e != nil {
			t.Fatal(e)
		}
		sum := sha1.Sum(append(sb, digest...))
		xb := append([]byte(nil), sum[:]...)
		for i, j := 0, len(xb)-1; i < j; i, j = i+1, j-1 {
			xb[i], xb[j] = xb[j], xb[i]
		}
		result := new(big.Int).Exp(big.NewInt(7), new(big.Int).SetBytes(xb), modulus)
		expected := make([]byte, 32)
		result.FillBytes(expected)
		for i, j := 0, len(expected)-1; i < j; i, j = i+1, j-1 {
			expected[i], expected[j] = expected[j], expected[i]
		}
		if !bytes.Equal(expected, vb) {
			t.Fatal("existing verifier does not match stored hash and salt")
		}
		count++
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no existing verifier available to validate")
	}
	t.Logf("verified %d existing account encodings without writes", count)
}

func TestHexWidths(t *testing.T) {
	for _, width := range []int{32, 40} {
		b := make([]byte, width)
		b[0] = 0x12
		b[1] = 0xab
		value := encodeNumber(b)
		if len(value) != width*2 || !strings.HasSuffix(value, "AB12") {
			t.Fatal("big-number hex encoding")
		}
		for _, s := range []string{value, strings.TrimLeft(value, "0"), strings.ToLower(value)} {
			got, err := decodeNumber(s, width)
			if err != nil || !bytes.Equal(got, b) {
				t.Fatal("decode padding", err)
			}
		}
	}
	for _, bad := range []string{"", "-1", "GG", strings.Repeat("A", 65)} {
		if _, err := decodeNumber(bad, 32); err == nil {
			t.Fatal("invalid integer accepted")
		}
	}
}
func TestRealmRouting(t *testing.T) {
	if !supportsBuild("8606 12340 ", 8606) || supportsBuild("18606", 8606) || supportsBuild("", 8606) {
		t.Fatal("build parsing")
	}
	for _, c := range []struct{ external, local, client, want string }{
		{"127.0.0.1", "192.168.1.2", "127.0.0.1", "127.0.0.1"},
		{"203.0.113.1", "192.168.1.2", "127.0.0.1", "192.168.1.2"},
		{"203.0.113.1", "192.168.1.2", "192.168.1.55", "192.168.1.2"},
		{"203.0.113.1", "192.168.1.2", "198.51.100.2", "203.0.113.1"},
	} {
		if got := addressForClient(c.external, c.local, "255.255.255.0", c.client); got != c.want {
			t.Fatal(got)
		}
	}
}

func TestImportedMySQLSchema(t *testing.T) {
	dsn := os.Getenv("WOW_TEST_DSN")
	if dsn == "" {
		t.Skip("set WOW_TEST_DSN to the isolated schema-import sandbox")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	if err = s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("T%015X", time.Now().UnixNano()&0xfffffffffffffff)
	if err = s.Create(ctx, name, "PASSWORD"); err != nil {
		t.Fatal(err)
	}
	a, err := s.Find(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.ExecContext(context.Background(), "DELETE FROM account WHERE id=?", a.ID)
	if err = s.Create(ctx, name, "PASSWORD"); !errors.Is(err, account.ErrConflict) {
		t.Fatal("duplicate", err)
	}
	var hash, salt, v string
	if err = s.DB.QueryRowContext(ctx, "SELECT sha_pass_hash,s,v FROM account WHERE id=?", a.ID).Scan(&hash, &salt, &v); err != nil {
		t.Fatal(err)
	}
	digest := sha1.Sum([]byte(name + ":PASSWORD"))
	if hash != strings.ToUpper(hex.EncodeToString(digest[:])) || salt != encodeNumber(a.Salt[:]) || v != encodeNumber(a.Verifier[:]) {
		t.Fatal("credential format")
	}
	if err = s.AddRealm(ctx, name, "203.0.113.1", 8085); err != nil {
		t.Fatal(err)
	}
	var rid uint32
	if err = s.DB.QueryRowContext(ctx, "SELECT id FROM realmlist WHERE name=?", name).Scan(&rid); err != nil {
		t.Fatal(err)
	}
	defer s.DB.ExecContext(context.Background(), "DELETE FROM realmlist WHERE id=?", rid)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.DB.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("UPDATE realmlist SET localAddress='192.168.1.2',allowedSecurityLevel=2,realmbuilds='8606 12340 ' WHERE id=?", rid)
	exec("INSERT INTO realmcharacters (realmid,acctid,numchars) VALUES (?,?,3)", rid, a.ID)
	list, err := s.ListForClient(ctx, a.ID, 8606, "192.168.1.9")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range list {
		if r.ID == rid {
			found = true
			if !r.Locked || r.Characters != 3 || r.Host != "192.168.1.2" {
				t.Fatal("realm mapping")
			}
		}
	}
	if !found {
		t.Fatal("realm missing")
	}
	exec("UPDATE account SET gmlevel=3 WHERE id=?", a.ID)
	list, err = s.List(ctx, a.ID, 8606)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range list {
		if r.ID == rid && r.Locked {
			t.Fatal("GM lock")
		}
	}
	exec("UPDATE realmlist SET realmflags=1 WHERE id=?", rid)
	list, err = s.List(ctx, a.ID, 8606)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range list {
		if r.ID == rid {
			t.Fatal("hidden realm leaked")
		}
	}
	exec("UPDATE realmlist SET realmflags=0 WHERE id=?", rid)

	// World authentication only reads the native login session. These writes
	// affect this test's disposable account and realm in the sandbox only.
	var worldKey [40]byte
	for i := range worldKey {
		worldKey[i] = byte(i + 1)
	}
	if err = s.Publish(ctx, a, worldKey, 8606, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if err = s.CheckWorldRealm(ctx, rid); err != nil {
		t.Fatal(err)
	}
	worldSession, err := s.WorldSession(ctx, name, "127.0.0.1", rid)
	if err != nil || worldSession.ID != a.ID || worldSession.Key != worldKey || worldSession.Expansion != 1 {
		t.Fatal("world session lookup", err)
	}
	exec("UPDATE account SET gmlevel=0 WHERE id=?", a.ID)
	if _, err = s.WorldSession(ctx, name, "127.0.0.1", rid); err == nil {
		t.Fatal("world realm security bypass")
	}
	exec("UPDATE account SET gmlevel=3,locked=1 WHERE id=?", a.ID)
	if _, err = s.WorldSession(ctx, name, "192.0.2.99", rid); err == nil {
		t.Fatal("world IP lock bypass")
	}
	exec("UPDATE account SET locked=0 WHERE id=?", a.ID)
	exec("UPDATE realmlist SET realmbuilds='12340' WHERE id=?", rid)
	if _, err = s.WorldSession(ctx, name, "127.0.0.1", rid); err == nil {
		t.Fatal("world build bypass")
	}
	exec("UPDATE realmlist SET realmbuilds='8606 12340 ' WHERE id=?", rid)

	// Real TCP login and realm requests use MaNGOS-formatted storage.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serverCtx, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan error, 1)
	handler := tbc.Handler{Store: s, Realms: s, Sessions: s, OnAuthenticated: transport.CompleteHandshake}
	server := transport.Server{MaxConnections: 4, SessionTimeout: time.Second, Handle: handler.Serve}
	go func() { done <- server.Serve(serverCtx, listener) }()
	defer func() { stop(); <-done }()
	dial := func() net.Conn {
		t.Helper()
		c, e := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
		if e != nil {
			t.Fatal(e)
		}
		_ = c.SetDeadline(time.Now().Add(3 * time.Second))
		return c
	}
	read := func(c net.Conn, n int) []byte {
		t.Helper()
		b := make([]byte, n)
		if _, e := io.ReadFull(c, b); e != nil {
			t.Fatal(e)
		}
		return b
	}
	write := func(c net.Conn, b []byte) {
		t.Helper()
		if _, e := c.Write(b); e != nil {
			t.Fatal(e)
		}
	}
	c := dial()
	defer c.Close()
	write(c, testclient.Challenge(name, 8606))
	challenge := read(c, 119)
	proof, key, m2 := testclient.Proof(name, "PASSWORD", challenge)
	write(c, proof)
	response := read(c, 32)
	if !bytes.Equal(response[2:22], m2[:]) {
		t.Fatal("server proof")
	}
	for range 2 {
		write(c, []byte{16, 0, 0, 0, 0})
		h := read(c, 3)
		body := read(c, int(binary.LittleEndian.Uint16(h[1:])))
		if !bytes.Contains(body, []byte(name)) {
			t.Fatal("realm response")
		}
	}
	c.Close()
	var saved string
	if err = s.DB.QueryRowContext(ctx, "SELECT sessionkey FROM account WHERE id=?", a.ID).Scan(&saved); err != nil {
		t.Fatal(err)
	}
	if saved != encodeNumber(key[:]) {
		t.Fatal("session key format")
	}
	c = dial()
	defer c.Close()
	req := testclient.Challenge(name, 8606)
	req[0] = 2
	write(c, req)
	recon := read(c, 34)
	rp := make([]byte, 58)
	rp[0] = 3
	rp[1] = 99
	h := sha1.New()
	h.Write([]byte(name))
	h.Write(rp[1:17])
	h.Write(recon[2:18])
	h.Write(key[:])
	copy(rp[17:37], h.Sum(nil))
	write(c, rp)
	if !bytes.Equal(read(c, 4), []byte{3, 0, 0, 0}) {
		t.Fatal("reconnect")
	}
	c.Close()

	// Restrictions use native ban tables and locked + last_ip.
	exec("INSERT INTO account_banned (id,bandate,unbandate,bannedby,banreason,active) VALUES (?,100,100,'test','test',1)", a.ID)
	banned, e := s.Find(ctx, name)
	if e != nil || !banned.Banned {
		t.Fatal("permanent ban", e)
	}
	if _, e = s.Session(ctx, a.ID, 8606, "127.0.0.1"); e == nil {
		t.Fatal("banned session accepted")
	}
	exec("UPDATE account_banned SET unbandate=UNIX_TIMESTAMP()+60 WHERE id=?", a.ID)
	banned, e = s.Find(ctx, name)
	if e != nil || !banned.Suspended {
		t.Fatal("temporary ban", e)
	}
	exec("UPDATE account_banned SET unbandate=101 WHERE id=?", a.ID)
	banned, e = s.Find(ctx, name)
	if e != nil || banned.Banned || banned.Suspended {
		t.Fatal("expired ban", e)
	}
	exec("UPDATE account SET locked=1,last_ip='192.0.2.42' WHERE id=?", a.ID)
	if _, e = s.Session(ctx, a.ID, 8606, "127.0.0.1"); e == nil {
		t.Fatal("IP lock ignored")
	}
	exec("UPDATE account SET locked=0 WHERE id=?", a.ID)
	testIP := "192.0.2.199"
	stamp := time.Now().UnixNano() / 1000000
	exec("INSERT INTO ip_banned (ip,bandate,unbandate) VALUES (?,?,?)", testIP, stamp, stamp)
	defer s.DB.ExecContext(context.Background(), "DELETE FROM ip_banned WHERE ip=? AND bandate=?", testIP, stamp)
	if ok, e := s.IPAllowed(ctx, testIP); e != nil || ok {
		t.Fatal("IP ban ignored", e)
	}
	if e = s.Publish(ctx, a, key, 8606, testIP); e == nil {
		t.Fatal("IP ban publish race")
	}
	exec("UPDATE ip_banned SET unbandate=1 WHERE ip=? AND bandate=?", testIP, stamp)
	if ok, e := s.IPAllowed(ctx, testIP); e != nil || !ok {
		t.Fatal("expired IP ban", e)
	}
	if err = s.Failed(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	exec("UPDATE account SET sha_pass_hash=REPEAT('0',40) WHERE id=?", a.ID)
	if err = s.Publish(ctx, a, key, 8606, "127.0.0.1"); !errors.Is(err, account.ErrChanged) {
		t.Fatal("password change race", err)
	}
	// Old accounts can have only sha_pass_hash; initialize s/v exactly once.
	exec("UPDATE account SET sha_pass_hash=?,s='0',v='0' WHERE id=?", hash, a.ID)
	restored, e := s.Find(ctx, name)
	if e != nil || restored.Verifier == [32]byte{} {
		t.Fatal("legacy hash initialization", e)
	}
	again, e := s.Find(ctx, name)
	if e != nil || again.Salt != restored.Salt {
		t.Fatal("salt regenerated", e)
	}
}
