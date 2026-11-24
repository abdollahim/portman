package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
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
}

const (
	defaultMaxConnections = 256
	defaultIdleTimeout    = 5 * time.Minute
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
		return nil
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
	return nil
}
