package config

import (
	"io"
	"time"
)

// Common contains only settings shared by both network services.
type Common struct {
	Listen                                   string
	MaxConnections                           int
	AuthTimeout, IdleTimeout, RequestTimeout time.Duration
	Debug                                    bool
	LogLevel                                 string
	LogColor                                 string
	Database                                 Database
	dsnOverride                              string
}

func (c Common) DSN() string {
	if c.dsnOverride != "" {
		return c.dsnOverride
	}
	return c.Database.DSN()
}

type RealmdConfig struct{ Common }
type WorldConfig struct {
	Common
	CharacterDatabase, WorldDatabase Database
	RealmID                          uint32
	SendQueue                        int
	AllowTwoSideAccounts             bool
	GameType                         int
	InstantLogout                    int
	DBCPath                          string
	DBCLocale                        int
	DataLoadTimeout                  time.Duration
}

func (c WorldConfig) CharacterDSN() string { return c.CharacterDatabase.DSN() }
func (c WorldConfig) WorldDSN() string     { return c.WorldDatabase.DSN() }
func common(c Config) Common {
	return Common{Listen: c.Listen, MaxConnections: c.MaxConnections, AuthTimeout: c.AuthTimeout, IdleTimeout: c.IdleTimeout, RequestTimeout: c.RequestTimeout, Debug: c.Debug, LogLevel: c.LogLevel, LogColor: c.LogColor, Database: c.Database, dsnOverride: c.dsnOverride}
}
func ParseRealmd(args []string, out io.Writer) (RealmdConfig, error) {
	c, e := parse(args, out, "realmd", "configs/realmd.conf", "127.0.0.1:3724")
	return RealmdConfig{Common: common(c)}, e
}
func ParseWorld(args []string, out io.Writer) (WorldConfig, error) {
	c, e := parse(args, out, "worldd", "configs/worldd.conf", "127.0.0.1:8085")
	return WorldConfig{Common: common(c), CharacterDatabase: c.CharacterDatabase, WorldDatabase: c.WorldDatabase, RealmID: c.RealmID, SendQueue: c.SendQueue, AllowTwoSideAccounts: c.AllowTwoSideAccounts, GameType: c.GameType, InstantLogout: c.InstantLogout, DBCPath: c.DBCPath, DBCLocale: c.DBCLocale, DataLoadTimeout: c.DataLoadTimeout}, e
}
