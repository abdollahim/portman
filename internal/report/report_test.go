package report

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"portman/internal/stats"
)

var zone = time.FixedZone("TEST", 3*3600+1800)

func at(day, hour, minute int) time.Time {
	return time.Date(2026, time.November, day, hour, minute, 0, 0, zone)
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{0: "0 B", 1536: "1.5 KiB", 1 << 30: "1.0 GiB"}
	for in, want := range cases {
		if got := HumanBytes(in); got != want {
			t.Fatalf("HumanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestNextRun(t *testing.T) {
	cases := []struct{ now, want time.Time }{
		{at(24, 10, 0), at(24, 23, 55)},
		{at(24, 23, 56), at(25, 23, 55)},
		{at(24, 23, 55), at(25, 23, 55)},
	}
	for _, c := range cases {
		if got := NextRun(c.now, 23, 55, zone); !got.Equal(c.want) {
			t.Fatalf("NextRun(%v) = %v, want %v", c.now, got, c.want)
		}
	}
}

func TestBuildMessageShowsBothSides(t *testing.T) {
	local := Payload{Label: "inside", LimitGB: 50, CountMode: "sum", Snapshot: stats.Snapshot{Interface: "eth0"}}

	msg := BuildMessage("2026-11-24", "Asia/Tehran", local, nil, errors.New("timeout"))
	for _, want := range []string{"[inside]", "not reachable: timeout", "Month to date"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q:\n%s", want, msg)
		}
	}

	foreign := Payload{Label: "foreign", CountMode: "sum", Snapshot: stats.Snapshot{Interface: "eth0"}}
	msg = BuildMessage("2026-11-24", "Asia/Tehran", local, &foreign, nil)
	if !strings.Contains(msg, "[foreign]") || strings.Contains(msg, "not reachable") {
		t.Fatalf("foreign section missing or wrong:\n%s", msg)
	}
}

func TestBaleSendsToTokenPath(t *testing.T) {
	var path string
	var body map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	b := Bale{BaseURL: srv.URL, Token: "123:TOKEN", ChatID: "42", HTTP: srv.Client(), Attempts: 1}
	if err := b.Send("hello"); err != nil {
		t.Fatal(err)
	}
	if path != "/bot123:TOKEN/sendMessage" {
		t.Fatalf("path = %q", path)
	}
	if body["chat_id"] != "42" || body["text"] != "hello" {
		t.Fatalf("body = %v", body)
	}
}

func TestBaleRetriesThenSucceeds(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"ok":false,"description":"busy"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	b := Bale{BaseURL: srv.URL, Token: "123:TOKEN", ChatID: "42", HTTP: srv.Client(), Attempts: 3}
	if err := b.Send("x"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestBaleGivesUp(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"ok":false,"description":"down"}`))
	}))
	defer srv.Close()

	b := Bale{BaseURL: srv.URL, Token: "123:TOKEN", ChatID: "42", HTTP: srv.Client(), Attempts: 2}
	if err := b.Send("x"); err == nil {
		t.Fatal("expected an error after all attempts fail")
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}
