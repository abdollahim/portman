package config

import (
	"os"
	"path/filepath"
	"testing"
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
