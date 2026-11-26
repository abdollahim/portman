// Package report builds the daily traffic report, sends it to Bale, and serves
// this server's counters to the other side over mutual TLS.
package report

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"portman/internal/config"
	"portman/internal/logging"
	"portman/internal/stats"
	"portman/internal/tunnel"
)

const gib = 1024 * 1024 * 1024

// Payload is one server's numbers. It goes into the report and is served at /stats.
type Payload struct {
	Label     string         `json:"label"`
	LimitGB   float64        `json:"monthly_limit_gb"`
	CountMode string         `json:"count_mode"`
	Snapshot  stats.Snapshot `json:"snapshot"`
}

// LocalPayload samples this server's interface and returns its numbers.
func LocalPayload(cfg config.Config, store *stats.Store, now time.Time) Payload {
	if err := store.Sample(now); err != nil {
		logging.Logf("warn", "stats sample failed: %v", err)
	}
	return Payload{
		Label:     cfg.Stats.Label,
		LimitGB:   cfg.Stats.MonthlyLimitGB,
		CountMode: cfg.Stats.Mode(),
		Snapshot:  store.Snapshot(now),
	}
}

// ServeStats answers GET /stats over TLS on ln. tlsCfg must require client certificates.
func ServeStats(ln net.Listener, tlsCfg *tls.Config, provide func(time.Time) Payload) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(provide(time.Now()))
	})
	srv := &http.Server{
		Handler:           mux,
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.ServeTLS(ln, "", "")
}

// FetchForeign asks the foreign server for its numbers over mutual TLS.
func FetchForeign(cfg config.Config, timeout time.Duration) (Payload, error) {
	tc, err := tunnel.NewClientTLSConfig(cfg)
	if err != nil {
		return Payload{}, err
	}
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig:   tc,
			DisableKeepAlives: true,
		},
	}
	resp, err := client.Get("https://" + cfg.Notify.ForeignStatsAddr + "/stats")
	if err != nil {
		return Payload{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Payload{}, fmt.Errorf("stats endpoint returned HTTP %d", resp.StatusCode)
	}
	var p Payload
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&p); err != nil {
		return Payload{}, fmt.Errorf("decode stats: %w", err)
	}
	return p, nil
}

// BuildMessage formats the report as plain text, which needs no markup.
func BuildMessage(date, zone string, local Payload, foreign *Payload, foreignErr error) string {
	var b strings.Builder
	fmt.Fprintf(&b, "portman daily report\n%s (%s)\n\n", date, zone)
	writeSection(&b, local)
	b.WriteString("\n")
	switch {
	case foreign != nil:
		writeSection(&b, *foreign)
	case foreignErr != nil:
		fmt.Fprintf(&b, "[foreign] not reachable: %v\n", foreignErr)
	}
	return b.String()
}

func writeSection(b *strings.Builder, p Payload) {
	s := p.Snapshot
	fmt.Fprintf(b, "[%s] interface %s\n", p.Label, s.Interface)
	fmt.Fprintf(b, "  Bridge today: %d connections, %s up, %s down\n",
		s.Today.BridgeConns, HumanBytes(s.Today.BridgeUp), HumanBytes(s.Today.BridgeDown))
	fmt.Fprintf(b, "  Interface today: %s received, %s sent\n",
		HumanBytes(s.Today.NICRx), HumanBytes(s.Today.NICTx))
	fmt.Fprintf(b, "  Interface yesterday (billable): %s\n", HumanBytes(s.Yesterday.Billable(p.CountMode)))

	used := s.Month.Billable(p.CountMode)
	if p.LimitGB > 0 {
		fmt.Fprintf(b, "  Cycle to date (since %s): %s of %.0f GB (%.1f%%)\n",
			s.CycleStart, HumanBytes(used), p.LimitGB, float64(used)/(p.LimitGB*gib)*100)
	} else {
		fmt.Fprintf(b, "  Cycle to date (since %s): %s (no limit set)\n", s.CycleStart, HumanBytes(used))
	}
}

// HumanBytes formats n with binary units, for example 1.5 KiB.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n/div >= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// SendDaily collects both servers and sends one report to Bale.
// It doesn't save the state file: the sample loop owns that file.
func SendDaily(cfg config.Config, store *stats.Store, now time.Time) error {
	if !cfg.Notify.Enabled {
		return errors.New("notify is disabled in the config")
	}
	loc, err := cfg.Stats.Location()
	if err != nil {
		return err
	}

	local := LocalPayload(cfg, store, now)

	var foreign *Payload
	var foreignErr error
	if cfg.Notify.ForeignStatsAddr != "" {
		p, err := FetchForeign(cfg, 20*time.Second)
		if err != nil {
			foreignErr = err
			logging.Logf("warn", "foreign stats unavailable: %v", err)
		} else {
			foreign = &p
		}
	}

	date := now.In(loc).Format("2006-01-02")
	msg := BuildMessage(date, loc.String(), local, foreign, foreignErr)

	bale := Bale{
		BaseURL:  cfg.Notify.BaleBase(),
		Token:    cfg.Notify.BotToken,
		ChatID:   cfg.Notify.ChatID,
		HTTP:     &http.Client{Timeout: 20 * time.Second},
		Attempts: 3,
		Backoff:  5 * time.Second,
	}
	if err := bale.Send(msg); err != nil {
		return fmt.Errorf("send report: %w", err)
	}
	logging.Logf("info", "daily report sent for %s", date)
	return nil
}

// RunDaily sends the report every day at notify.send_at, in the stats timezone.
func RunDaily(cfg config.Config, store *stats.Store) {
	loc, err := cfg.Stats.Location()
	if err != nil {
		logging.Logf("error", "daily report disabled: %v", err)
		return
	}
	hour, minute, err := cfg.Notify.Clock()
	if err != nil {
		logging.Logf("error", "daily report disabled: %v", err)
		return
	}
	for {
		next := NextRun(time.Now(), hour, minute, loc)
		logging.Logf("info", "next daily report at %s", next.Format(time.RFC3339))
		time.Sleep(time.Until(next))
		if err := SendDaily(cfg, store, time.Now()); err != nil {
			logging.Logf("error", "daily report failed: %v", err)
		}
	}
}

// NextRun returns the first hour:minute in loc that is after now.
func NextRun(now time.Time, hour, minute int, loc *time.Location) time.Time {
	local := now.In(loc)
	next := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, loc)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
