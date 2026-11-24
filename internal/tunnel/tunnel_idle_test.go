package tunnel

import (
	"net"
	"testing"
	"time"
)

func TestIdleConnectionIsClosed(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()

	idle := idleConn{Conn: a, idle: 100 * time.Millisecond}
	start := time.Now()
	_, err := idle.Read(make([]byte, 1))
	if !isTimeout(err) {
		t.Fatalf("expected an idle timeout, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("idle timeout took too long: %v", elapsed)
	}
}
