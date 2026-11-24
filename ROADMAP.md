# Roadmap
See [ROADMAP.md](ROADMAP.md) for planned versions.

This file lists planned work. Each item becomes a release when it's finished, tested, and recorded in [CHANGELOG.md](CHANGELOG.md). Items can change order or scope.

## Done

- **0.1.0**: TLS tunnel, server and client modes, logging, startup checks, config validation.
- **0.2.0**: Dial retry, timing logs, `-version` flag, unit and integration tests, release script with test gate.
- **0.3.0**: Optional mutual TLS. The server requires client certificates when `client_ca_file` is set, and existing configs keep working unchanged.
- **0.4.0**: Connection limit, idle timeout, log file permissions with startup warnings, private key redaction in logs.
- **0.5.0**: Daily traffic report to Bale covering both servers, per-day counters, monthly limit tracking.

## Next

### 0.6.0: Release trust and operations

Planned:
- Signed release files (for example with `cosign` or `minisign`), with the verification steps in the README.
- A `-check-config` flag that validates a config and reports problems without starting the service.
- Clearer startup messages for the most common mistakes (wrong port, certificate name mismatch, unreadable key).

### 0.7.0: Connection reuse

Problem: each local connection pays a new TLS handshake, which adds about 250 ms on a long link.

Planned:
- Keep a small pool of warm TLS connections to the server and reuse them for new local connections.
- Idle connections in the pool close after a timeout.
- Measured before and after with the `dial took` log lines, so the change is justified by numbers.

## Later

- Multiple backends or multiple bridges per client, with failover.
- Native Windows service support.
- Per-user statistics in the daily report.

## Criteria for 1.0.0

Version 1.0.0 is for when the project meets all of these:

- Client authentication and hardening are in place and tested.
- The bridge has run for a sustained period on real traffic without a crash or a stuck connection.
- The configuration format is stable, and changes are documented in the changelog with a migration path.
- The security model and its limits are documented in the README.
- The release files are signed and the verification steps are documented.