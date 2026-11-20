package tunnel

import (
	"crypto/tls"
	"net"
	"time"

	"portman/internal/config"
)

func dialTLS(cfg config.Config, timeout time.Duration) (*tls.Conn, error) {
	d := &net.Dialer{Timeout: timeout}
	return tls.DialWithDialer(d, "tcp", cfg.RemoteAddr, &tls.Config{
		ServerName: cfg.ServerName,
		MinVersion: tls.VersionTLS12,
	})
}
