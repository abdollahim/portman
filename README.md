# Portman

A small TLS tunnel that forwards TCP traffic between two servers. It offers optional mutual TLS, per-day traffic accounting on both servers, a daily traffic report, and scheduled connectivity checks, all sent to a Bale or Telegram bot.

Portman does not parse the traffic. It is a byte pipe, so the backend on the far side must accept exactly what the near side sends. For example, a VLESS outbound on the inside server needs a VLESS inbound on the foreign server, with the same client ID.

## Status

Experimental (v0.7.1). Tested end to end with one inside server and one foreign server carrying real traffic. Not tested with multiple clients or under production load.

## Versioning

Releases use [Semantic Versioning](https://semver.org/) and are tagged `vX.Y.Z`. Until 1.0.0, a minor version may change configuration or behavior. See [CHANGELOG.md](CHANGELOG.md) for what changed and [ROADMAP.md](ROADMAP.md) for what's planned.

Check the installed build (both flags print the same output):

```bash
/usr/local/bin/portman -v
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
| Connects to | `bridge.example.com:4443`, `bridge.example.com:8444`, Bale or Telegram API, check targets | `127.0.0.1:10000` (3X-UI) |
| TLS identity | `client.pem` and `client.key` | Let's Encrypt certificate, and `ca.pem` to verify clients |
| Extra work | Daily traffic report and scheduled connectivity checks | Counts its own traffic and serves it |

Both servers run the same binary, `/usr/local/bin/portman`, as the unprivileged `portman` user and group.

| Port | Server | Purpose | Allow from |
|---|---|---|---|
| 4443/tcp | foreign | Bridge (TLS) | inside server IP only |
| 8444/tcp | foreign | Traffic counters (mutual TLS) | inside server IP only |
| 10000/tcp | foreign | 3X-UI bridge inbound | localhost only |
| 20001/tcp | inside | Local entry from 3X-UI | localhost only |
| 443/tcp (outbound) | inside | Bot API calls and connectivity check targets | none (outbound) |

## Requirements

- Debian 12 or 13, or Ubuntu 22.04 or newer, on both servers, with systemd and `sudo`.
- A hostname for the bridge, such as `bridge.example.com`, with a **DNS-only** record pointing to the foreign server. Cloudflare-proxied records don't pass ports 4443 or 8444.
- A Let's Encrypt certificate for that hostname on the foreign server.
- 3X-UI on both servers.
- For the daily report and checks: a bot token and a chat ID (Bale by default, or Telegram).
- The `ping` command on the inside server.
- To build from source: Go 1.21 or newer.

## Build and tests

```bash
go build -o dist/portman ./cmd/portman
go test ./...
```

`release.ps1` (Windows PowerShell) runs `go vet`, a format check, and `go test`, then builds Linux and Windows binaries with the version and commit embedded, and writes SHA-256 checksums to `dist/`.

## Connectivity checks

The inside server can check a list of IP addresses on a schedule and send the result to the same bot as the traffic report. For each address it sends one ping and one DNS query over UDP. The status is `OK` when both answer, `PARTIAL` when one does, and `DOWN` when neither does.

Targets live in a JSON file that is read on every run, so you can change the list without restarting:

```json
{
  "groups": [
    { "name": "Global", "targets": ["1.1.1.1", "8.8.8.8"] },
    { "name": "Operators", "targets": ["185.55.226.26"] }
  ]
}
```

Each group becomes a section of the report. Addresses must be valid IPv4 or IPv6 addresses. Keep this file with the other configs, and don't commit a private list.

The schedule is set in the `checks` section of `client.json`. Days are English names, and `send_at` is `HH:MM` in `checks.timezone`.

Run one check now, as the service user:

```bash
cd / && sudo -u portman /usr/local/bin/portman -config /etc/portman/client.json -checks-now; echo "exit=$?"
```

### Reading the report

Each address shows one status:

- `OK`: ping and DNS both answered.
- `PARTIAL`: one of the two answered. Often the host is up, but its resolver refuses queries from outside its network.
- `DOWN`: neither answered.

The last line counts successes across all addresses: `ping ok: 20/21` means 20 of 21 addresses answered a ping, and `DNS ok: 16/21` means 16 answered the DNS query.

## Traffic stats and daily report

Each server counts traffic per day in its own timezone and billing cycle, and saves the totals to `stats.state_path`. Every figure in the report falls into one of three groups:

- **Whole server (in + out):** read from the network interface (`/proc/net/dev`). It includes everything: SSH, the panel, OS updates, direct clients, and portman. This is the figure your provider bills, and the one compared against `monthly_limit_gb`.
- **Portman bridge:** the tunnel's own bytes, in both directions.
- **Other:** whole server minus portman. This is an estimate. The bridge counts decrypted payload, while the interface also counts TLS overhead.

The inside server (`notify.enabled`) sends the report at `notify.send_at`. Each server gets its own section, and the inside server fetches the foreign server's numbers over mutual TLS from `notify.foreign_stats_addr`.

Send a report now:

```bash
cd / && sudo -u portman /usr/local/bin/portman -config /etc/portman/client.json -report-now
```

Limits: sampling runs every `sample_interval`, so an abrupt stop can lose up to one interval of counts. If the inside server is off at `send_at`, that day's report is not sent.

## Configuration reference

JSON numbers must not have leading zeros. Write `billing_day` as `1`, not `01`.

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
| `stats.interface` | both | Network interface to count. Find it with `ip -br addr` |
| `stats.timezone` | both | Day and month boundaries for this server, for example `Asia/Tehran`. Set it to match that server's provider |
| `stats.billing_day` | both | Day of the month when this server's cycle resets (1 to 31, default 1). If the month is shorter, the cycle starts on its last day. Set it to match that server's provider |
| `stats.monthly_limit_gb` | both | Monthly limit in GiB. `0` means no limit line |
| `stats.count_mode` | both | `sum` (received + sent) or `max` (the larger of the two). Match your provider's billing |
| `stats.sample_interval` | both | How often counters are read, for example `1m` (minimum `10s`) |
| `stats.listen_addr` | server | Stats endpoint address, for example `:8444`. Requires `client_ca_file` |
| `notify.enabled` | client | Send the daily traffic report |
| `notify.provider` | client | `bale` (default) or `telegram`. Sets the default bot API host. Both use the same message API |
| `notify.api_base` | client | Bot API base. Default follows `provider` |
| `notify.bot_token` | client | Bot token |
| `notify.chat_id` | client | Chat to send to. Quote it, even if it's a number |
| `notify.send_at` | client | Traffic report send time, `HH:MM`, in `stats.timezone` |
| `notify.foreign_stats_addr` | client | Foreign stats endpoint, for example `bridge.example.com:8444` |
| `checks.enabled` | client | Run connectivity checks and send the result on the scheduled days |
| `checks.send_days` | client | Days to send, in English, for example `["friday"]` |
| `checks.send_at` | client | Time to send, `HH:MM`, in `checks.timezone` |
| `checks.timezone` | client | Time zone for the schedule, for example `Asia/Tehran`. Default `UTC` |
| `checks.targets_file` | client | JSON file with the groups and IP addresses to check |

## Example configuration

Install these as `/etc/portman/server.json` on the foreign server and `/etc/portman/client.json` on the inside server. They are also in `examples/`.

Replace the placeholders, and set `billing_day`, `timezone`, and `interface` to match each server.

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
    "timezone": "Europe/Berlin",
    "billing_day": 1,
    "monthly_limit_gb": 0,
    "count_mode": "sum",
    "sample_interval": "1m",
    "listen_addr": ":8444"
  }
}
```

The foreign server must not contain `notify` or `checks` blocks. It refuses to start with them.

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
    "billing_day": 1,
    "monthly_limit_gb": 100,
    "count_mode": "sum",
    "sample_interval": "1m"
  },
  "notify": {
    "enabled": true,
    "provider": "bale",
    "api_base": "https://tapi.bale.ai",
    "bot_token": "PASTE_BOT_TOKEN",
    "chat_id": "PASTE_CHAT_ID",
    "send_at": "23:55",
    "foreign_stats_addr": "bridge.example.com:8444"
  },
  "checks": {
    "enabled": true,
    "send_days": ["friday"],
    "send_at": "07:00",
    "timezone": "Asia/Tehran",
    "targets_file": "/etc/portman/targets.json"
  }
}
```

Before you start, change these values:

- `stats.interface`: the interface that holds the server's public address. Find it with `ip -br addr`. Often `eth0`, but it may be `ens3` or `ens160`. Set it separately on each server.
- `timezone` and `billing_day`: your provider's time zone and reset day. Each server has its own.
- `monthly_limit_gb`: your provider's limit in GiB.
- `count_mode`: `sum` if your provider bills in + out, `max` if it bills the larger of the two.
- `bot_token` and `chat_id`: the values from step 3.8.
- `provider`: `bale` or `telegram`.

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

Copy `ca.pem` to the foreign server. Copy `client.pem`, `client.key`, and `targets.json` to the inside server. Use `scp` or your file manager. Never copy `ca.key` to either server.

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
sudo apt install -y acl curl python3 ufw iputils-ping
```

