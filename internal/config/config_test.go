package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsServer(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"explicit server", Config{Mode: "server"}, true},
		{"explicit client wins over cert", Config{Mode: "client", CertFile: "c.pem"}, false},
		{"inferred from cert", Config{CertFile: "c.pem"}, true},
		{"default is client", Config{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.IsServer(); got != tc.want {
				t.Fatalf("IsServer() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	server := Config{
		Mode: "server", ListenAddr: ":4443", LogPath: "x.log",
		CertFile: "c.pem", KeyFile: "k.pem", BackendAddr: "127.0.0.1:10000",
	}
	client := Config{
		Mode: "client", ListenAddr: "127.0.0.1:20001", LogPath: "x.log",
		RemoteAddr: "bridge.example.com:4443",
	}

	cases := []struct {
		name    string
		base    Config
		edit    func(*Config)
		wantErr bool
	}{
		{"valid server", server, func(*Config) {}, false},
		{"valid client", client, func(*Config) {}, false},
		{"missing listen", client, func(c *Config) { c.ListenAddr = "" }, true},
		{"missing log path", client, func(c *Config) { c.LogPath = "" }, true},
		{"client without remote", client, func(c *Config) { c.RemoteAddr = "" }, true},
		{"server without cert", server, func(c *Config) { c.CertFile = "" }, true},
		{"server without key", server, func(c *Config) { c.KeyFile = "" }, true},
		{"server without backend", server, func(c *Config) { c.BackendAddr = "" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.base
			tc.edit(&cfg)
			err := cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.json")
	data := `{"mode":"client","listen_addr":"127.0.0.1:20001","remote_addr":"h:4443","server_name":"h","log_level":"debug","log_path":"p.log"}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RemoteAddr != "h:4443" || cfg.LogLevel != "debug" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadRejectsBadInput(t *testing.T) {
	dir := t.TempDir()

	truncated := filepath.Join(dir, "truncated.json")
	if err := os.WriteFile(truncated, []byte(`{"mode":"client"`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(truncated); err == nil {
		t.Fatal("expected error for truncated JSON")
	}

	if _, err := Load(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestMutualTLSFieldValidation(t *testing.T) {
	client := Config{Mode: "client", ListenAddr: "127.0.0.1:20001", LogPath: "x.log", RemoteAddr: "h:4443"}
	server := Config{
		Mode: "server", ListenAddr: ":4443", LogPath: "x.log",
		CertFile: "c.pem", KeyFile: "k.pem", BackendAddr: "127.0.0.1:10000",
	}

	cases := []struct {
		name    string
		base    Config
		edit    func(*Config)
		wantErr bool
	}{
		{"client cert with key", client, func(c *Config) { c.ClientCertFile = "c.pem"; c.ClientKeyFile = "k.pem" }, false},
		{"client cert without key", client, func(c *Config) { c.ClientCertFile = "c.pem" }, true},
		{"client key without cert", client, func(c *Config) { c.ClientKeyFile = "k.pem" }, true},
		{"client with server CA", client, func(c *Config) { c.ClientCAFile = "ca.pem" }, true},
		{"server with client CA", server, func(c *Config) { c.ClientCAFile = "ca.pem" }, false},
		{"server with client cert", server, func(c *Config) { c.ClientCertFile = "c.pem"; c.ClientKeyFile = "k.pem" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.base
			tc.edit(&cfg)
			err := cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestHardeningDefaults(t *testing.T) {
	var cfg Config
	if cfg.MaxConn() != 256 {
		t.Fatalf("MaxConn() = %d, want 256", cfg.MaxConn())
	}
	if cfg.IdleDuration() != 5*time.Minute {
		t.Fatalf("IdleDuration() = %v, want 5m", cfg.IdleDuration())
	}

	cfg.MaxConnections = 10
	cfg.IdleTimeout = "30s"
	if cfg.MaxConn() != 10 {
		t.Fatalf("MaxConn() = %d, want 10", cfg.MaxConn())
	}
	if cfg.IdleDuration() != 30*time.Second {
		t.Fatalf("IdleDuration() = %v, want 30s", cfg.IdleDuration())
	}
}

func TestHardeningValidation(t *testing.T) {
	base := Config{Mode: "client", ListenAddr: "127.0.0.1:20001", LogPath: "x.log", RemoteAddr: "h:4443"}
	cases := []struct {
		name    string
		edit    func(*Config)
		wantErr bool
	}{
		{"valid idle timeout", func(c *Config) { c.IdleTimeout = "5m" }, false},
		{"bad idle timeout", func(c *Config) { c.IdleTimeout = "bogus" }, true},
		{"zero idle timeout", func(c *Config) { c.IdleTimeout = "0s" }, true},
		{"negative max connections", func(c *Config) { c.MaxConnections = -1 }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.edit(&cfg)
			err := cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestStatsAndNotifyValidation(t *testing.T) {
	base := Config{
		Mode: "client", ListenAddr: "127.0.0.1:20001", LogPath: "x.log",
		RemoteAddr: "h:4443", ServerName: "h",
		ClientCertFile: "c.pem", ClientKeyFile: "k.pem",
	}
	goodStats := Stats{Enabled: true, Label: "inside", StatePath: "s.json", Interface: "eth0", Timezone: "Asia/Tehran"}
	goodNotify := Notify{Enabled: true, BotToken: "123456789:TOKEN", ChatID: "1", SendAt: "23:55", ForeignStatsAddr: "h:4444"}

	cases := []struct {
		name    string
		edit    func(*Config)
		wantErr bool
	}{
		{"stats only", func(c *Config) { c.Stats = goodStats }, false},
		{"stats without label", func(c *Config) { c.Stats = goodStats; c.Stats.Label = "" }, true},
		{"bad timezone", func(c *Config) { c.Stats = goodStats; c.Stats.Timezone = "Not/AZone" }, true},
		{"bad count mode", func(c *Config) { c.Stats = goodStats; c.Stats.CountMode = "avg" }, true},
		{"sample interval too short", func(c *Config) { c.Stats = goodStats; c.Stats.SampleInterval = "5s" }, true},
		{"stats listen on client", func(c *Config) { c.Stats = goodStats; c.Stats.ListenAddr = ":4444" }, true},
		{"notify without stats", func(c *Config) { c.Notify = goodNotify }, true},
		{"notify valid", func(c *Config) { c.Stats = goodStats; c.Notify = goodNotify }, false},
		{"notify without token", func(c *Config) { c.Stats = goodStats; c.Notify = goodNotify; c.Notify.BotToken = "" }, true},
		{"bad send_at", func(c *Config) { c.Stats = goodStats; c.Notify = goodNotify; c.Notify.SendAt = "25:99" }, true},
		{"foreign fetch without client cert", func(c *Config) {
			c.Stats = goodStats
			c.Notify = goodNotify
			c.ClientCertFile, c.ClientKeyFile = "", ""
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.edit(&cfg)
			err := cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestServerStatsRequiresClientCA(t *testing.T) {
	server := Config{
		Mode: "server", ListenAddr: ":4443", LogPath: "x.log",
		CertFile: "c.pem", KeyFile: "k.pem", BackendAddr: "127.0.0.1:10000",
		Stats: Stats{Enabled: true, Label: "foreign", StatePath: "s.json", Interface: "eth0", ListenAddr: ":4444"},
	}
	if err := server.Validate(); err == nil {
		t.Fatal("stats endpoint without client_ca_file must be refused")
	}
	server.ClientCAFile = "ca.pem"
	if err := server.Validate(); err != nil {
		t.Fatalf("with client_ca_file it should be valid: %v", err)
	}
	server.Notify = Notify{Enabled: true}
	if err := server.Validate(); err == nil {
		t.Fatal("notify must be refused on the server")
	}
}

func TestBillingDayValidation(t *testing.T) {
	base := Config{
		Mode: "client", ListenAddr: "127.0.0.1:20001", LogPath: "x.log", RemoteAddr: "h:4443",
		Stats: Stats{Enabled: true, Label: "inside", StatePath: "s.json", Interface: "eth0"},
	}
	cases := []struct {
		day     int
		wantErr bool
	}{{0, false}, {1, false}, {28, false}, {31, false}, {32, true}, {-1, true}}

	for _, tc := range cases {
		cfg := base
		cfg.Stats.BillingDay = tc.day
		if err := cfg.Validate(); (err != nil) != tc.wantErr {
			t.Fatalf("billing_day %d: error = %v, wantErr %v", tc.day, err, tc.wantErr)
		}
	}
}

func TestChecksValidation(t *testing.T) {
	base := Config{
		Mode: "client", ListenAddr: "127.0.0.1:20001", LogPath: "x.log", RemoteAddr: "h:4443",
		ClientCertFile: "c.pem", ClientKeyFile: "k.pem",
		Notify: Notify{BotToken: "123456:TOKEN", ChatID: "1"},
		Checks: Checks{Enabled: true, SendDays: []string{"friday"}, SendAt: "07:00", TargetsFile: "t.json"},
	}
	server := Config{
		Mode: "server", ListenAddr: ":4443", LogPath: "x.log",
		CertFile: "c.pem", KeyFile: "k.pem", BackendAddr: "127.0.0.1:10000",
		Checks: Checks{Enabled: true, SendDays: []string{"friday"}, SendAt: "07:00", TargetsFile: "t.json"},
	}

	cases := []struct {
		name    string
		cfg     Config
		edit    func(*Config)
		wantErr bool
	}{
		{"valid", base, func(*Config) {}, false},
		{"no days", base, func(c *Config) { c.Checks.SendDays = nil }, true},
		{"unknown day", base, func(c *Config) { c.Checks.SendDays = []string{"funday"} }, true},
		{"bad time", base, func(c *Config) { c.Checks.SendAt = "7am" }, true},
		{"no targets file", base, func(c *Config) { c.Checks.TargetsFile = "" }, true},
		{"bad timezone", base, func(c *Config) { c.Checks.Timezone = "Mars/Base" }, true},
		{"no bot token", base, func(c *Config) { c.Notify.BotToken = "" }, true},
		{"telegram provider", base, func(c *Config) { c.Notify.Provider = "telegram" }, false},
		{"unknown provider", base, func(c *Config) { c.Notify.Provider = "signal" }, true},
		{"disabled ignores bad fields", base, func(c *Config) { c.Checks.Enabled = false; c.Checks.SendAt = "bad" }, false},
		{"server refuses checks", server, func(*Config) {}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			tc.edit(&cfg)
			err := cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
