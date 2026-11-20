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
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		log.Fatalf("Error loading certificates: %v", err)
	}

	listener, err := tls.Listen("tcp", cfg.ListenAddr, &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		log.Fatalf("Server listener error on %s: %v", cfg.ListenAddr, err)
	}
	defer listener.Close()

	logging.Logf("info", "server listening on %s, forwarding to backend %s", cfg.ListenAddr, cfg.BackendAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			logging.Logf("error", "accept failed: %v", err)
			time.Sleep(100 * time.Millisecond) // avoid a hot loop on persistent errors
			continue
		}
		go handleServerConn(conn, cfg.BackendAddr)
	}
}

func handleServerConn(conn net.Conn, backendAddr string) {
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

	backend, err := net.DialTimeout("tcp", backendAddr, dialTimeout)
	if err != nil {
		logging.Logf("error", "conn#%d backend dial %s failed: %v", id, backendAddr, err)
		return
	}
	defer backend.Close()

	logging.Logf("info", "conn#%d bridged %s <-> backend %s", id, conn.RemoteAddr(), backendAddr)

	relay(conn, backend, id, "client->backend", "backend->client")

	logging.Logf("info", "conn#%d closed after %s", id, time.Since(start).Round(time.Millisecond))
}