Test:

```bash
command -v setfacl curl python3 ufw ping
```

All five paths should print. `iputils-ping` provides the `ping` command the checks use.

#### 1.4 Install the binary

Download `portman-<version>-linux-amd64` from the release page, then:

```bash
sudo install -m 0755 portman-<version>-linux-amd64 /usr/local/bin/portman
/usr/local/bin/portman -v
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

Paste the `server.json` block from the Example configuration section, and set `stats.interface`, `timezone`, and `billing_day` for this server. Then set the permissions:

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

Test: `sudo ufw status | grep -E '4443|8444'` should list only the two `INSIDE_IP` rules. Also check the provider's firewall panel, which is separate from ufw. It must allow the same ports from the inside server's IP.

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
sudo install -o root -g portman -m 0644 /tmp/client.pem /etc/portman/client.pem
sudo install -o root -g portman -m 0640 /tmp/client.key /etc/portman/client.key
```

Test:

```bash
sudo -u portman test -r /etc/portman/client.key && echo "key readable"
```

The first line should print `key readable`. The client certificate was signed by the CA on your own machine in step 0, so verify it there rather than on this server.

#### 3.2 Install the client config and the targets file

Find the interface name first:

```bash
ip -br addr
```

Then open the config:

```bash
sudo nano /etc/portman/client.json
```

