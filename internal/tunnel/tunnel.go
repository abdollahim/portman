// Package tunnel implements the TLS bridge client and server.
package tunnel

import (
	"sync/atomic"
	"time"
)

const (
	dialTimeout      = 10 * time.Second
	handshakeTimeout = 10 * time.Second
	drainTimeout     = 15 * time.Second
	checkTimeout     = 10 * time.Second
)

// connSeq numbers connections so client and server log lines can be matched.
var connSeq atomic.Uint64