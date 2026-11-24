package logging

import (
	"fmt"
	"log"
	"regexp"
	"strings"
)

var (
	rank  = map[string]int{"debug": 1, "info": 2, "warn": 3, "error": 4}
	level = rank["info"]

	// privateKeyBlock matches a PEM private key, from its BEGIN line to its END
	// line. If the END line is missing, it matches to the end of the message.
	privateKeyBlock = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(-----END [A-Z ]*PRIVATE KEY-----|$)`)
)

// SetLevel sets the minimum level. Unknown names fall back to info.
func SetLevel(name string) {
	if r, ok := rank[strings.ToLower(name)]; ok {
		level = r
		return
	}
	level = rank["info"]
}

// Redact replaces any PEM private key in s with a placeholder.
func Redact(s string) string {
	return privateKeyBlock.ReplaceAllString(s, "[redacted private key]")
}

// Logf writes a leveled line. Private keys are redacted before writing.
// log.Output(2, ...) keeps the caller's file:line.
func Logf(lvl, format string, args ...any) {
	if rank[lvl] < level {
		return
	}
	msg := Redact(fmt.Sprintf(format, args...))
	log.Output(2, fmt.Sprintf("[%-5s] %s", strings.ToUpper(lvl), msg))
}