Paste the `client.json` block from the Example configuration section, and set `stats.interface`, `timezone`, `billing_day`, `bot_token`, and `chat_id`. Step 3.8 explains how to get the last two. Then set permissions:

```bash
sudo chown root:portman /etc/portman/client.json
sudo chmod 0640 /etc/portman/client.json
```

Install the targets file from the copy you made in step 0 (or the one you prepared for checks):

```bash
sudo install -o root -g portman -m 0640 /tmp/targets.json /etc/portman/targets.json
sudo python3 -m json.tool /etc/portman/targets.json > /dev/null && echo "targets JSON OK"
```

The targets file is needed on the inside server only.

#### 3.3 Check the config

```bash
sudo python3 -m json.tool /etc/portman/client.json > /dev/null && echo "JSON OK"
cd / && sudo -u portman timeout 5 /usr/local/bin/portman -config /etc/portman/client.json; echo "exit=$?"
```

Expected: `JSON OK`, then `exit=124`. Anything else prints the error. Common causes:

- `client_ca_file is for server mode only`: remove that line from the client config.
- `client_cert_file and client_key_file must be set together`: add the missing one.
- `notify.chat_id is required` or `bot_token is required`: fill in the bot values.
- `json: cannot unmarshal number into Go struct field`: quote `chat_id`.
- `decode config: invalid character '1' after object key:value pair`: a number has a leading zero, such as `01`. Write `1`.

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

The client unit grants `CAP_NET_RAW` so `ping` works under `NoNewPrivileges`. Check that these two lines are under `[Service]` in `deploy/portman-client.service`:

```ini
AmbientCapabilities=CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_RAW
```

Then install and start it:

```bash
sudo install -m 0644 deploy/portman-client.service /etc/systemd/system/portman-client.service
sudo systemctl daemon-reload
sudo systemctl enable --now portman-client
systemctl show portman-client -p AmbientCapabilities
```

The last command should print `AmbientCapabilities=cap_net_raw`.

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
- `next connectivity check at ...`

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

