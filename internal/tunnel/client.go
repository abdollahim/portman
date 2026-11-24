package tunnel

import (
	"log"
	"net"
	"time"

	"portman/internal/config"
	"portman/internal/logging"
)

func RunClient(cfg config.Config) {
	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		log.Fatalf("Client listener error on %s: %v", cfg.ListenAddr, err)
	}
	defer listener.Close()

	slots := newLimiter(cfg.MaxConn())
	logging.Logf("info", "client listening on %s, tunnel to %s (max %d connections, idle timeout %s)",
		cfg.ListenAddr, cfg.RemoteAddr, cfg.MaxConn(), cfg.IdleDuration())

	for {
		conn, err := listener.Accept()
		if err != nil {
			logging.Logf("error", "local accept failed: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if !slots.acquire() {
			logging.Logf("warn", "connection limit %d reached, dropping local client %s", cfg.MaxConn(), conn.RemoteAddr())
			conn.Close()
			continue
		}
		go func() {
			defer slots.release()
			handleClientConn(conn, cfg)
		}()
	}
}

func handleClientConn(local net.Conn, cfg config.Config) {
	id := connSeq.Add(1)
	start := time.Now()
	defer local.Close()

	logging.Logf("debug", "conn#%d accepted local client %s", id, local.RemoteAddr())

	dialStart := time.Now()
	remote, err := dialTLSWithRetry(cfg)
	if err != nil {
		logging.Logf("error", "conn#%d TLS dial to %s failed: %v", id, cfg.RemoteAddr, err)
		return
	}
	defer remote.Close()

	logging.Logf("info", "conn#%d tunnel up to %s from local port %s, dial took %s",
		id, cfg.RemoteAddr, remote.LocalAddr(), time.Since(dialStart).Round(time.Millisecond))
	recorder.AddConn()

	relay(local, remote, id, "local->remote", "remote->local", cfg.IdleDuration())

	logging.Logf("info", "conn#%d closed after %s", id, time.Since(start).Round(time.Millisecond))
}
