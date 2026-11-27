# Roadmap

This file lists planned work. Each item becomes a release when it's finished, tested, and recorded in [CHANGELOG.md](CHANGELOG.md). Items can change order or scope.

## Done

- **0.1.0**: TLS tunnel, server and client modes, logging, startup checks, config validation.
- **0.2.0**: Dial retry, timing logs, `-version` flag, unit and integration tests, release script with test gate.
- **0.3.0**: Optional mutual TLS. The server requires client certificates when `client_ca_file` is set, and existing configs keep working unchanged.
- **0.4.0**: Connection limit, idle timeout, log file permissions with startup warnings, private key redaction in logs.
- **0.5.0**: Daily traffic report to Bale covering both servers, per-day counters, monthly limit tracking.
- **0.5.1**: Examples renamed to `examples/`, service units create their state and log directories, README setup guide, stats endpoint on port 8444.
- **0.6.0**: Billing cycle reset day (`stats.billing_day`, 1 to 28), so monthly totals start on the provider's reset day.
- **0.6.1**: `billing_day` accepts 1 to 31. In shorter months, the cycle starts on the last day.
- **0.6.2**: Billing settings documented per server, with a test that each server keeps its own cycle.
- **0.7.0**: Connectivity checks: scheduled ping and DNS report through the bot, targets in a JSON file, `-checks-now`, `checks` config section.

## Next

### 0.8.0: Per-category accounting

Problem: the report shows totals for the whole bridge, not for each device or group.

Planned:
- Each device or group gets its own bridge UUID and its own outbound in the inside 3X-UI.
- The client reads the VLESS header at the start of each tunnel connection (the version byte and the 16-byte UUID), looks the UUID up in `stats.categories`, and counts the bytes under that label. Connections with an unknown UUID go into an `unknown` bucket.
- Raw UUIDs never appear in logs or reports. Only labels do.
- The report shows per-category totals for the day and the billing cycle, on each server that counts them.
- Tests: header parsing, unknown UUID handling, and a check that logs contain labels, not UUIDs.

This is the first version where Portman reads a protocol header, so the README's "byte pipe" description will need to change.

### 0.9.0: Release trust and operations

Planned:
- Signed release files (for example with `cosign` or `minisign`), with verification steps in the README.
- A `-check-config` flag that validates a config and reports problems without starting the service.
- Clearer startup messages for the most common mistakes (wrong port, certificate name mismatch, unreadable key).

### 0.10.0: Connection reuse

Problem: each local connection pays a new TLS handshake, which adds about 250 ms on a long link.

Planned:
- Keep a small pool of warm TLS connections to the server and reuse them for new local connections.
- Idle connections in the pool close after a timeout.
- Measured before and after with the `dial took` log lines, so the change is justified by numbers.

## Later

- Ping-only checks per group, so resolvers that refuse outside DNS queries don't show as `PARTIAL`.
- A fallback transport for the stats endpoint (over the bridge port or 443 under a path) if a port is blocked on the network.
- Multiple backends or multiple bridges per client, with failover.
- Native Windows service support.
- A Bale or Telegram alert when a server stops reporting.

## Criteria for 1.0.0

Version 1.0.0 is for when the project meets all of these:

- Client authentication and hardening are in place and tested.
- The bridge has run for a sustained period on real traffic without a crash or a stuck connection.
- The configuration format is stable, and changes are documented in the changelog with a migration path.
- The security model and its limits are documented in the README.
- The release files are signed and the verification steps are documented.