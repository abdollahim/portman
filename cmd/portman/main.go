package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// Set at build time by release.ps1 via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type Config struct {
	Mode        string `json:"mode"` // "client" or "server"
	ListenAddr  string `json:"listen_addr"`
	RemoteAddr  string `json:"remote_addr,omitempty"`
	BackendAddr string `json:"backend_addr,omitempty"`
	CertFile    string `json:"cert_file,omitempty"`
	KeyFile     string `json:"key_file,omitempty"`
	ServerName  string `json:"server_name,omitempty"`
	LogLevel    string `json:"log_level"` // "debug", "info", "warn", "error"
	LogPath     string `json:"log_path"`
}

const (
	dialTimeout      = 10 * time.Second
	handshakeTimeout = 10 * time.Second
	drainTimeout     = 15 * time.Second
	checkTimeout     = 10 * time.Second
)

var (
	levelRank   = map[string]int{"debug": 1, "info": 2, "warn": 3, "error": 4}
	logLevelNum = levelRank["info"]
	connSeq     atomic.Uint64
)

func setLogLevel(level string) {
	r, ok := levelRank[strings.ToLower(level)]
	if !ok {
		r = levelRank["info"]
	}
	logLevelNum = r
}

// logf writes a tagged line. log.Output(2, ...) keeps the file:line of the caller.
func logf(level, format string, args ...any) {
	if levelRank[level] < logLevelNum {
		return
	}
	msg := fmt.Sprintf(format, args...)
	log.Output(2, fmt.Sprintf("[%-5s] %s", strings.ToUpper(level), msg))
}

func isServer(cfg Config) bool {
	if cfg.Mode != "" {
		return cfg.Mode == "server"
	}
	return cfg.CertFile != ""
}

func validate(cfg Config) error {
	if cfg.ListenAddr == "" {
		return errors.New("listen_addr is required")
	}
	if cfg.LogPath == "" {
		return errors.New("log_path is required")
	}
	if isServer(cfg) {
		if cfg.CertFile == "" || cfg.KeyFile == "" {
			return errors.New("server mode needs cert_file and key_file")
		}
		if cfg.BackendAddr == "" {
			return errors.New("server mode needs backend_addr")
		}
	} else if cfg.RemoteAddr == "" {
		return errors.New("client mode needs remote_addr")
	}
	return nil
}

func main() {
	configFile := flag.String("config", "config.json", "path to config file")
	flag.Parse()

	file, err := os.Open(*configFile)
	if err != nil {
		log.Fatalf("Failed to open config: %v", err)
	}
	defer file.Close()

	var cfg Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		log.Fatalf("Failed to decode config: %v", err)
	}
	if err := validate(cfg); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	setLogLevel(cfg.LogLevel)

	logFile, err := os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()

	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	mode := "client"
	if isServer(cfg) {
		mode = "server"
	}
	logf("info", "starting portman in %s mode, log level %s", mode, strings.ToLower(cfg.LogLevel))
	logf("info", "portman %s (commit %s, built %s)", version, commit, date)

	go startupChecks(cfg)

	if isServer(cfg) {
		runServer(cfg)
	} else {
		runClient(cfg)
	}
}

// ---------- startup checks (log only, never stop the service) ----------

func startupChecks(cfg Config) {
	if isServer(cfg) {
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
		logf("warn", "startup check: %s %s TCP connect failed: %v", name, addr, err)
		return false
	}
	c.Close()
	logf("info", "startup check: %s %s TCP connect OK", name, addr)
	return true
}

func checkTLS(cfg Config) bool {
	c, err := dialTLS(cfg, checkTimeout)
	if err != nil {
		logf("warn", "startup check: TLS handshake to %s failed: %v", cfg.RemoteAddr, err)
		return false
	}
	c.Close()
	logf("info", "startup check: TLS handshake to %s OK (server_name=%q)", cfg.RemoteAddr, cfg.ServerName)
	return true
}

