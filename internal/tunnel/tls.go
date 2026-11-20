package tunnel

import (
	"crypto/tls"
	"net"
	"time"

	"portman/internal/config"
	"portman/internal/logging"
)

// retryDelay is the base wait between dial attempts. Tests set it to 0.
var retryDelay = time.Second

func dialTLS(cfg config.Config, timeout time.Duration) (*tls.Conn, error) {
	d := &net.Dialer{Timeout: timeout}
	return tls.DialWithDialer(d, "tcp", cfg.RemoteAddr, &tls.Config{
		ServerName: cfg.ServerName,
		MinVersion: tls.VersionTLS12,
	})
}

// dialTLSWithRetry tries up to 3 times, waiting 1s and then 2s between attempts.
// Transient network failures are common on unreliable links.
func dialTLSWithRetry(cfg config.Config) (*tls.Conn, error) {
	const attempts = 3
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		c, err := dialTLS(cfg, dialTimeout)
		if err == nil {
			return c, nil
		}
		lastErr = err
		logging.Logf("warn", "TLS dial attempt %d/%d to %s failed: %v", attempt, attempts, cfg.RemoteAddr, err)
		if attempt < attempts {
			time.Sleep(time.Duration(attempt) * retryDelay)
		}
	}
	return nil, lastErr
}
