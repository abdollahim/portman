package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	_ "time/tzdata" // embeds the timezone database so zones work on any machine
)

type Config struct {
	Mode           string `json:"mode"`
	ListenAddr     string `json:"listen_addr"`
	RemoteAddr     string `json:"remote_addr,omitempty"`
	BackendAddr    string `json:"backend_addr,omitempty"`
	CertFile       string `json:"cert_file,omitempty"`
	KeyFile        string `json:"key_file,omitempty"`
	ServerName     string `json:"server_name,omitempty"`
	ClientCAFile   string `json:"client_ca_file,omitempty"`
	ClientCertFile string `json:"client_cert_file,omitempty"`
	ClientKeyFile  string `json:"client_key_file,omitempty"`
	MaxConnections int    `json:"max_connections,omitempty"`
	IdleTimeout    string `json:"idle_timeout,omitempty"`
	LogLevel       string `json:"log_level"`
	LogPath        string `json:"log_path"`
	Stats          Stats  `json:"stats"`
	Notify         Notify `json:"notify"`
}

// Stats controls traffic counting. Either server can enable it.
type Stats struct {
	Enabled        bool    `json:"enabled"`
	Label          string  `json:"label,omitempty"`
	StatePath      string  `json:"state_path,omitempty"`
	Interface      string  `json:"interface,omitempty"`
	Timezone       string  `json:"timezone,omitempty"`
	BillingDay     int     `json:"billing_day,omitempty"`
	MonthlyLimitGB float64 `json:"monthly_limit_gb,omitempty"`
	CountMode      string  `json:"count_mode,omitempty"`
	SampleInterval string  `json:"sample_interval,omitempty"`
	ListenAddr     string  `json:"listen_addr,omitempty"`
}

// Notify controls the daily report. It runs on the inside (client) server only.
type Notify struct {
	Enabled          bool   `json:"enabled"`
	APIBase          string `json:"api_base,omitempty"`
	BotToken         string `json:"bot_token,omitempty"`
	ChatID           string `json:"chat_id,omitempty"`
	SendAt           string `json:"send_at,omitempty"`
	ForeignStatsAddr string `json:"foreign_stats_addr,omitempty"`
}

const (
	defaultMaxConnections = 256
	defaultIdleTimeout    = 5 * time.Minute
	defaultSampleEvery    = time.Minute
	minSampleEvery        = 10 * time.Second
	defaultBaleAPI        = "https://tapi.bale.ai"
)

// MaxConn returns the connection limit, using the default when unset.
func (c Config) MaxConn() int {
	if c.MaxConnections <= 0 {
		return defaultMaxConnections
	}
	return c.MaxConnections
}

// IdleDuration returns the idle timeout, using the default when unset or invalid.
func (c Config) IdleDuration() time.Duration {
	d, err := time.ParseDuration(c.IdleTimeout)
	if err != nil || d <= 0 {
		return defaultIdleTimeout
	}
	return d
}

// Location returns the timezone that sets day and month boundaries.
func (s Stats) Location() (*time.Location, error) {
	name := s.Timezone
	if name == "" {
		name = "UTC"
	}
	return time.LoadLocation(name)
}

// Every returns how often interface counters are sampled.
func (s Stats) Every() time.Duration {
	d, err := time.ParseDuration(s.SampleInterval)
	if err != nil || d < minSampleEvery {
		return defaultSampleEvery
	}
	return d
}

// Mode returns the monthly counting rule: "sum" (in + out) or "max" (the larger one).
func (s Stats) Mode() string {
	if s.CountMode == "max" {
		return "max"
	}
	return "sum"
}

// BaleBase returns the Bale API base URL without a trailing slash.
func (n Notify) BaleBase() string {
	if n.APIBase == "" {
		return defaultBaleAPI
	}
	return strings.TrimRight(n.APIBase, "/")
}

// Clock returns the send time as hour and minute.
func (n Notify) Clock() (int, int, error) {
	t, err := time.Parse("15:04", n.SendAt)
	if err != nil {
		return 0, 0, errors.New("notify.send_at must be HH:MM, for example 23:55")
	}
	return t.Hour(), t.Minute(), nil
}

