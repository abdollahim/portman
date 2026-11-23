# Roadmap
See [ROADMAP.md](ROADMAP.md) for planned versions.

This file lists planned work. Each item becomes a release when it's finished, tested, and recorded in [CHANGELOG.md](CHANGELOG.md). Items can change order or scope.

## Done

- **0.1.0**: TLS tunnel, server and client modes, logging, startup checks, config validation.
- **0.2.0**: Dial retry, timing logs, `-version` flag, unit and integration tests, release script with test gate.

## Next

### 0.3.0: Client authentication (mutual TLS)

Problem: anyone who can reach the server's port can open a TLS connection and send data to the backend.

Planned:
- The server requires a client certificate signed by a private CA that you control (`client_ca_file` in the server config).
- The client presents its own certificate and key (`client_cert_file`, `client_key_file`).
- A small helper, documented in the README, creates the CA and client certificates.
- Tests: a client without a certificate is rejected, a certificate from another CA is rejected, a valid certificate connects.

This changes how the server accepts connections, so it's a minor bump under the pre-1.0 rules. The changelog will list the new config fields and the migration steps.

### 0.4.0: Hardening

Planned:
- Maximum concurrent connections per server (configurable, with a sensible default).
- Idle timeout for relayed connections, so stalled connections are closed.
- Log file created with mode `0640`, and a startup warning if an existing log or config file is readable by other users.
- Log lines never include secrets (tokens, UUIDs, certificate keys). This is checked by a test.

### 0.5.0: Daily traffic report to Telegram

Goal: a configurable notifier that sends a message to a Telegram bot at a fixed time each day, reporting traffic that passed through the bridge during that day.

Planned:
- Byte and connection counters per day, kept on the server, with totals saved to a small state file so a restart doesn't reset the day.
- A `notify` section in the config: enabled flag, time of day to send, timezone, bot token file, chat ID. The token is read from a file with mode `0600`, never put inline in the config and never logged.
- Retries on send failure, and a log line when a send fails.
- Message format to be decided when this version starts. It may include bytes in each direction, connection count, and comparison with the previous day.

Before this version starts, confirm that the server running the notifier can reach the Telegram API. In some networks it's blocked, and the notifier would then need to run on a different host or through another route.

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