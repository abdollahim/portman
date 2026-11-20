package tunnel

import (
	"net"
	"testing"

	"portman/internal/config"
)

func TestDialTLSWithRetryGivesUp(t *testing.T) {
	old := retryDelay
	retryDelay = 0
	t.Cleanup(func() { retryDelay = old })

	// Reserve a port, then close it so nothing is listening there.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	cfg := config.Config{Mode: "client", ListenAddr: "127.0.0.1:0", RemoteAddr: addr, LogPath: "x.log"}
	if _, err := dialTLSWithRetry(cfg); err == nil {
		t.Fatal("expected an error when nothing listens on the remote address")
	}
}