// Load reads and validates a config file.
func Load(path string) (Config, error) {
	var cfg Config
	f, err := os.Open(path)
	if err != nil {
		return cfg, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

func (c Config) IsServer() bool {
	if c.Mode != "" {
		return c.Mode == "server"
	}
	return c.CertFile != ""
}

func (c Config) Validate() error {
	if c.ListenAddr == "" {
		return errors.New("listen_addr is required")
	}
	if c.LogPath == "" {
		return errors.New("log_path is required")
	}
	if c.MaxConnections < 0 {
		return errors.New("max_connections cannot be negative")
	}
	if c.IdleTimeout != "" {
		d, err := time.ParseDuration(c.IdleTimeout)
		if err != nil || d <= 0 {
			return errors.New("idle_timeout must be a positive duration such as 5m")
		}
	}

	if c.IsServer() {
		if c.CertFile == "" || c.KeyFile == "" {
			return errors.New("server mode needs cert_file and key_file")
		}
		if c.BackendAddr == "" {
			return errors.New("server mode needs backend_addr")
		}
		if c.ClientCertFile != "" || c.ClientKeyFile != "" {
			return errors.New("client_cert_file and client_key_file are for client mode only")
		}
		if c.Notify.Enabled {
			return errors.New("notify is for the inside (client) server only")
		}
		return c.Stats.validate(true, c.ClientCAFile)
	}

	if c.RemoteAddr == "" {
		return errors.New("client mode needs remote_addr")
	}
	if c.ClientCAFile != "" {
		return errors.New("client_ca_file is for server mode only")
	}
	if (c.ClientCertFile == "") != (c.ClientKeyFile == "") {
		return errors.New("client_cert_file and client_key_file must be set together")
	}
	if err := c.Stats.validate(false, ""); err != nil {
		return err
	}
	return c.Notify.validate(c.Stats, c.ClientCertFile, c.ClientKeyFile, c.ServerName)
}

func (s Stats) validate(server bool, clientCA string) error {
	if !s.Enabled {
		if s.ListenAddr != "" {
			return errors.New("stats.listen_addr needs stats.enabled = true")
		}
		return nil
	}
	if s.Label == "" {
		return errors.New("stats.label is required when stats is enabled")
	}
	if s.StatePath == "" {
		return errors.New("stats.state_path is required when stats is enabled")
	}
	if s.Interface == "" {
		return errors.New("stats.interface is required, for example eth0")
	}
	if _, err := s.Location(); err != nil {
		return fmt.Errorf("stats.timezone: %w", err)
	}
	if s.CountMode != "" && s.CountMode != "sum" && s.CountMode != "max" {
		return errors.New(`stats.count_mode must be "sum" or "max"`)
	}
	if s.MonthlyLimitGB < 0 {
		return errors.New("stats.monthly_limit_gb cannot be negative")
	}
	if s.SampleInterval != "" {
		d, err := time.ParseDuration(s.SampleInterval)
		if err != nil || d < minSampleEvery {
			return errors.New("stats.sample_interval must be at least 10s, such as 1m")
		}
	}
	if s.ListenAddr != "" {
		if !server {
			return errors.New("stats.listen_addr is for server mode only")
		}
		if clientCA == "" {
			return errors.New("stats.listen_addr needs client_ca_file, so the stats endpoint requires a client certificate")
		}
	}
	if s.BillingDay != 0 && (s.BillingDay < 1 || s.BillingDay > 28) {
		return errors.New("stats.billing_day must be between 1 and 28")
	}
	return nil
}

func (n Notify) validate(s Stats, certFile, keyFile, serverName string) error {
	if !n.Enabled {
		return nil
	}
	if !s.Enabled {
		return errors.New("notify needs stats.enabled = true, because the report uses the counters")
	}
	if n.BotToken == "" {
		return errors.New("notify.bot_token is required")
	}
	if n.ChatID == "" {
		return errors.New("notify.chat_id is required")
	}
	if _, _, err := n.Clock(); err != nil {
		return err
	}
	if n.APIBase != "" && !strings.HasPrefix(n.APIBase, "https://") && !strings.HasPrefix(n.APIBase, "http://") {
		return errors.New("notify.api_base must start with https://")
	}
	if n.ForeignStatsAddr != "" {
		if certFile == "" || keyFile == "" {
			return errors.New("notify.foreign_stats_addr needs client_cert_file and client_key_file")
		}
		if serverName == "" {
			return errors.New("notify.foreign_stats_addr needs server_name")
		}
	}
	return nil
}
