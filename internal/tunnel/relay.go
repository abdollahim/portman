package tunnel

import (
	"errors"
	"io"
	"net"
	"os"
	"time"

	"portman/internal/logging"
)

// copyStream copies src to dst, logs the byte count and any error,
// then half-closes dst so the peer sees EOF.
func copyStream(dst, src net.Conn, id uint64, dir string) {
	n, err := io.Copy(dst, src)
	switch {
	case err == nil || errors.Is(err, io.EOF):
		logging.Logf("debug", "conn#%d %s finished, %d bytes", id, dir, n)
	case errors.Is(err, os.ErrDeadlineExceeded):
		logging.Logf("debug", "conn#%d %s stopped by drain timeout after %d bytes", id, dir, n)
	default:
		logging.Logf("warn", "conn#%d %s stopped after %d bytes: %v", id, dir, n, err)
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