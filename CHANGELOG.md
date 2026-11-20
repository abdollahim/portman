# Changelog

All notable changes to this project are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added
- Unit tests for config loading and validation, and for logging levels.
- Integration test for the server: TLS handshake, forwarding to a backend, and an echo round trip.
- `release.ps1` runs `go test` before building.
- Versioning section in the README.

## [0.1.0] - 2026-11-20

### Added
- TLS client and server modes, selected by config.
- Per-connection IDs and per-direction byte counts in the logs.
- Startup checks for the remote TCP port and TLS handshake (log only).
- Config validation with clear error messages.
- Half-close handling and a drain timeout, so one finished direction does not hang the other.
- Minimum TLS 1.2 on both sides.
- `-version` flag prints the version, commit, and build time.

### Changed
- Client retries the TLS connection up to 3 times, waiting 1s and then 2s, before it drops a local connection.
- Logs include microsecond timestamps, the local port used for each tunnel, and the time each TLS dial took.