# Portman

A small TLS tunnel that forwards TCP traffic between two servers.

A client listens on a local port, wraps each incoming connection in TLS, and sends it to a server. The server completes the TLS handshake and forwards the decrypted stream to a local backend such as Xray or 3X-UI.

Portman does not parse the traffic. It is a byte pipe, so the backend must accept exactly what the client sends. For example, a VLESS outbound on the client side needs a VLESS inbound on the backend, with a matching identity.

## Status

Experimental (v0.2.0). Tested end to end with one client and one server, carrying real traffic. It has not been tested under production load or with multiple clients.

## Versioning

Releases use [Semantic Versioning](https://semver.org/) and are tagged `vX.Y.Z`. Until 1.0.0, a minor version may change configuration or behavior. See [CHANGELOG.md](CHANGELOG.md) for what changed in each release.

Check the installed version with:

```bash
portman -version
```

## Requirements

- Go 1.21 or newer, to build from source
- A hostname with a valid certificate on the server (Let's Encrypt works)
- Linux with systemd, if you want to run it as a service

## Build

Build the binary into `dist/`:

```bash
go build -o dist/portman ./cmd/portman
```

`release.ps1` (Windows PowerShell) runs the vet, format, and test checks, then builds Linux and Windows binaries with the version and commit embedded, and writes SHA-256 checksums.

## Tests

```bash
go test ./...
```

The tests cover config loading and validation, log levels, the server's TLS forwarding with an echo backend, and the client's dial retry.

## Configuration

Portman reads one JSON file, passed with `-config`. Mode is chosen by the `mode` field, or inferred from `cert_file` if `mode` is empty.

| Field | Used by | Description |
|---|---|---|
| `mode` | both | `server` or `client` |
| `listen_addr` | both | Address to listen on |
| `remote_addr` | client | Server to connect to, `host:port` |
| `server_name` | client | TLS server name; must match the certificate |
| `backend_addr` | server | Local backend to forward to, `host:port` |
| `cert_file`, `key_file` | server | TLS certificate and key |
| `client_ca_file` | server | CA certificate (PEM). When set, clients must present a certificate signed by it |
| `client_cert_file`, `client_key_file` | client | Client certificate and key (PEM). Set both or neither |
| `max_connections` | both | Maximum concurrent connections per process (default 256). Extra connections are dropped |
| `idle_timeout` | both | Close a connection after this long with no data in either direction (default `5m`, for example `30m`) |
| `log_level` | both | `debug`, `info`, `warn`, or `error` |
| `log_path` | both | Log file; output also goes to stdout |
| `stats.enabled` | both | Count traffic per day and save it to `stats.state_path` |
| `stats.label` | both | Name shown in the report, for example `inside` |
| `stats.state_path` | both | JSON state file, for example `/var/lib/portman/state.json` |
| `stats.interface` | both | Network interface to count, for example `eth0` |
| `stats.timezone` | both | Day and month boundaries, for example `Asia/Tehran` |
| `stats.monthly_limit_gb` | both | Monthly limit in GiB; `0` means no limit line |
| `stats.count_mode` | both | `sum` (in + out) or `max` (the larger of the two) |
| `stats.sample_interval` | both | How often counters are read, for example `1m` |
| `stats.listen_addr` | server | Where the stats endpoint listens, for example `:4444` |
| `notify.enabled` | client | Send the daily report |
| `notify.api_base` | client | Bale API base; default `https://tapi.bale.ai` |
| `notify.bot_token` | client | Bale bot token |
| `notify.chat_id` | client | Chat the report is sent to |
| `notify.send_at` | client | Send time, `HH:MM`, in `stats.timezone` |
| `notify.foreign_stats_addr` | client | Foreign stats endpoint, for example `bridge.example.com:4444` |

Examples are in `configs/server.example.json` and `configs/client.example.json`. Copy one and edit it. Don't commit your real config.

## Example setup

This example uses two servers:

- **Inside server:** where your devices connect. It runs 3X-UI and a portman client.
- **Foreign server:** where traffic exits to the internet. It runs 3X-UI and a portman server.

```
phone -> inside 3X-UI inbound -> outbound "portman" -> portman client (127.0.0.1:20001)
      -> TLS -> portman server (FOREIGN_IP:4443) -> foreign 3X-UI inbound (127.0.0.1:10000) -> internet
```

### Prerequisites

- A hostname for the bridge, for example `bridge.example.com`, with a **DNS-only** A record pointing to the foreign server's IP. If the record is proxied through Cloudflare, the bridge port won't work, because Cloudflare only forwards web ports.
- A valid certificate for that hostname on the foreign server. Let's Encrypt works. The certificate's name must match `server_name` in the client config.
- Port 4443 open on the foreign server, and only from the inside server's IP if you can restrict it.

Replace these placeholders everywhere:

| Placeholder | Meaning |
|---|---|
| `bridge.example.com` | Bridge hostname (DNS-only, certificate name) |
| `FOREIGN_IP` | Public IP of the foreign server |
| `YOUR-BRIDGE-UUID` | A new UUID you create, used only for the bridge |

Generate a UUID with `uuidgen` on Linux, or `[guid]::NewGuid()` in PowerShell.

### Portman server config (foreign server)

`/etc/portman/server.json`:

```json
{
  "mode": "server",
  "listen_addr": ":4443",
  "backend_addr": "127.0.0.1:10000",
  "cert_file": "/etc/letsencrypt/live/bridge.example.com/fullchain.pem",
  "key_file": "/etc/letsencrypt/live/bridge.example.com/privkey.pem",
  "log_level": "info",
  "log_path": "/var/log/portman/portman.log"
}
```

### Portman client config (inside server)

`/etc/portman/client.json`:

```json
{
  "mode": "client",
  "listen_addr": "127.0.0.1:20001",
  "remote_addr": "bridge.example.com:4443",
  "server_name": "bridge.example.com",
  "log_level": "info",
  "log_path": "/var/log/portman/portman.log"
}
```

Set `log_level` to `debug` only while testing.

### 3X-UI on the foreign server: bridge inbound

Add a new inbound. It only listens on localhost, so the internet can't reach it directly.

| Field | Value |
|---|---|
| Protocol | VLESS |
| Listen IP | `127.0.0.1` |
| Port | `10000` |
| Network | tcp |
| Security | none |

Add one client:

| Field | Value |
|---|---|
| ID | `YOUR-BRIDGE-UUID` |
| Flow | empty |
| Email / remark | `portman-bridge` |

Keep this UUID separate from the UUIDs you give your devices. Anyone with it can use the foreign server through the bridge.

### 3X-UI on the inside server: outbound and routing

**1. Add the outbound.** In **Xray Configs → Outbounds**, add:

| Field | Value |
|---|---|
| Protocol | VLESS |
| Tag | `portman` |
| Address | `127.0.0.1` |
| Port | `20001` |
| ID | `YOUR-BRIDGE-UUID` |
| Encryption | none |
| Network | tcp |
| Security | none |
| Mux | off (for the first test) |

For reference, the outbound in JSON looks like this:

```json
{
  "tag": "portman",
  "protocol": "vless",
  "settings": {
    "vnext": [
      {
        "address": "127.0.0.1",
        "port": 20001,
        "users": [
          { "id": "YOUR-BRIDGE-UUID", "encryption": "none" }
        ]
      }
    ]
  }
}
```

**2. Add a routing rule.** In **Routing**, send the traffic you want through the bridge to the `portman` outbound. For example, if your devices use an inbound named `phone-in`:

```json
{
  "type": "field",
  "inboundTag": ["phone-in"],
  "outboundTag": "portman"
}
```

Keep the device's existing direct outbound for everything else. For testing, use a separate inbound for the bridge, so a broken bridge doesn't cut off your normal connection.

**3. Restart Xray** from the 3X-UI panel.

### Verify each hop

On the foreign server, check that the bridge port listens on localhost only:

```bash
sudo ss -tlnp | grep 10000
```

Expected: `127.0.0.1:10000`.

On the inside server, confirm the client is up and forwarding:

```bash
sudo journalctl -u portman-client -n 20 --no-pager
```

Expected: `client listening on 127.0.0.1:20001`, and after a connection, `tunnel up` followed by `closed after`.

On the foreign server, confirm the server is forwarding:

```bash
sudo journalctl -u portman-server -n 20 --no-pager
```

Expected: `bridged` lines, then `client->backend finished` and `backend->client finished` with byte counts above zero.

Finally, connect a device through the inside inbound, open a website, and check that the `portman-bridge` client's traffic counters increase in the foreign panel.

### Common mistakes

- **Wrong bridge port:** Portman listens on 4443, but the 3X-UI inbound listens on 10000 on localhost. Don't swap them.
- **Mismatched UUID:** the inside outbound and the foreign inbound must use the same UUID. If they don't match, the foreign server closes the connection right away, with `0 bytes` in its logs.
- **Mismatched protocol:** the outbound and inbound must both be VLESS with encryption `none` and no TLS of their own. TLS is handled by portman.
- **Proxied hostname:** a Cloudflare-proxied hostname won't pass port 4443. Use a DNS-only record.

## Running as a service (Linux, systemd)

These steps are for Debian and Ubuntu. Run them as root or with `sudo`.

Pick either the server or the client role on each machine. The steps are the same except for the config file and the unit file.

### 1. Install the binary

Download `portman-<version>-linux-amd64` from the release page, then:

```bash
sudo install -m 0755 portman-<version>-linux-amd64 /usr/local/bin/portman
```

### 2. Create a service user

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin portman
```

### 3. Create the directories and set ownership

```bash
sudo install -d -o root -g portman -m 0750 /etc/portman
sudo install -d -o portman -g portman -m 0750 /var/log/portman
```

- `/etc/portman` holds the config. Root can edit it, and the service can read it.
- `/var/log/portman` holds the logs. Only the service user can write to it.

### 4. Install the config

Server:

```bash
sudo install -o root -g portman -m 0640 configs/server.example.json /etc/portman/server.json
sudo nano /etc/portman/server.json
```

Client:

```bash
sudo install -o root -g portman -m 0640 configs/client.example.json /etc/portman/client.json
sudo nano /etc/portman/client.json
```

### 5. Give the server access to the TLS certificate (server only)

Let's Encrypt keys are readable only by root by default. Grant read access to the service user:

```bash
sudo apt install -y acl
sudo setfacl -m u:portman:x /etc/letsencrypt
sudo setfacl -R -m u:portman:rX /etc/letsencrypt/live /etc/letsencrypt/archive
sudo setfacl -R -d -m u:portman:rX /etc/letsencrypt/archive
```

Check that the service user can read the key:

```bash
sudo -u portman test -r /etc/letsencrypt/live/YOUR_HOSTNAME/privkey.pem && echo "key readable"
```

### 6. Install and start the unit

Server:

```bash
sudo install -m 0644 deploy/portman-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now portman-server
```

Client:

```bash
sudo install -m 0644 deploy/portman-client.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now portman-client
```

### 7. Rotate the logs

```bash
sudo install -m 0644 deploy/portman.logrotate /etc/logrotate.d/portman
```

### 8. Check that it works

```bash
sudo systemctl status portman-server --no-pager   # or portman-client
sudo journalctl -u portman-server -n 50 --no-pager
sudo tail -n 50 /var/log/portman/portman.log
```

The log should show `startup check ... OK`. Use `log_level` `info` in production. `debug` logs every connection.

### Updating

```bash
sudo install -m 0755 portman-<new-version>-linux-amd64 /usr/local/bin/portman
sudo systemctl restart portman-server   # or portman-client
```

## Logging

At `debug`, each connection logs its ID, its byte counts per direction, and its duration. At `info`, it logs bridging and close events, and each client tunnel logs the local port it used and how long the TLS dial took. Use `info` in production, because `debug` logs every connection.

Startup checks log whether the remote TCP port and TLS handshake are reachable. They only log results and never stop the service.

## Reliability

The client retries a failed TLS dial up to 3 times, waiting 1 second and then 2 seconds between attempts. This helps on unstable links. The retry adds delay when the remote server is down: a local connection can wait for three dial timeouts before it fails.

## Traffic stats and daily report

With `stats.enabled`, each server counts traffic per day in its timezone and saves the totals to `stats.state_path`:

- **Interface** counters come from `/proc/net/dev`. They include all traffic, including SSH, the panel, and direct connections. Providers bill this figure.
- **Bridge** counters count only tunnel traffic.

The inside server (`notify.enabled`) sends one plain-text report to a Bale bot every day at `notify.send_at`. It includes its own numbers, plus the foreign server's numbers, which it fetches over mutual TLS from `notify.foreign_stats_addr`. The foreign server serves them only when `stats.listen_addr` is set, and only with `client_ca_file` configured.

Send a report immediately with `portman -config /etc/portman/config.json -report-now`.

The bot token and chat ID live in the config file. Keep that file mode `0640` with group `portman`. The token is redacted from logs and errors.

Limits: sampling runs every `sample_interval`, so an abrupt stop can lose up to one interval of counts. If the inside server is off at `send_at`, that day's report is not sent.

## Security

### Client authentication (optional)

Set up mutual TLS so that only your inside server can use the bridge, even if the port is reachable. Generate the certificates on a trusted machine with the commands in the release notes, keep `ca.key` offline, and copy `ca.pem` to the server, and `client.pem` and `client.key` to the client.

Server (`/etc/portman/server.json`): add `"client_ca_file": "/etc/portman/ca.pem"`.

Client (`/etc/portman/client.json`): add `"client_cert_file": "/etc/portman/client.pem"` and `"client_key_file": "/etc/portman/client.key"`.

Permissions: `ca.pem` and `client.pem` are public, so `0644` is fine. `client.key` is private: `root:portman`, mode `0640`.

Portman also limits concurrent connections and closes idle ones, so a stalled or flooding client can't use up the server. Adjust `max_connections` and `idle_timeout` in the config if your traffic needs more.

### Restrict the bridge port to your client

On the foreign server, allow the bridge port only from the inside server's IP, then remove any rule that allows it from everywhere. Replace `INSIDE_IP` with the inside server's public IP:

```bash
sudo ufw allow from INSIDE_IP to any port 4443 proto tcp
sudo ufw status numbered
sudo ufw delete NUMBER   # the general "4443/tcp ALLOW IN Anywhere" rule, IPv4 and IPv6
```

Check that the open rule is gone with `sudo ufw status | grep 4443`. Only the `INSIDE_IP` rule should remain. If the inside server's IP changes, update the rule.

### Keep the backend on localhost

The backend should listen only on `127.0.0.1`, never on a public address. Check it with:

```bash
sudo ss -tlnp | grep 10000
```

The output should show `127.0.0.1:10000`. If it shows `0.0.0.0:10000` or `*:10000`, change the listen address in the 3X-UI inbound.

### Use a separate identity for the bridge

Give the bridge its own UUID, used only between the two servers. Don't reuse a device UUID. If a device is lost or the UUID leaks, create a new bridge UUID on both sides and remove the old one.

### Run the service as an unprivileged user

Use the hardened unit files in `deploy/`, which run as the `portman` user with `NoNewPrivileges` and a read-only system. Don't run portman as root. If you tested with an unhardened unit, switch to the templates in step 6 of the service setup.

### Protect the config and logs

- `/etc/portman` should be owned by `root` with group `portman`, mode `0750`, and config files mode `0640`. These files contain your hostnames and the bridge UUID.
- `/var/log/portman` should be owned by `portman`, mode `0750`. Logs at `debug` level record every connection, including client IP addresses, so use `info` in production.

### Protect the 3X-UI panel and SSH

- Use a strong, unique password for each 3X-UI panel, and change the default panel path if your version allows it.
- Restrict panel ports to your own IP with the same `ufw allow from` method, if you always connect from one address.
- Use key-based SSH login, disable password login if you can, and keep `fail2ban` running with its `sshd` jail enabled.

### Keep the system updated

Run `sudo apt update && sudo apt upgrade` regularly on both servers, and update Xray and 3X-UI through their own update paths.

### Verify downloads

Each release includes `checksums-<version>.txt` with SHA-256 hashes. After downloading a binary, check it against the file:

```bash
sha256sum -c checksums-<version>.txt --ignore-missing
```

Expected output: `portman-<version>-linux-amd64: OK`. Don't install a binary whose hash doesn't match.

### Report problems

If you find a security issue, don't open a public issue with exploit details. Contact the maintainer privately first.

## Limitations

- No protocol awareness. Protocol and identity must match between client-side and backend.
- One listener per process.

## License

MIT. See [LICENSE](LICENSE).