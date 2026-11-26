# Portman

A small TLS tunnel that forwards TCP traffic between two servers. It offers optional mutual TLS, per-day traffic accounting on both servers, and a daily report sent to a Bale bot.

Portman does not parse the traffic. It is a byte pipe, so the backend on the far side must accept exactly what the near side sends. For example, a VLESS outbound on the inside server needs a VLESS inbound on the foreign server, with the same client ID.

## Status

Experimental (v0.5.1). Tested end to end with one inside server and one foreign server carrying real traffic. Not tested with multiple clients or under production load.

## Versioning

Releases use [Semantic Versioning](https://semver.org/) and are tagged `vX.Y.Z`. Until 1.0.0, a minor version may change configuration or behavior. See [CHANGELOG.md](CHANGELOG.md) for what changed and [ROADMAP.md](ROADMAP.md) for what's planned.

Check the installed build:

```bash
/usr/local/bin/portman -version
```

## How it works

```
phone -> inside 3X-UI -> portman client (127.0.0.1:20001)
      -> TLS with mutual authentication -> portman server (:4443)
      -> foreign 3X-UI (127.0.0.1:10000) -> internet
```

| | Inside server | Foreign server |
|---|---|---|
| Role | `client` | `server` |
| Service | `portman-client` | `portman-server` |
| Config | `/etc/portman/client.json` | `/etc/portman/server.json` |
| Listens on | `127.0.0.1:20001` | `:4443` (bridge), `:8444` (stats) |
| Connects to | `bridge.example.com:4443`, `bridge.example.com:8444`, Bale API | `127.0.0.1:10000` (3X-UI) |
| TLS identity | `client.pem` and `client.key` | Let's Encrypt certificate, and `ca.pem` to verify clients |
| Extra work | Daily report to Bale | Counts its own traffic and serves it |

Both servers run the same binary, `/usr/local/bin/portman`, as the unprivileged `portman` user and group.

| Port | Server | Purpose | Allow from |
|---|---|---|---|
| 4443/tcp | foreign | Bridge (TLS) | inside server IP only |
| 8444/tcp | foreign | Traffic counters (mutual TLS) | inside server IP only |
| 10000/tcp | foreign | 3X-UI bridge inbound | localhost only |
| 20001/tcp | inside | Local entry from 3X-UI | localhost only |

## Requirements

- Debian 12 or 13, or Ubuntu 22.04 or newer, on both servers, with systemd and `sudo`.
- A hostname for the bridge, such as `bridge.example.com`, with a **DNS-only** record pointing to the foreign server. Cloudflare-proxied records don't pass ports 4443 or 8444.
- A Let's Encrypt certificate for that hostname on the foreign server.
- 3X-UI on both servers.
- For the daily report: a Bale bot token and a chat ID.
- To build from source: Go 1.21 or newer.

## Build and tests

```bash
go build -o dist/portman ./cmd/portman
go test ./...
```

`release.ps1` (Windows PowerShell) runs `go vet`, a format check, and `go test`, then builds Linux and Windows binaries with the version and commit embedded, and writes SHA-256 checksums to `dist/`.

## Configuration reference

| Field | Used by | Description |
|---|---|---|
| `mode` | both | `server` or `client` |
| `listen_addr` | both | Address to listen on |
| `log_level` | both | `debug`, `info`, `warn`, or `error`. Use `info` in production |
| `log_path` | both | Log file. Output also goes to stdout |
| `max_connections` | both | Concurrent connections per process (default 256). Extra connections are dropped with a warning |
| `idle_timeout` | both | Close a connection after this long with no data in either direction (default `5m`) |
| `remote_addr` | client | Bridge server, `host:port` |
| `server_name` | client | TLS name, must match the server certificate |
| `client_cert_file`, `client_key_file` | client | Client certificate and key. Set both or neither |
| `backend_addr` | server | Local backend, normally `127.0.0.1:10000` |
| `cert_file`, `key_file` | server | Server certificate and key |
| `client_ca_file` | server | CA that signs client certificates. When set, clients must present one |
| `stats.enabled` | both | Count traffic per day and save totals to `stats.state_path` |
| `stats.label` | both | Name shown in the report |
| `stats.state_path` | both | JSON state file, for example `/var/lib/portman/state.json` |
| `stats.interface` | both | Network interface to count. Find it with `ip -br link` |
| `stats.timezone` | both | Day and month boundaries, for example `Asia/Tehran`. Use the same value on both servers |
| `stats.billing_day` | both | Day of the month when this server's cycle resets (1 to 31, default 1). If the month is shorter, the cycle starts on its last day. Set it to match that server's provider |
| `stats.monthly_limit_gb` | both | Monthly limit in GiB. `0` means no limit line |
| `stats.count_mode` | both | `sum` (received + sent) or `max` (the larger of the two). Match your provider's billing |
| `stats.sample_interval` | both | How often counters are read, for example `1m` (minimum `10s`) |
| `stats.listen_addr` | server | Stats endpoint address, for example `:8444`. Requires `client_ca_file` |
| `notify.enabled` | client | Send the daily report |
| `notify.api_base` | client | Bale API base. Default `https://tapi.bale.ai` |
| `notify.bot_token` | client | Bale bot token |
| `notify.chat_id` | client | Chat to send to. Quote it, even if it's a number |
| `notify.send_at` | client | Send time, `HH:MM`, in `stats.timezone` |
| `notify.foreign_stats_addr` | client | Foreign stats endpoint, for example `bridge.example.com:8444` |

## Example configuration

Install these as `/etc/portman/server.json` on the foreign server and `/etc/portman/client.json` on the inside server. They are also in `examples/`.

### server.json (foreign server)

```json
{
  "mode": "server",
  "listen_addr": ":4443",
  "backend_addr": "127.0.0.1:10000",
  "cert_file": "/etc/letsencrypt/live/bridge.example.com/fullchain.pem",
  "key_file": "/etc/letsencrypt/live/bridge.example.com/privkey.pem",
  "client_ca_file": "/etc/portman/ca.pem",
  "max_connections": 256,
  "idle_timeout": "5m",
  "log_level": "info",
  "log_path": "/var/log/portman/server.log",
  "stats": {
    "enabled": true,
    "label": "foreign",
    "state_path": "/var/lib/portman/state.json",
    "interface": "eth0",
    "timezone": "Asia/Tehran",
    "billing_day": 27,
    "monthly_limit_gb": 0,
    "count_mode": "sum",
    "sample_interval": "1m",
    "listen_addr": ":8444"
  }
}
```

### client.json (inside server)

```json
{
  "mode": "client",
  "listen_addr": "127.0.0.1:20001",
  "remote_addr": "bridge.example.com:4443",
  "server_name": "bridge.example.com",
  "client_cert_file": "/etc/portman/client.pem",
  "client_key_file": "/etc/portman/client.key",
  "max_connections": 256,
  "idle_timeout": "5m",
  "log_level": "info",
  "log_path": "/var/log/portman/client.log",
  "stats": {
    "enabled": true,
    "label": "inside",
    "state_path": "/var/lib/portman/state.json",
    "interface": "eth0",
    "timezone": "Asia/Tehran",
    "billing_day": 27,
    "monthly_limit_gb": 100,
    "count_mode": "sum",
    "sample_interval": "1m"
  },
  "notify": {
    "enabled": true,
    "api_base": "https://tapi.bale.ai",
    "bot_token": "PASTE_BALE_BOT_TOKEN",
    "chat_id": "PASTE_CHAT_ID",
    "send_at": "23:55",
    "foreign_stats_addr": "bridge.example.com:8444"
  }
}
```

Before you start, change these values:

- `stats.interface`: use the interface that holds the server's public address (`ip -br addr`). Often `eth0`, but it may be `ens3` or `ens160`. Set it separately on each server.
- `monthly_limit_gb`: your provider's limit in GiB.
- `count_mode`: `sum` if your provider bills in + out, `max` if it bills the larger of the two.
- `bot_token` and `chat_id`: the values from the Bale setup in step 3.5.

Each server has its own `timezone`, `billing_day`, `monthly_limit_gb`, and `count_mode`. Set them to match that server's provider.

## Setup

Replace these placeholders everywhere:

| Placeholder | Meaning | Example |
|---|---|---|
| `bridge.example.com` | Bridge hostname (DNS-only, certificate name) | your domain |
| `FOREIGN_IP` | Public IP of the foreign server | |
| `INSIDE_IP` | Public IP of the inside server | |
| `YOUR-BRIDGE-UUID` | New UUID used only for the bridge | `uuidgen` |

Work through steps 0 and 1 on both servers. Then do step 2 on the foreign server and step 3 on the inside server. Each step ends with checks. Don't skip them, because each check confirms the step before it.

### Step 0: Create the certificates for mutual TLS (once, on a trusted machine)

Do this on your own computer, not on the servers. Keep `ca.key` offline.

```bash
mkdir -p ~/portman-certs && cd ~/portman-certs

openssl ecparam -name prime256v1 -genkey -noout -out ca.key
openssl req -x509 -new -key ca.key -sha256 -days 3650 -subj "/CN=portman-ca" -out ca.pem

openssl ecparam -name prime256v1 -genkey -noout -out client.key
openssl req -new -key client.key -subj "/CN=portman-client" -out client.csr
printf "basicConstraints=CA:FALSE\nkeyUsage=digitalSignature\nextendedKeyUsage=clientAuth\n" > client.ext
openssl x509 -req -in client.csr -CA ca.pem -CAkey ca.key -CAcreateserial -days 825 -sha256 -extfile client.ext -out client.pem
```

Check the result:

```bash
openssl verify -CAfile ca.pem client.pem
```

Expected: `client.pem: OK`.

Copy `ca.pem` to the foreign server. Copy `client.pem` and `client.key` to the inside server. Use `scp` or your file manager. Never copy `ca.key` to either server.

### Step 1: Common steps (both servers)

#### 1.1 Create the portman group and user

```bash
sudo groupadd --system portman
sudo useradd --system --gid portman --home-dir /nonexistent --no-create-home --shell /usr/sbin/nologin portman
```

Test:

```bash
getent group portman
id portman
```

`id portman` should show `gid` equal to the `portman` group, and no login shell is used.

#### 1.2 Create the directories

```bash
sudo install -d -o root -g portman -m 0750 /etc/portman
sudo install -d -o portman -g portman -m 0750 /var/log/portman
sudo install -d -o portman -g portman -m 0750 /var/lib/portman
```

- `/etc/portman` holds the configs and keys. Root owns it, and the service group can read it.
- `/var/log/portman` holds the logs. Only the service user writes to it.
- `/var/lib/portman` holds the traffic counters.

Test:

```bash
ls -ld /etc/portman /var/log/portman /var/lib/portman
```

Expected: `root portman` on `/etc/portman`, and `portman portman` on the other two.

#### 1.3 Install packages

```bash
sudo apt update
sudo apt install -y acl curl python3 ufw
```

Test:

```bash
command -v setfacl curl python3 ufw
```

All four paths should print.

#### 1.4 Install the binary

Download `portman-<version>-linux-amd64` from the release page, then:

```bash
sudo install -m 0755 portman-<version>-linux-amd64 /usr/local/bin/portman
/usr/local/bin/portman -version
```

Test: the output shows the version and commit you expect.

#### 1.5 Firewall baseline

Allow SSH before you enable the firewall, or you can lock yourself out. If you use a non-standard SSH port, allow that port instead.

```bash
sudo ufw allow OpenSSH
sudo ufw --force enable
sudo ufw status verbose
```

Test: `Status: active`, and an `OpenSSH` allow rule is listed.

### Step 2: Foreign server (server role)

#### 2.1 Get the Let's Encrypt certificate

The bridge hostname must point, DNS-only, to this server's public IP. Port 80 must be free for the standalone challenge. If 3X-UI uses port 80, stop it briefly during issuance.

```bash
sudo apt install -y certbot
sudo certbot certonly --standalone -d bridge.example.com
```

Test:

```bash
sudo openssl x509 -in /etc/letsencrypt/live/bridge.example.com/cert.pem -noout -subject -enddate
sudo openssl x509 -in /etc/letsencrypt/live/bridge.example.com/cert.pem -noout -ext subjectAltName
```

The name must appear in the SAN list. The expiry date should be in the future.

#### 2.2 Let the portman user read the certificate

```bash
sudo setfacl -m u:portman:x /etc/letsencrypt
sudo setfacl -R -m u:portman:rX /etc/letsencrypt/live /etc/letsencrypt/archive
sudo setfacl -R -d -m u:portman:rX /etc/letsencrypt/archive
```

Test:

```bash
sudo -u portman test -r /etc/letsencrypt/live/bridge.example.com/privkey.pem && echo "key readable"
```

#### 2.3 Install the CA certificate

Copy `ca.pem` from step 0 to this server (for example into `/tmp`), then:

```bash
sudo install -o root -g root -m 0644 /tmp/ca.pem /etc/portman/ca.pem
openssl x509 -in /etc/portman/ca.pem -noout -subject
```

Expected: `subject=CN = portman-ca`.

#### 2.4 Install the server config

```bash
sudo nano /etc/portman/server.json
```

Paste the `server.json` block from the Example configuration section, and change the `stats.interface` value. Then set the permissions:

```bash
sudo chown root:portman /etc/portman/server.json
sudo chmod 0640 /etc/portman/server.json
```

#### 2.5 Check the config

```bash
sudo python3 -m json.tool /etc/portman/server.json > /dev/null && echo "JSON OK"
cd / && sudo -u portman timeout 5 /usr/local/bin/portman -config /etc/portman/server.json; echo "exit=$?"
```

Expected: `JSON OK`, then `exit=124`. The value 124 means the program started and ran until the timeout stopped it. A `1` means it printed an error before that, and the error names the field to fix.

#### 2.6 Create the 3X-UI bridge inbound

Generate the UUID on any machine:

```bash
uuidgen
```

In the foreign 3X-UI panel, add an inbound:

| Field | Value |
|---|---|
| Protocol | VLESS |
| Listen IP | `127.0.0.1` |
| Port | `10000` |
| Network | tcp |
| Security | none |

Add one client with ID `YOUR-BRIDGE-UUID`, flow empty, and remark `portman-bridge`. Keep this UUID separate from the UUIDs given to your devices.

Test:

```bash
sudo ss -tlnp | grep 10000
```

Expected: `127.0.0.1:10000`. If you see `0.0.0.0:10000` or `*:10000`, change the listen IP.

#### 2.7 Firewall rules for the bridge and stats

Allow only the inside server's IP, on both ports:

```bash
sudo ufw allow from INSIDE_IP to any port 4443 proto tcp comment 'portman bridge'
sudo ufw allow from INSIDE_IP to any port 8444 proto tcp comment 'portman stats'
sudo ufw status numbered
```

Delete any rule that allows 4443 or 8444 from anywhere, including IPv6 rules. Use the numbers from the status output:

```bash
sudo ufw delete NUMBER
```

Test: `sudo ufw status | grep -E '4443|8444'` should list only the two `INSIDE_IP` rules. Also check the provider's firewall panel, which is separate from ufw. It must allow the same ports from the inside server's IP, or allow them with a rule that matches.

#### 2.8 Install and start the service

```bash
sudo install -m 0644 deploy/portman-server.service /etc/systemd/system/portman-server.service
sudo systemctl daemon-reload
sudo systemctl enable --now portman-server
```

If you had an older unit named `portman` on this server, remove it:

```bash
sudo systemctl disable --now portman 2>/dev/null
sudo rm -f /etc/systemd/system/portman.service
sudo systemctl daemon-reload
```

#### 2.9 Verify the foreign server

```bash
systemctl is-active portman-server
sudo journalctl -u portman-server -n 40 --no-pager
sudo ss -tlnp | grep -E ':4443|:8444|:10000'
```

Expected in the journal, in this order:

- `starting portman <version> (commit ...) in server mode`
- `client certificates required (client_ca_file set)`
- `server listening on :4443, forwarding to backend 127.0.0.1:10000`
- `startup check: backend 127.0.0.1:10000 TCP connect OK`
- `stats endpoint listening on :8444 (client certificate required)`

Expected from `ss`: portman listens on `:4443` and `:8444`, and 3X-UI listens on `127.0.0.1:10000`.

Test that the stats endpoint refuses clients without a certificate:

```bash
curl -sS --max-time 10 https://bridge.example.com:8444/stats
```

Expected: an error about a certificate being required. A timeout means the firewall is still blocking you, and a `connection refused` means the service isn't listening.

### Step 3: Inside server (client role)

#### 3.1 Install the client certificate and key

```bash
sudo install -o root -g root -m 0644 /tmp/client.pem /etc/portman/client.pem
sudo install -o root -g portman -m 0640 /tmp/client.key /etc/portman/client.key
```

Test:

```bash
sudo -u portman test -r /etc/portman/client.key && echo "key readable"
openssl verify -CAfile /etc/portman/client.pem /etc/portman/client.pem 2>/dev/null | head -1
```

The first line should print `key readable`. The client certificate is signed by the CA, but you don't have the CA on this server, so verify it on your own machine in step 0 instead of here.

#### 3.2 Install the client config

Find the interface name first:

```bash
ip -br addr
```

Then open the config:

```bash
sudo nano /etc/portman/client.json
```

Paste the `client.json` block from the Example configuration section, and set `stats.interface`, `bot_token`, and `chat_id` (step 3.8 explains how to get the last two). Then set permissions:

```bash
sudo chown root:portman /etc/portman/client.json
sudo chmod 0640 /etc/portman/client.json
```

#### 3.3 Check the config

```bash
sudo python3 -m json.tool /etc/portman/client.json > /dev/null && echo "JSON OK"
cd / && sudo -u portman timeout 5 /usr/local/bin/portman -config /etc/portman/client.json; echo "exit=$?"
```

Expected: `JSON OK`, then `exit=124`. Anything else prints the error. Common causes:

- `client_ca_file is for server mode only`: remove that line from the client config.
- `client_cert_file and client_key_file must be set together`: add the missing one.
- `notify.chat_id is required` or `bot_token is required`: fill in the Bale values.
- `json: cannot unmarshal number into Go struct field`: quote `chat_id`.

#### 3.4 Create the 3X-UI outbound and routing

**Outbound.** In **Xray Configs → Outbounds**, add:

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
| Mux | off for the first test |

For reference, the JSON looks like this:

```json
{
  "tag": "portman",
  "protocol": "vless",
  "settings": {
    "vnext": [
      {
        "address": "127.0.0.1",
        "port": 20001,
        "users": [{ "id": "YOUR-BRIDGE-UUID", "encryption": "none" }]
      }
    ]
  }
}
```

**Routing.** Send only the traffic you want through the bridge. For a device inbound named `phone-in`:

```json
{
  "type": "field",
  "inboundTag": ["phone-in"],
  "outboundTag": "portman"
}
```

For testing, use a separate inbound for the bridge, so a broken bridge doesn't cut off your normal connection. Restart Xray from the panel.

#### 3.5 Install and start the client service

```bash
sudo install -m 0644 deploy/portman-client.service /etc/systemd/system/portman-client.service
sudo systemctl daemon-reload
sudo systemctl enable --now portman-client
```

#### 3.6 Verify the bridge

```bash
systemctl is-active portman-client
sudo journalctl -u portman-client -n 40 --no-pager
sudo ss -tlnp | grep 20001
```

Expected in the journal:

- `client listening on 127.0.0.1:20001, tunnel to bridge.example.com:4443`
- `startup check: remote bridge ... TCP connect OK`
- `startup check: TLS handshake to bridge.example.com:4443 OK`
- `next daily report at ...`

Then test the reachability of the bridge port:

```bash
timeout 10 bash -c 'exec 3<>/dev/tcp/bridge.example.com/4443' && echo "TCP 4443 open" || echo "TCP 4443 blocked"
```

Connect a device through the bridge inbound, open a website, and check:

```bash
sudo tail -n 20 /var/log/portman/client.log
```

Expected: `tunnel up` lines, and `local->remote finished` and `remote->local finished` with bytes above zero. On the foreign server, the same connection appears as `bridged` lines in `portman-server`. The foreign 3X-UI panel's counters for `portman-bridge` should increase.

#### 3.7 Verify the stats endpoint from the inside server

With the client certificate, the foreign server should return JSON:

```bash
sudo curl -sS --max-time 10 --cert /etc/portman/client.pem --key /etc/portman/client.key https://bridge.example.com:8444/stats | python3 -m json.tool | head -30
```

Expected: `"label": "foreign"` near the top, and a `snapshot` section.

Without a certificate, it must refuse:

```bash
curl -sS --max-time 10 https://bridge.example.com:8444/stats
```

Expected: a certificate-required error.

#### 3.8 Set up the Bale bot and test the report

1. In Bale, create a bot with the bot creator, and save its token.
2. Put the token in `bot_token` in `/etc/portman/client.json`.
3. Send any message to the bot from the account that should receive reports.
4. Read the token from the config without pasting it anywhere, and get the chat ID:

```bash
TOKEN=$(sudo python3 -c 'import json;print(json.load(open("/etc/portman/client.json"))["notify"]["bot_token"])')
curl -sS "https://tapi.bale.ai/bot${TOKEN}/getMe"
curl -sS "https://tapi.bale.ai/bot${TOKEN}/getUpdates"
```

`getMe` should return `"ok":true`. In `getUpdates`, find `"chat":{"id":...}` and put that number in `chat_id`, in quotes. Then restart the service:

```bash
sudo systemctl restart portman-client
```

Send a report now. Run it as the `portman` user, from a directory it can access:

```bash
cd / && sudo -u portman /usr/local/bin/portman -config /etc/portman/client.json -report-now; echo "exit=$?"
```

Expected: `exit=0` and a message in Bale with the `[inside]` section, plus a `[foreign]` section. If the foreign section says it's not reachable, return to the firewall checks in 2.7 and 3.7.

Check that the token never reached the log file. This should print `0`:

```bash
sudo grep -c -F "$TOKEN" /var/log/portman/client.log
```

### Operations

Status and logs:

```bash
systemctl is-active portman-server      # or portman-client
sudo journalctl -u portman-client -f
sudo tail -f /var/log/portman/client.log
```

Change a config: edit it, run the JSON check and the timeout test from step 2.5 or 3.3, then restart.

Send a report now, always as `portman`:

```bash
cd / && sudo -u portman /usr/local/bin/portman -config /etc/portman/client.json -report-now
```

Update to a new release:

```bash
sudo cp /usr/local/bin/portman /usr/local/bin/portman.prev
sudo install -m 0755 portman-<new-version>-linux-amd64 /usr/local/bin/portman
sudo systemctl restart portman-client    # or portman-server
/usr/local/bin/portman -version
```

Roll back:

```bash
sudo install -m 0755 /usr/local/bin/portman.prev /usr/local/bin/portman
sudo systemctl restart portman-client    # or portman-server
```

Turn the report off: set `notify.enabled` to `false` in `client.json`, then restart. The bridge keeps running.

Reset the counters: stop the service, move the state file aside, and start again. This erases the history.

```bash
sudo systemctl stop portman-client
sudo mv /var/lib/portman/state.json /var/lib/portman/state.json.old
sudo systemctl start portman-client
```

### Troubleshooting

| Symptom | Check | Fix |
|---|---|---|
| Service shows `activating (auto-restart)` | `sudo journalctl -u portman-client -n 50` | Run the binary manually as `portman` (step 3.3). It prints the exact error. |
| `invalid config: ...` | The message names the field | Fix that field. Mode-specific settings are the usual cause. |
| `Failed to open log file: permission denied` | `ls -l /var/log/portman` | `sudo chown -R portman:portman /var/log/portman`. A root-run command created the file. |
| Server log: `certificate required` | Client config | Add `client_cert_file` and `client_key_file`, and check that the client certificate is signed by `ca.pem`. |
| Client log: `unknown authority` or `bad certificate` | Server config and `ca.pem` | `client_ca_file` must be the CA that signed `client.pem`. |
| Client log: `certificate is valid for ..., not ...` | `server_name` | Set `server_name` and the hostname to the certificate name. |
| Stats: `context deadline exceeded` | Step 3.7 | Firewall: allow 8444 from `INSIDE_IP`, and check the provider's firewall panel. |
| Stats: `connection refused` | `ss -tlnp \| grep 8444` on foreign | The service isn't listening. Check its journal, and check `portman -version` for the stats-capable version. |
| Report has only `[inside]` | Journal: `foreign stats unavailable` | Same as the stats row above. |
| Report shows zero counts | `sudo cat /var/lib/portman/state.json`; journal for `stats save failed` | Check ownership of `/var/lib/portman`. A one-shot run has no baseline, so the scheduled service provides the numbers. |
| Bale `HTTP 401` | Token | Copy the token again from the bot creator. |
| Bale `chat not found` | `chat_id` | Message the bot, then read `getUpdates` again. |
| Bridge connects, but nothing passes | Foreign log shows `0 bytes` | The inside outbound's ID doesn't match the foreign inbound client. |
| Pages are slow | Client log `dial took` | Each new connection pays a handshake. See the roadmap for connection reuse. |

### Security

Portman checks client identity with mutual TLS when `client_ca_file` is set, and the stats endpoint always requires it. Portman doesn't protect the 3X-UI panel, SSH, or the backend, so protect those yourself:

- **Allow only your inside server.** Ports 4443 and 8444 should accept traffic only from `INSIDE_IP`, both in ufw and in the provider's firewall.
- **Keep the backend on localhost.** The 3X-UI bridge inbound listens on `127.0.0.1:10000`.
- **Use a separate bridge identity.** The bridge UUID is used only between the two servers. If it leaks, create a new one on both sides.
- **Run as `portman`.** The units run as an unprivileged user with a read-only system. Don't run portman as root.
- **Protect the files.** `/etc/portman/*.json` and `client.key` are mode `0640` with group `portman`. They hold your hostnames, the bridge UUID, and the bot token.
- **Use `info` in production.** `debug` logs every connection, including client addresses.
- **Protect the panels.** Use strong, unique passwords for 3X-UI. Restrict the panel ports to your own IP with ufw, if you always connect from one address.
- **Protect SSH.** Use key-based login, and keep `fail2ban` running with its `sshd` jail.
- **Keep updated.** Run `sudo apt update && sudo apt upgrade` regularly, and update 3X-UI and Xray through their own update paths.
- **Verify downloads.** Each release has `checksums-<version>.txt`. Run `sha256sum -c checksums-<version>.txt --ignore-missing` and don't install a file whose hash doesn't match.

Report security problems privately to the maintainer, not in a public issue.

### Limitations

- Portman has no protocol awareness. The protocol and identity must match on both sides.
- One listener per process. A server handles one bridge port.
- Counters are sampled every `sample_interval`. If the service stops abruptly, up to one interval of counts can be lost.
- The report is sent only if the inside server is running at `send_at`. Missed reports are not sent later.
- Interface counters use `/proc/net/dev`, so they work on Linux only.
- Each new local connection pays a new TLS handshake, which adds latency on long links.

## License

MIT. See [LICENSE](LICENSE).