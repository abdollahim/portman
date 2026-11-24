package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"

	"portman/internal/config"
	"portman/internal/logging"
	"portman/internal/tunnel"
)

// Set at build time by release.ps1 via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	configPath := flag.String("config", "config.json", "path to config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("portman %s (commit %s, built %s)\n", version, commit, date)
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

	go tunnel.StartupChecks(cfg)

	if cfg.IsServer() {
		tunnel.RunServer(cfg)
	} else {
		tunnel.RunClient(cfg)
	}
}

// warnIfExposed logs a warning when a file is readable by users other than
// its owner and group. Windows file modes don't map to this check, so it's skipped there.
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
