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

func TestPrivateKeyIsRedacted(t *testing.T) {
	buf := captureLog(t)
	SetLevel("info")

	Logf("info", "loaded key:\n-----BEGIN EC PRIVATE KEY-----\nMHcCAQEEIFakeKeyMaterial\n-----END EC PRIVATE KEY-----\ndone")

	out := buf.String()
	if strings.Contains(out, "MHcCAQEEIFakeKeyMaterial") || strings.Contains(out, "PRIVATE KEY") {
		t.Fatalf("private key leaked into log: %q", out)
	}
	if !strings.Contains(out, "[redacted private key]") {
		t.Fatalf("expected placeholder in log: %q", out)
	}
	if !strings.Contains(out, "done") {
		t.Fatalf("text after the key should be kept: %q", out)
	}
}

func TestSecretIsRedacted(t *testing.T) {
	buf := captureLog(t)
	SetLevel("info")
	AddSecret("123456789:AAFakeBotTokenValue")

	Logf("error", "request to https://example.test/bot123456789:AAFakeBotTokenValue/sendMessage failed")

	out := buf.String()
	if strings.Contains(out, "AAFakeBotTokenValue") {
		t.Fatalf("secret leaked into log: %q", out)
	}
	if !strings.Contains(out, "[redacted]") {
		t.Fatalf("expected placeholder in log: %q", out)
	}
}
