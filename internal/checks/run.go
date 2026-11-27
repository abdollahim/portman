package checks

import (
	"fmt"
	"net/http"
	"time"

	"portman/internal/config"
	"portman/internal/logging"
	"portman/internal/report"
)

// RunOnce probes every target, builds the report, and sends it to the bot.
// The target file is read on every run, so edits take effect without a restart.
func RunOnce(cfg config.Config, now time.Time) error {
	tf, err := LoadTargets(cfg.Checks.TargetsFile)
	if err != nil {
		return err
	}
	loc, err := cfg.Checks.Location()
	if err != nil {
		return err
	}

	results := RunProbes(tf, DefaultProber(), 8)

	label := cfg.Stats.Label
	if label == "" {
		label = "portman"
	}
	msg := BuildReport(label, now.In(loc), loc.String(), results)

	bot := report.Bale{
		BaseURL:  cfg.Notify.BaleBase(),
		Token:    cfg.Notify.BotToken,
		ChatID:   cfg.Notify.ChatID,
		HTTP:     &http.Client{Timeout: 20 * time.Second},
		Attempts: 3,
		Backoff:  5 * time.Second,
	}
	if err := bot.Send(msg); err != nil {
		return fmt.Errorf("send connectivity report: %w", err)
	}
	logging.Logf("info", "connectivity report sent: %d targets", len(results))
	return nil
}

// RunScheduled sends the report on each configured weekday at checks.send_at.
func RunScheduled(cfg config.Config) {
	days, err := cfg.Checks.Weekdays()
	if err != nil {
		logging.Logf("error", "connectivity checks disabled: %v", err)
		return
	}
	hour, minute, err := cfg.Checks.ClockAt()
	if err != nil {
		logging.Logf("error", "connectivity checks disabled: %v", err)
		return
	}
	loc, err := cfg.Checks.Location()
	if err != nil {
		logging.Logf("error", "connectivity checks disabled: %v", err)
		return
	}

	for {
		next := NextCheck(time.Now(), days, hour, minute, loc)
		logging.Logf("info", "next connectivity check at %s", next.Format(time.RFC3339))
		time.Sleep(time.Until(next))
		if err := RunOnce(cfg, time.Now()); err != nil {
			logging.Logf("error", "connectivity check failed: %v", err)
		}
	}
}

// NextCheck returns the first time after now that falls on one of days at hour:minute in loc.
func NextCheck(now time.Time, days []time.Weekday, hour, minute int, loc *time.Location) time.Time {
	local := now.In(loc)
	for i := 0; i <= 7; i++ {
		candidate := time.Date(local.Year(), local.Month(), local.Day()+i, hour, minute, 0, 0, loc)
		if !candidate.After(now) {
			continue
		}
		for _, d := range days {
			if candidate.Weekday() == d {
				return candidate
			}
		}
	}
	return now.Add(24 * time.Hour) // unreachable when days is non-empty
}