Expected: `"label": "foreign"` near the top, and a `snapshot` section. The response includes `cycle_start`, which shows the billing cycle date this server uses. Compare it with the `billing_day` you set.

Without a certificate, it must refuse:

```bash
curl -sS --max-time 10 https://bridge.example.com:8444/stats
```

Expected: a certificate-required error.

#### 3.8 Set up the bot and test the report

1. Create a bot in Bale (or Telegram, and set `provider` to `telegram`), and save its token.
2. Put the token in `bot_token` in `/etc/portman/client.json`.
3. Send any message to the bot from the account that should receive reports.
4. Read the token from the config without pasting it anywhere, and get the chat ID. For Bale:

```bash
TOKEN=$(sudo python3 -c 'import json;print(json.load(open("/etc/portman/client.json"))["notify"]["bot_token"])')
curl -sS "https://tapi.bale.ai/bot${TOKEN}/getMe"
curl -sS "https://tapi.bale.ai/bot${TOKEN}/getUpdates"
```

For Telegram, use `https://api.telegram.org` in place of `https://tapi.bale.ai`.

`getMe` should return `"ok":true`. In `getUpdates`, find `"chat":{"id":...}` and put that number in `chat_id`, in quotes. Then restart the service:

```bash
sudo systemctl restart portman-client
```

Send a traffic report now. Run it as the `portman` user, from a directory it can access:

```bash
cd / && sudo -u portman /usr/local/bin/portman -config /etc/portman/client.json -report-now; echo "exit=$?"
```

Expected: `exit=0` and a message in the bot with the `[inside]` section, plus a `[foreign]` section. If the foreign section says it's not reachable, return to the firewall checks in 2.7 and 3.7.

Send a connectivity check now:

```bash
cd / && sudo -u portman /usr/local/bin/portman -config /etc/portman/client.json -checks-now; echo "exit=$?"
```

Expected: `exit=0` and a bot message with one section per group.

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

Send a traffic report or a connectivity check now, always as `portman`:

```bash
cd / && sudo -u portman /usr/local/bin/portman -config /etc/portman/client.json -report-now
cd / && sudo -u portman /usr/local/bin/portman -config /etc/portman/client.json -checks-now
```

Change the target list: edit `/etc/portman/targets.json` and keep the file's mode `0640` with group `portman`. The service reads the file on every check, so no restart is needed. Check the JSON after each edit:

```bash
sudo python3 -m json.tool /etc/portman/targets.json > /dev/null && echo "targets JSON OK"
```

Retire the old cron script: once the first scheduled connectivity report arrives on its real day, remove only the `check_net.sh` line from the crontab. Check first:

```bash
crontab -l
sudo crontab -l
```

Keep the script file until one full cycle has passed without problems.

Update to a new release:

```bash
sudo cp /usr/local/bin/portman /usr/local/bin/portman.prev
sudo install -m 0755 portman-<new-version>-linux-amd64 /usr/local/bin/portman
sudo systemctl restart portman-client    # or portman-server
/usr/local/bin/portman -v
```

Roll back:

```bash
sudo install -m 0755 /usr/local/bin/portman.prev /usr/local/bin/portman
sudo systemctl restart portman-client    # or portman-server
```

Turn the traffic report off: set `notify.enabled` to `false` in `client.json`, then restart. The bridge keeps running. Turn the connectivity checks off: set `checks.enabled` to `false`.

Reset the counters: stop the service, move the state file aside, and start again. This erases the history.

```bash
sudo systemctl stop portman-client
sudo mv /var/lib/portman/state.json /var/lib/portman/state.json.old
sudo systemctl start portman-client
```

### File permissions

Each file in `/etc/portman` must be readable by the `portman` group, because the service runs as `portman`. Configs and keys hold tokens and private data, so they're `0640`. Public certificates are `0644`.

```bash
sudo chown root:portman /etc/portman
sudo chmod 0750 /etc/portman

sudo chown root:portman /etc/portman/client.json /etc/portman/targets.json
sudo chmod 0640 /etc/portman/client.json /etc/portman/targets.json

sudo chown root:portman /etc/portman/client.key /etc/portman/client.pem
sudo chmod 0640 /etc/portman/client.key
sudo chmod 0644 /etc/portman/client.pem
```

