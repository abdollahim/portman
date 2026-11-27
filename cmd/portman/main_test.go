package main

import (
	"strings"
	"testing"
)

func TestVersionString(t *testing.T) {
	s := versionString()
	if !strings.HasPrefix(s, "portman ") || !strings.Contains(s, "(commit ") {
		t.Fatalf("unexpected version string: %q", s)
	}
}
