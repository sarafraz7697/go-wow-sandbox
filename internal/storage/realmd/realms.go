package realmd

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"wowserver/internal/realm"
)

func supportsBuild(value string, build uint16) bool {
	for _, s := range strings.Fields(value) {
		n, err := strconv.ParseUint(s, 10, 16)
		if err == nil && uint16(n) == build {
			return true
		}
	}
	return false
}
func (s *Store) List(ctx context.Context, id uint64, build uint16) ([]realm.Realm, error) {
	return s.ListForClient(ctx, id, build, "")
}
func (s *Store) ListForClient(ctx context.Context, id uint64, build uint16, ip string) ([]realm.Realm, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT r.id,r.name,r.address,r.localAddress,r.localSubnetMask,r.port,r.icon,r.realmflags,r.population,r.timezone,r.allowedSecurityLevel,r.realmbuilds,COALESCE(c.numchars,0),COALESCE(a.gmlevel,0)
 FROM realmlist r LEFT JOIN realmcharacters c ON c.realmid=r.id AND c.acctid=? LEFT JOIN account a ON a.id=? WHERE (r.realmflags & 1)=0 ORDER BY r.name LIMIT 1025`, id, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []realm.Realm{}
	for rows.Next() {
		var r realm.Realm
		var local, mask, builds string
		var required, security uint8
		if err = rows.Scan(&r.ID, &r.Name, &r.Host, &local, &mask, &r.Port, &r.Type, &r.Flags, &r.Population, &r.Category, &required, &builds, &r.Characters, &security); err != nil {
			return nil, err
		}
		if !supportsBuild(builds, build) {
			continue
		}
		r.Build = build
		if required > 3 {
			required = 3
		}
		if security > 3 {
			security = 3
		}
		r.Locked = required > security
		r.Flags &= 0x02 | 0x04 | 0x20 | 0x40
		if ip != "" {
			r.Host = addressForClient(r.Host, local, mask, ip)
		}
		list = append(list, r)
	}
	return list, rows.Err()
}
func addressForClient(external, local, mask, client string) string {
	ip := net.ParseIP(client)
	if ip == nil {
		return external
	}
	extIP, localIP := net.ParseIP(external), net.ParseIP(local)
	if ip.IsLoopback() {
		if (extIP != nil && extIP.IsLoopback()) || (localIP != nil && localIP.IsLoopback()) {
			return ip.String()
		}
		return local
	}
	m := net.ParseIP(mask).To4()
	if ip.To4() != nil && localIP.To4() != nil && m != nil {
		network := net.IPNet{IP: localIP.To4().Mask(net.IPMask(m)), Mask: net.IPMask(m)}
		if network.Contains(ip) {
			return local
		}
	}
	return external
}
func (s *Store) AddRealm(ctx context.Context, name, host string, port uint16) error {
	if len(name) > 32 {
		return errors.New("realm name exceeds schema limit")
	}
	_, err := s.DB.ExecContext(ctx, "INSERT INTO realmlist (name,address,localAddress,port,realmflags,timezone,realmbuilds) VALUES (?,?,?,?,0,1,'8606')", name, host, host, port)
	return err
}
