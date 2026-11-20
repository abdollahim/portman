package logging

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
)

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		SetLevel("info")
	})
	return &buf
}

func TestLevelFiltering(t *testing.T) {
	buf := captureLog(t)
	SetLevel("warn")

	Logf("info", "hidden message")
	Logf("warn", "shown message")

	out := buf.String()
	if strings.Contains(out, "hidden message") {
		t.Fatalf("info line should be filtered at warn level: %q", out)
	}
	if !strings.Contains(out, "[WARN ] shown message") {
		t.Fatalf("warn line missing: %q", out)
	}
}

func TestUnknownLevelFallsBackToInfo(t *testing.T) {
	buf := captureLog(t)
	SetLevel("bogus")

	Logf("debug", "debug hidden")
	Logf("info", "info shown")

	out := buf.String()
	if strings.Contains(out, "debug hidden") {
		t.Fatalf("debug should be filtered at info level: %q", out)
	}
	if !strings.Contains(out, "info shown") {
		t.Fatalf("info line missing: %q", out)
	}
}
