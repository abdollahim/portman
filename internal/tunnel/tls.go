package tunnel

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"portman/internal/config"
	"portman/internal/logging"
)

// retryDelay is the base wait between dial attempts. Tests set it to 0.
var retryDelay = time.Second

// clientTLSConfig builds the TLS settings used to dial the server.
func clientTLSConfig(cfg config.Config) (*tls.Config, error) {
	tc := &tls.Config{
		ServerName: cfg.ServerName,
		MinVersion: tls.VersionTLS12,
	}
	if cfg.ClientCertFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.ClientCertFile, cfg.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		tc.Certificates = []tls.Certificate{cert}
	}
	return tc, nil
}

// serverTLSConfig builds the TLS settings for the listener. When client_ca_file
// is set, clients must present a certificate signed by that CA.
func serverTLSConfig(cfg config.Config) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load server certificate: %w", err)
	}
	tc := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	if cfg.ClientCAFile != "" {
		pool, err := loadCAPool(cfg.ClientCAFile)
		if err != nil {
			return nil, err
		}
		tc.ClientAuth = tls.RequireAndVerifyClientCert
		tc.ClientCAs = pool
	}
	return tc, nil
}

func loadCAPool(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read client CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, errors.New("client CA file contains no valid certificates")
	}
	return pool, nil
}

func dialTLS(cfg config.Config, timeout time.Duration) (*tls.Conn, error) {
	tc, err := clientTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	d := &net.Dialer{Timeout: timeout}
	return tls.DialWithDialer(d, "tcp", cfg.RemoteAddr, tc)
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