func dialTLS(cfg Config, timeout time.Duration) (*tls.Conn, error) {
	d := &net.Dialer{Timeout: timeout}
	return tls.DialWithDialer(d, "tcp", cfg.RemoteAddr, &tls.Config{
		ServerName: cfg.ServerName,
		MinVersion: tls.VersionTLS12,
	})
}

// ---------- server ----------

func runServer(cfg Config) {
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

	logf("info", "server listening on %s, forwarding to backend %s", cfg.ListenAddr, cfg.BackendAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			logf("error", "accept failed: %v", err)
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

	logf("debug", "conn#%d accepted from %s", id, conn.RemoteAddr())

	if tc, ok := conn.(*tls.Conn); ok {
		_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))
		if err := tc.Handshake(); err != nil {
			logf("warn", "conn#%d TLS handshake from %s failed: %v", id, conn.RemoteAddr(), err)
			return
		}
		_ = conn.SetDeadline(time.Time{})
	}

	backend, err := net.DialTimeout("tcp", backendAddr, dialTimeout)
	if err != nil {
		logf("error", "conn#%d backend dial %s failed: %v", id, backendAddr, err)
		return
	}
	defer backend.Close()

	logf("info", "conn#%d bridged %s <-> backend %s", id, conn.RemoteAddr(), backendAddr)

	relay(conn, backend, id, "client->backend", "backend->client")

	logf("info", "conn#%d closed after %s", id, time.Since(start).Round(time.Millisecond))
}

// ---------- client ----------

func runClient(cfg Config) {
	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		log.Fatalf("Client listener error on %s: %v", cfg.ListenAddr, err)
	}
	defer listener.Close()

	logf("info", "client listening on %s, tunnel to %s", cfg.ListenAddr, cfg.RemoteAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			logf("error", "local accept failed: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		go handleClientConn(conn, cfg)
	}
}

func handleClientConn(local net.Conn, cfg Config) {
	id := connSeq.Add(1)
	start := time.Now()
	defer local.Close()

	logf("debug", "conn#%d accepted local client %s", id, local.RemoteAddr())

	remote, err := dialTLS(cfg, dialTimeout)
	if err != nil {
		logf("error", "conn#%d TLS dial to %s failed: %v", id, cfg.RemoteAddr, err)
		return
	}
	defer remote.Close()

	logf("info", "conn#%d tunnel up to %s", id, cfg.RemoteAddr)

	relay(local, remote, id, "local->remote", "remote->local")

	logf("info", "conn#%d closed after %s", id, time.Since(start).Round(time.Millisecond))
}

// ---------- shared relay ----------

// copyStream copies src to dst, logs the byte count and any error,
// then half-closes dst so the peer sees EOF.
func copyStream(dst, src net.Conn, id uint64, dir string) {
	n, err := io.Copy(dst, src)
	switch {
	case err == nil || errors.Is(err, io.EOF):
		logf("debug", "conn#%d %s finished, %d bytes", id, dir, n)
	case errors.Is(err, os.ErrDeadlineExceeded):
		logf("debug", "conn#%d %s stopped by drain timeout after %d bytes", id, dir, n)
	default:
		logf("warn", "conn#%d %s stopped after %d bytes: %v", id, dir, n, err)
	}
	if cw, ok := dst.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
}

// relay runs both directions. When one direction ends, the other gets
// drainTimeout to finish before both connections are forced closed.
func relay(a, b net.Conn, id uint64, aToB, bToA string) {
	done := make(chan struct{}, 2)

	go func() {
		copyStream(b, a, id, aToB)
		done <- struct{}{}
	}()
	go func() {
		copyStream(a, b, id, bToA)
		done <- struct{}{}
	}()

	<-done

	deadline := time.Now().Add(drainTimeout)
	_ = a.SetDeadline(deadline)
	_ = b.SetDeadline(deadline)

	<-done
}
