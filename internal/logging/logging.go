package logging

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
)

var (
	rank  = map[string]int{"debug": 1, "info": 2, "warn": 3, "error": 4}
	level = rank["info"]

	// privateKeyBlock matches a PEM private key from its BEGIN line to its END line.
	privateKeyBlock = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(-----END [A-Z ]*PRIVATE KEY-----|$)`)

	secretsMu sync.RWMutex
	secrets   []string
)

// SetLevel sets the minimum level. Unknown names fall back to info.
func SetLevel(name string) {
	if r, ok := rank[strings.ToLower(name)]; ok {
		level = r
		return
	}
	level = rank["info"]
}

// AddSecret registers a value, such as a bot token, that must never appear in logs.
// Very short values are ignored so common words are not masked by accident.
func AddSecret(s string) {
	if len(s) < 8 {
		return
	}
	secretsMu.Lock()
	defer secretsMu.Unlock()
	secrets = append(secrets, s)
}

// Redact removes private key blocks and registered secrets from s.
func Redact(s string) string {
	s = privateKeyBlock.ReplaceAllString(s, "[redacted private key]")
	secretsMu.RLock()
	defer secretsMu.RUnlock()
	for _, secret := range secrets {
		s = strings.ReplaceAll(s, secret, "[redacted]")
	}
	return s
}

// Logf writes a leveled line. Secrets are redacted before writing.
// log.Output(2, ...) keeps the caller's file:line.
func Logf(lvl, format string, args ...any) {
	if rank[lvl] < level {
		return
	}
	msg := Redact(fmt.Sprintf(format, args...))
	log.Output(2, fmt.Sprintf("[%-5s] %s", strings.ToUpper(lvl), msg))
}
