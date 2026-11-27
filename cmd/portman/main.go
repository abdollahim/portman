package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"portman/internal/checks"
	"portman/internal/config"
	"portman/internal/logging"
	"portman/internal/report"
	"portman/internal/stats"
	"portman/internal/tunnel"
)

// Set at build time by release.ps1 via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// versionString returns the build identity printed by -v and -version.
func versionString() string {
	return fmt.Sprintf("portman %s (commit %s, built %s)", version, commit, date)
}

func main() {
	configPath := flag.String("config", "config.json", "path to config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	showVersionShort := flag.Bool("v", false, "print version and exit (same as -version)")
	reportNow := flag.Bool("report-now", false, "send one traffic report now and exit (inside server)")
	checksNow := flag.Bool("checks-now", false, "run the connectivity check now, send it, and exit (inside server)")
	flag.Parse()

	if *showVersion || *showVersionShort {
		fmt.Println(versionString())
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	// New log files are created with 0640: readable by the owner and group only.
	logFile, err := os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()

	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	logging.SetLevel(cfg.LogLevel)
	logging.AddSecret(cfg.Notify.BotToken)

	mode := "client"
	if cfg.IsServer() {
		mode = "server"
	}
	logging.Logf("info", "starting portman %s (commit %s, built %s) in %s mode, log level %s",
		version, commit, date, mode, strings.ToLower(cfg.LogLevel))

	warnIfExposed(*configPath)
	warnIfExposed(cfg.LogPath)
	warnIfExposed(cfg.KeyFile)
	warnIfExposed(cfg.ClientKeyFile)

	if cfg.IsServer() {
		runServer(cfg)
		return
	}
	runClient(cfg, *reportNow, *checksNow)
}

func runServer(cfg config.Config) {
	if store := openStats(cfg); store != nil {
		tunnel.SetRecorder(store)
		go sampleLoop(cfg, store)
		if cfg.Stats.ListenAddr != "" {
			go serveStats(cfg, store)
		}
	}
	go tunnel.StartupChecks(cfg)
	tunnel.RunServer(cfg)
}

func runClient(cfg config.Config, reportNow, checksNow bool) {
	store := openStats(cfg)

	if reportNow {
		if store == nil || !cfg.Notify.Enabled {
			fmt.Fprintln(os.Stderr, "-report-now needs stats.enabled and notify.enabled in the config")
			os.Exit(1)
		}
		if err := report.SendDaily(cfg, store, time.Now()); err != nil {
			logging.Logf("error", "report failed: %v", err)
			os.Exit(1)
		}
		return
	}

	if checksNow {
		if !cfg.Checks.Enabled {
			fmt.Fprintln(os.Stderr, "-checks-now needs checks.enabled in the config")
			os.Exit(1)
		}
		if err := checks.RunOnce(cfg, time.Now()); err != nil {
			logging.Logf("error", "connectivity check failed: %v", err)
			os.Exit(1)
		}
		return
	}

	if store != nil {
		tunnel.SetRecorder(store)
		go sampleLoop(cfg, store)
	}
	if cfg.Notify.Enabled {
		go report.RunDaily(cfg, store)
	}
	if cfg.Checks.Enabled {
		go checks.RunScheduled(cfg)
	}
	go tunnel.StartupChecks(cfg)
	tunnel.RunClient(cfg)
}

func openStats(cfg config.Config) *stats.Store {
	if !cfg.Stats.Enabled {
		return nil
	}
	loc, err := cfg.Stats.Location()
	if err != nil {
		log.Fatalf("stats timezone: %v", err)
	}
	store, err := stats.Open(cfg.Stats.StatePath, cfg.Stats.Interface, loc)
	if err != nil {
		log.Fatalf("open stats state: %v", err)
	}
	store.SetBillingDay(cfg.Stats.BillingDay)
	return store
}

// sampleLoop reads the interface counters and saves the state file on each tick.
func sampleLoop(cfg config.Config, store *stats.Store) {
	if err := store.Sample(time.Now()); err != nil {
		logging.Logf("warn", "stats sample failed: %v", err)
	}
	ticker := time.NewTicker(cfg.Stats.Every())
	defer ticker.Stop()
	for now := range ticker.C {
		if err := store.Sample(now); err != nil {
			logging.Logf("warn", "stats sample failed: %v", err)
		}
		if err := store.Save(); err != nil {
			logging.Logf("warn", "stats save failed: %v", err)
		}
	}
}

// serveStats runs the foreign server's stats endpoint. Failure here logs an
// error but doesn't stop the bridge.
func serveStats(cfg config.Config, store *stats.Store) {
	tlsCfg, err := tunnel.NewServerTLSConfig(cfg)
	if err != nil {
		logging.Logf("error", "stats endpoint not started: %v", err)
		return
	}
	ln, err := net.Listen("tcp", cfg.Stats.ListenAddr)
	if err != nil {
		logging.Logf("error", "stats endpoint not started: %v", err)
		return
	}
	logging.Logf("info", "stats endpoint listening on %s (client certificate required)", cfg.Stats.ListenAddr)

	provide := func(now time.Time) report.Payload {
		return report.LocalPayload(cfg, store, now)
	}
	if err := report.ServeStats(ln, tlsCfg, provide); err != nil {
		logging.Logf("error", "stats endpoint stopped: %v", err)
	}
}

// warnIfExposed logs a warning when a file is readable by users other than its owner
// and group. Windows file modes don't map to this check, so it's skipped there.
func warnIfExposed(path string) {
	if path == "" || runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if perm := info.Mode().Perm(); perm&0o007 != 0 {
		logging.Logf("warn", "%s is readable by other users (mode %o); use chmod 0640 or stricter", path, perm)
	}
}
