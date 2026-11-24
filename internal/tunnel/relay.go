package tunnel

import (
	"errors"
	"io"
	"net"
	"time"

	"portman/internal/logging"
)

// idleConn resets its read and write deadlines on every call, so a connection
// is closed only after it has been idle for the whole idle timeout.
type idleConn struct {
	net.Conn
	idle time.Duration
}

func (c idleConn) Read(p []byte) (int, error) {
	_ = c.Conn.SetReadDeadline(time.Now().Add(c.idle))
	return c.Conn.Read(p)
}

func (c idleConn) Write(p []byte) (int, error) {
	_ = c.Conn.SetWriteDeadline(time.Now().Add(c.idle))
	return c.Conn.Write(p)
}

// CloseWrite passes the half-close through, so the peer still sees EOF.
func (c idleConn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// copyStream copies src to dst, logs the byte count and any error,
// then half-closes dst so the peer sees EOF.
func copyStream(dst, src net.Conn, id uint64, dir string) {
	n, err := io.Copy(dst, src)
	switch {
	case err == nil || errors.Is(err, io.EOF):
		logging.Logf("debug", "conn#%d %s finished, %d bytes", id, dir, n)
	case isTimeout(err):
		logging.Logf("info", "conn#%d %s idle timeout after %d bytes", id, dir, n)
	case errors.Is(err, net.ErrClosed):
		logging.Logf("debug", "conn#%d %s closed after %d bytes", id, dir, n)
	default:
		logging.Logf("warn", "conn#%d %s stopped after %d bytes: %v", id, dir, n, err)
	}
	if cw, ok := dst.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
}

// relay runs both directions with an idle timeout on each side. When one
// direction ends, the other gets drainTimeout to finish, then both are closed.
func relay(a, b net.Conn, id uint64, aToB, bToA string, idle time.Duration) {
	a = idleConn{Conn: a, idle: idle}
	b = idleConn{Conn: b, idle: idle}

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
	drain := time.AfterFunc(drainTimeout, func() {
		a.Close()
		b.Close()
	})
	<-done
	drain.Stop()
}
