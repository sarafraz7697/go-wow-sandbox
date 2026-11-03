package config

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

type Config struct {
	Listen            string
	MaxConnections    int
	AuthTimeout       time.Duration
	IdleTimeout       time.Duration
	RequestTimeout    time.Duration
	Debug             bool
	LogLevel          string
	Database          Database
	CharacterDatabase Database
	WorldDatabase     Database
	RealmID           uint32
	SendQueue         int
	DBCPath           string
	DBCLocale         int
	DataLoadTimeout   time.Duration
	dsnOverride       string
}

type Database struct {
	Host                 string
	Port                 int
	Name, User, Password string
}

// DSN uses the driver's formatter rather than concatenating credentials.
func (c Config) DSN() string {
	if c.dsnOverride != "" {
		return c.dsnOverride
	}
	return c.Database.DSN()
}
func (c Config) CharacterDSN() string { return c.CharacterDatabase.DSN() }
func (c Config) WorldDSN() string     { return c.WorldDatabase.DSN() }
func (database Database) DSN() string {
	d := mysql.NewConfig()
	d.Net = "tcp"
	d.Addr = net.JoinHostPort(database.Host, strconv.Itoa(database.Port))
	d.DBName, d.User, d.Passwd = database.Name, database.User, database.Password
	return d.FormatDSN()
}

func Parse(args []string, output io.Writer) (Config, error) {
	return parse(args, output, "realmd", "configs/realmd.conf", "127.0.0.1:3724")
}

func parse(args []string, output io.Writer, service, defaultPath, defaultListen string) (Config, error) {
	var c Config
	c.RealmID, c.SendQueue = 1, 64
	c.DBCPath, c.DataLoadTimeout = "dbc", 60*time.Second
	var path string
	f := flag.NewFlagSet(service, flag.ContinueOnError)
	f.SetOutput(output)
	f.StringVar(&path, "config", defaultPath, "configuration file (relative to working directory)")
	f.StringVar(&c.Listen, "listen", defaultListen, "TCP listen address")
	f.IntVar(&c.MaxConnections, "max-connections", 64, "maximum simultaneous connections")
	f.DurationVar(&c.AuthTimeout, "auth-timeout", 10*time.Second, "absolute connection time budget")
	f.DurationVar(&c.IdleTimeout, "idle-timeout", 2*time.Minute, "authenticated idle timeout")
	f.DurationVar(&c.RequestTimeout, "request-timeout", 10*time.Second, "request and write time budget")
	f.BoolVar(&c.Debug, "debug", false, "enable diagnostic logs (no packet payloads)")
	f.StringVar(&c.LogLevel, "log-level", "info", "debug, info, warn or error")
	if err := f.Parse(args); err != nil {
		return c, err
	}
	if f.NArg() != 0 {
		return c, fmt.Errorf("unexpected positional arguments")
	}
	visited := map[string]bool{}
	f.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	cli := c
	c.Database = Database{Host: "127.0.0.1", Port: 3306, Name: "realmd", User: "root"}
	c.CharacterDatabase = Database{Host: "127.0.0.1", Port: 3306, Name: "character1", User: "root"}
	c.WorldDatabase = Database{Host: "127.0.0.1", Port: 3306, Name: "mangos1", User: "root"}
	values, err := readFile(path)
	if err != nil && !(errors.Is(err, os.ErrNotExist) && !visited["config"]) {
		return c, err
	}
	for key, value := range values {
		if err := c.set(key, value); err != nil {
			return c, err
		}
	}
	if err := c.applyEnvironment(); err != nil {
		return c, err
	}
	c.applyCLI(cli, visited)
	err = c.validate(service == "worldd")
	return c, err
}
func (c *Config) applyEnvironment() error {
	envKeys := map[string]string{
		"WOW_LISTEN": "network.listen", "WOW_MAX_CONNECTIONS": "network.max_connections",
		"WOW_AUTH_TIMEOUT": "network.auth_timeout", "WOW_LOG_LEVEL": "logging.level",
		"WOW_IDLE_TIMEOUT": "network.idle_timeout", "WOW_REQUEST_TIMEOUT": "network.request_timeout",
		"WOW_DB_HOST": "database.host", "WOW_DB_PORT": "database.port", "WOW_DB_NAME": "database.name",
		"WOW_DB_USER": "database.user", "WOW_DB_PASSWORD": "database.password",
	}
	for env, key := range envKeys {
		if value, ok := os.LookupEnv(env); ok {
			if err := c.set(key, value); err != nil {
				return err
			}
		}
	}
	c.dsnOverride = os.Getenv("WOW_AUTH_DSN")
	return nil
}
func (c *Config) applyCLI(cli Config, visited map[string]bool) {
	if visited["listen"] {
		c.Listen = cli.Listen
	}
	if visited["max-connections"] {
		c.MaxConnections = cli.MaxConnections
	}
	if visited["auth-timeout"] {
		c.AuthTimeout = cli.AuthTimeout
	}
	if visited["idle-timeout"] {
		c.IdleTimeout = cli.IdleTimeout
	}
	if visited["request-timeout"] {
		c.RequestTimeout = cli.RequestTimeout
	}
	if visited["log-level"] {
		c.LogLevel = cli.LogLevel
	}
	if visited["debug"] {
		if cli.Debug {
			c.LogLevel = "debug"
		} else {
			c.LogLevel = "info"
		}
	}
}
func (c *Config) validate(world bool) error {
	if world && (c.DBCPath == "" || c.DBCLocale < 0 || c.DBCLocale > 15 || c.DataLoadTimeout <= 0) {
		return fmt.Errorf("invalid data dbc_path, dbc_locale or load_timeout")
	}
	c.Debug = c.LogLevel == "debug"
	if c.LogLevel != "debug" && c.LogLevel != "info" && c.LogLevel != "warn" && c.LogLevel != "error" {
		return fmt.Errorf("invalid logging.level")
	}
	if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	if c.MaxConnections < 1 || c.AuthTimeout <= 0 || c.IdleTimeout <= 0 || c.RequestTimeout <= 0 {
		return fmt.Errorf("connection limit and timeout must be positive")
	}
	if world && (c.RealmID == 0 || c.SendQueue < 1 || c.SendQueue > 1024) {
		return fmt.Errorf("invalid world realm_id or send_queue (1..1024)")
	}
	databases := []Database{c.Database}
	if world {
		databases = append(databases, c.CharacterDatabase, c.WorldDatabase)
	}
	for _, d := range databases {
		if d.Host == "" || d.Port < 1 || d.Port > 65535 || d.Name == "" || d.User == "" {
			return fmt.Errorf("invalid database settings")
		}
	}
	return nil
}

