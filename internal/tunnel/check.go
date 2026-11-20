package tunnel

import (
	"net"

	"portman/internal/config"
	"portman/internal/logging"
)

// StartupChecks logs whether the peer is reachable. It never stops the service.
func StartupChecks(cfg config.Config) {
	if cfg.IsServer() {
		checkTCP("backend", cfg.BackendAddr)
		return
	}
	if checkTCP("remote bridge", cfg.RemoteAddr) {
		checkTLS(cfg)
	}
}

func checkTCP(name, addr string) bool {
	c, err := net.DialTimeout("tcp", addr, checkTimeout)
	if err != nil {
		logging.Logf("warn", "startup check: %s %s TCP connect failed: %v", name, addr, err)
		return false
	}
	c.Close()
	logging.Logf("info", "startup check: %s %s TCP connect OK", name, addr)
	return true
}

func checkTLS(cfg config.Config) bool {
	c, err := dialTLS(cfg, checkTimeout)
	if err != nil {
		logging.Logf("warn", "startup check: TLS handshake to %s failed: %v", cfg.RemoteAddr, err)
		return false
	}
	c.Close()
	logging.Logf("info", "startup check: TLS handshake to %s OK (server_name=%q)", cfg.RemoteAddr, cfg.ServerName)
	return true
}
