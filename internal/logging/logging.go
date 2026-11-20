package logging

import (
	"fmt"
	"log"
	"strings"
)

var (
	rank  = map[string]int{"debug": 1, "info": 2, "warn": 3, "error": 4}
	level = rank["info"]
)

// SetLevel sets the minimum level. Unknown names fall back to info.
func SetLevel(name string) {
	if r, ok := rank[strings.ToLower(name)]; ok {
		level = r
		return
	}
	level = rank["info"]
}

// Logf writes a leveled line. log.Output(2, ...) keeps the caller's file:line.
func Logf(lvl, format string, args ...any) {
	if rank[lvl] < level {
		return
	}
	msg := fmt.Sprintf(format, args...)
	log.Output(2, fmt.Sprintf("[%-5s] %s", strings.ToUpper(lvl), msg))
}