func (c *Config) set(key, value string) error {
	var err error
	for _, entry := range []struct {
		prefix string
		db     *Database
	}{{"database.login.", &c.Database}, {"database.characters.", &c.CharacterDatabase}, {"database.world.", &c.WorldDatabase}} {
		if strings.HasPrefix(key, entry.prefix) {
			return setDatabase(entry.db, strings.TrimPrefix(key, entry.prefix), value)
		}
	}
	switch key {
	case "data.dbc_path":
		c.DBCPath = value
	case "data.dbc_locale":
		c.DBCLocale, err = strconv.Atoi(value)
	case "data.load_timeout":
		c.DataLoadTimeout, err = time.ParseDuration(value)
	case "world.realm_id":
		var valueID uint64
		valueID, err = strconv.ParseUint(value, 10, 32)
		c.RealmID = uint32(valueID)
	case "world.send_queue":
		c.SendQueue, err = strconv.Atoi(value)
	case "network.listen":
		c.Listen = value
	case "network.max_connections":
		c.MaxConnections, err = strconv.Atoi(value)
	case "network.auth_timeout":
		c.AuthTimeout, err = time.ParseDuration(value)
	case "network.idle_timeout":
		c.IdleTimeout, err = time.ParseDuration(value)
	case "network.request_timeout":
		c.RequestTimeout, err = time.ParseDuration(value)
	case "logging.level":
		c.LogLevel = value
	case "database.host":
		c.Database.Host = value
	case "database.port":
		c.Database.Port, err = strconv.Atoi(value)
	case "database.name":
		c.Database.Name = value
	case "database.user":
		c.Database.User = value
	case "database.password":
		c.Database.Password = value
	default:
		return fmt.Errorf("unknown configuration key %s", key)
	}
	if err != nil {
		return fmt.Errorf("invalid configuration value for %s", key)
	}
	return nil
}

func setDatabase(d *Database, key, value string) error {
	switch key {
	case "host":
		d.Host = value
	case "port":
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("invalid database port")
		}
		d.Port = n
	case "name":
		d.Name = value
	case "user":
		d.User = value
	case "password":
		d.Password = value
	default:
		return fmt.Errorf("unknown database key")
	}
	return nil
}

// Small strict INI subset: full-line comments, sections, key=value and optional
// single/double quotes. No interpolation or inline comments (passwords may use #).
// Parse the entire file before applying it. Never include secret values in errors.
func readFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	values := map[string]string{}
	section := ""
	line := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line++
		s := strings.TrimSpace(scanner.Text())
		if line == 1 {
			s = strings.TrimPrefix(s, "\ufeff")
		}
		if s == "" || strings.HasPrefix(s, "#") || strings.HasPrefix(s, ";") {
			continue
		}
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
			section = strings.TrimSpace(s[1 : len(s)-1])
			if section != "network" && section != "world" && section != "data" && section != "database" && section != "logging" && section != "database.login" && section != "database.characters" && section != "database.world" {
				return nil, fmt.Errorf("invalid config section at line %d", line)
			}
			continue
		}
		k, v, ok := strings.Cut(s, "=")
		if !ok || section == "" {
			return nil, fmt.Errorf("invalid config syntax at line %d", line)
		}
		key := section + "." + strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, "\"") {
			v, err = strconv.Unquote(v)
			if err != nil {
				return nil, fmt.Errorf("invalid quoted value at line %d", line)
			}
		} else if strings.HasPrefix(v, "'") {
			if len(v) < 2 || v[len(v)-1] != '\'' {
				return nil, fmt.Errorf("invalid quoted value at line %d", line)
			}
			v = v[1 : len(v)-1]
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("duplicate configuration key at line %d", line)
		}
		// Validate keys here, even if an environment variable would override them.
		var check Config
		if err := check.set(key, v); err != nil {
			return nil, fmt.Errorf("invalid configuration entry at line %d", line)
		}
		values[key] = v
	}
	if scanner.Err() != nil {
		return nil, fmt.Errorf("cannot read configuration file")
	}
	return values, nil
}
