package tunnel

import (
	"crypto/tls"
	"log"
	"net"
	"time"

	"portman/internal/config"
	"portman/internal/logging"
)

func RunServer(cfg config.Config) {
	tlsCfg, err := serverTLSConfig(cfg)
	if err != nil {
		log.Fatalf("TLS setup: %v", err)
	}
	if cfg.ClientCAFile != "" {
		logging.Logf("info", "client certificates required (client_ca_file set)")
	}

	listener, err := tls.Listen("tcp", cfg.ListenAddr, tlsCfg)
	if err != nil {
		log.Fatalf("Server listener error on %s: %v", cfg.ListenAddr, err)
	}
	defer listener.Close()

	slots := newLimiter(cfg.MaxConn())
	logging.Logf("info", "server listening on %s, forwarding to backend %s (max %d connections, idle timeout %s)",
		cfg.ListenAddr, cfg.BackendAddr, cfg.MaxConn(), cfg.IdleDuration())

	for {
		conn, err := listener.Accept()
		if err != nil {
			logging.Logf("error", "accept failed: %v", err)
			time.Sleep(100 * time.Millisecond) // avoid a hot loop on persistent errors
			continue
		}
		if !slots.acquire() {
			logging.Logf("warn", "connection limit %d reached, dropping %s", cfg.MaxConn(), conn.RemoteAddr())
			conn.Close()
			continue
		}
		go func() {
			defer slots.release()
			handleServerConn(conn, cfg)
		}()
	}
}

func handleServerConn(conn net.Conn, cfg config.Config) {
	id := connSeq.Add(1)
	start := time.Now()
	defer conn.Close()

	logging.Logf("debug", "conn#%d accepted from %s", id, conn.RemoteAddr())

	if tc, ok := conn.(*tls.Conn); ok {
		_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))
		if err := tc.Handshake(); err != nil {
			logging.Logf("warn", "conn#%d TLS handshake from %s failed: %v", id, conn.RemoteAddr(), err)
			return
		}
		_ = conn.SetDeadline(time.Time{})
	}

	backend, err := net.DialTimeout("tcp", cfg.BackendAddr, dialTimeout)
	if err != nil {
		logging.Logf("error", "conn#%d backend dial %s failed: %v", id, cfg.BackendAddr, err)
		return
	}
	defer backend.Close()

	logging.Logf("info", "conn#%d bridged %s <-> backend %s", id, conn.RemoteAddr(), cfg.BackendAddr)

	relay(conn, backend, id, "client->backend", "backend->client", cfg.IdleDuration())

	logging.Logf("info", "conn#%d closed after %s", id, time.Since(start).Round(time.Millisecond))
}