On the foreign server, use `server.json` and the server key in place of `client.json` and `client.key`, and omit `targets.json` and `client.pem`.

Check that the service user can read each file:

```bash
sudo -u portman test -r /etc/portman/client.json && echo "client.json readable"
sudo -u portman test -r /etc/portman/targets.json && echo "targets readable"
sudo -u portman test -r /etc/portman/client.key && echo "key readable"
sudo -u portman test -r /etc/portman/client.pem && echo "cert readable"
systemctl is-active portman-client
```

### Troubleshooting

| Symptom | Check | Fix |
|---|---|---|
| Service shows `activating (auto-restart)` | `sudo journalctl -u portman-client -n 50` | Run the binary manually as `portman` (step 3.3). It prints the exact error. |
| `invalid config: ...` | The message names the field | Fix that field. Mode-specific settings are the usual cause. |
| `decode config: invalid character '1' after object key:value pair` | A number with a leading zero, such as `"billing_day": 01` | Write `1`. JSON doesn't allow leading zeros. |
| `flag provided but not defined: -v` | Binary older than 0.7.1 | Install 0.7.1 or later, then use `-v` or `-version`. |
| `Failed to open log file: permission denied` | `ls -l /var/log/portman` | `sudo chown -R portman:portman /var/log/portman`. A root-run command created the file. |
| Server log: `certificate required` | Client config | Add `client_cert_file` and `client_key_file`, and check that the client certificate is signed by `ca.pem`. |
| Client log: `unknown authority` or `bad certificate` | Server config and `ca.pem` | `client_ca_file` must be the CA that signed `client.pem`. |
| Client log: `certificate is valid for ..., not ...` | `server_name` | Set `server_name` and the hostname to the certificate name. |
| Stats: `context deadline exceeded` | Step 3.7 | Firewall: allow 8444 from `INSIDE_IP`, and check the provider's firewall panel. |
| Stats: `connection refused` | `ss -tlnp \| grep 8444` on foreign | The service isn't listening. Check its journal. The stats endpoint needs 0.5.0 or later. |
| Report has only `[inside]` | Journal: `foreign stats unavailable` | Same as the stats row above. |
| Report shows zero counts | `sudo cat /var/lib/portman/state.json`; journal for `stats save failed` | Check ownership of `/var/lib/portman`. A one-shot run has no baseline, so the scheduled service provides the numbers. |
| Bot `HTTP 401` | Token | Copy the token again from the bot creator. |
| Bot `chat not found` | `chat_id` | Message the bot, then read `getUpdates` again. |
| Bridge connects, but nothing passes | Foreign log shows `0 bytes` | The inside outbound's ID doesn't match the foreign inbound client. |
| Pages are slow | Client log `dial took` | Each new connection pays a handshake. See the roadmap for connection reuse. |
| `read targets file: permission denied` | `ls -l /etc/portman` and `ls -ld /etc/portman` | Apply the File permissions section. The directory needs `root:portman` with mode `0750`. |
| Checks show `ping no reply` with `-checks-now` working in a terminal | `systemctl show portman-client -p AmbientCapabilities` | Add `AmbientCapabilities=CAP_NET_RAW` and `CapabilityBoundingSet=CAP_NET_RAW` to the client unit, then `daemon-reload` and restart. |
| Checks show `PARTIAL` with ping OK and `DNS no answer` | The address's resolver | The resolver refuses queries from outside its network. This is expected for some ISP resolvers. Treat it as "host up, DNS closed to us." |
| Config readable by other users | `ls -l /etc/portman/client.json` | Set mode `0640` (see File permissions). The config holds the bot token. |

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
- The traffic report is sent only if the inside server is running at `send_at`. Missed reports are not sent later.
- Interface counters use `/proc/net/dev`, so they work on Linux only.
- Each new local connection pays a new TLS handshake, which adds latency on long links.
- DNS results depend on the resolver. Some ISP resolvers refuse outside queries, so their addresses show `PARTIAL` even when the host is up.
- The connectivity checks use the system `ping` command. It must be installed, and the client unit must grant `CAP_NET_RAW`.

## License

MIT. See [LICENSE](LICENSE).