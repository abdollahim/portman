# Changelog

All notable changes to this project are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.5.0] - 2026-11-24

### Added
- Daily traffic report sent to a Bale bot from the inside server at `notify.send_at`.
- Per-day counters on both servers: interface traffic (what providers bill) and bridge traffic (tunnel only), saved to a state file that survives restarts.
- Foreign server serves its counters over mutual TLS at `stats.listen_addr`. The endpoint is refused unless `client_ca_file` is set.
- `monthly_limit_gb` and `count_mode` show month-to-date usage against a limit.
- `-report-now` sends one report and exits.
- Registered secrets (the bot token) are redacted from all log lines and errors.

## [0.4.0] - 2026-11-24

### Added
- `max_connections` (default 256): caps concurrent connections per process. Extra connections are dropped with a warning.
- `idle_timeout` (default `5m`): closes a relayed connection after no data has moved in either direction for this long.
- Startup warning when a config, log, or private key file is readable by users other than its owner and group.
- Private key blocks are redacted from log lines.
- Tests for the connection limit, the idle timeout, the config defaults and validation, and log redaction.

### Changed
- New log files are created with mode `0640` (was `0644`).
- When one direction of a relay ends, the other gets the drain timeout to finish, then both sides are closed. The result is the same as before, but the implementation no longer relies on connection deadlines.
- Connections closed by the idle timeout or by the drain timeout are logged at info or debug level, not as warnings.

## [0.3.0] - 2026-11-23

### Added
- ROADMAP.md with planned versions: client authentication, hardening, daily Telegram report, release trust, connection reuse.
- Optional mutual TLS. On the server, `client_ca_file` requires clients to present a certificate signed by that CA. On the client, `client_cert_file` and `client_key_file` supply the certificate.
- Config validation for the new fields: the certificate and key must be set together, and each field is accepted only in the mode that uses it.
- Tests for mutual TLS: a valid client is accepted, and a client with no certificate or with a certificate from another CA is rejected.

### Changed
- README updated for v0.2.0: current build and config paths, service templates, tests, reliability and logging notes.
- README: example setup for both servers, including portman configs and 3X-UI inbound, outbound, and routing steps.
- README: user-facing security section covering firewall rules, loopback backend, bridge identity, unprivileged service, file permissions, panel and SSH hardening, and checksum verification.

## [0.2.0] - 2026-11-20

### Added
- (keep the items you listed under Unreleased → Added)
- `-version` flag prints the version, commit, and build time.

### Changed
- (keep the items you listed under Unreleased → Changed)

### Notes
- Tested end to end with one inside/foreign pair. Still experimental.

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