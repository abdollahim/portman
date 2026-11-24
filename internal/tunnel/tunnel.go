// Package tunnel implements the TLS bridge client and server.
package tunnel

import (
	"sync/atomic"
	"time"

	"portman/internal/stats"
)

const (
	dialTimeout      = 10 * time.Second
	handshakeTimeout = 10 * time.Second
	drainTimeout     = 15 * time.Second
	checkTimeout     = 10 * time.Second
)

// connSeq numbers connections so client and server log lines can be matched.
var connSeq atomic.Uint64

// recorder receives bridge traffic counts. It is stats.Discard until SetRecorder is called.
var recorder stats.Recorder = stats.Discard{}

// SetRecorder sets where bridge traffic is counted. Call it before the listeners start.
func SetRecorder(r stats.Recorder) {
	recorder = r
}

// limiter caps the number of connections handled at once.
type limiter chan struct{}

func newLimiter(n int) limiter {
	return make(limiter, n)
}

// acquire takes a slot without blocking. It returns false when the limit is reached.
func (l limiter) acquire() bool {
	select {
	case l <- struct{}{}:
		return true
	default:
		return false
	}
}

func (l limiter) release() {
	<-l
}
