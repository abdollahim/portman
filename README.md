# Portman

A small TLS tunnel that forwards TCP traffic between two servers.

A client listens on a local port, wraps each incoming connection in TLS, and sends it to a server. The server completes the TLS handshake and forwards the decrypted stream to a local backend such as Xray or 3X-UI.

Portman does not parse the traffic. It is a byte pipe, so the backend must accept exactly what the client sends. For example, a VLESS outbound on the client side needs a VLESS inbound on the backend, with a matching identity.

## Status

Experimental (v0.1.0). The TLS tunnel and logging work, but the project has not yet been tested with a full production workload.

## Versioning

Releases use [Semantic Versioning](https://semver.org/) and are tagged `vX.Y.Z`. Until 1.0.0, a minor version may change configuration or behavior. See [CHANGELOG.md](CHANGELOG.md) for what changed in each release.

## Requirements

- Go 1.21 or newer
- A hostname with a valid certificate on the server (Let's Encrypt works)
- Linux with systemd, if you want to run it as a service

## Build

```bash
go build -o dist/portman ./cmd/portman
```

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
| `log_level` | both | `debug`, `info`, `warn`, or `error` |
| `log_path` | both | Log file; output also goes to stdout |

Examples are in `config.server.example.json` and `config.client.example.json`.

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

At `debug`, each connection logs its ID, its byte counts per direction, and its duration. At `info`, it logs bridging and close events. Use `info` in production, because `debug` logs every connection.

Startup checks log whether the remote TCP port and TLS handshake are reachable. They only log results and never stop the service.

## Security notes

- Portman does not authenticate clients. Anyone who can reach the server's listen port can open a TLS connection and send data to the backend. Keep the backend bound to `127.0.0.1` and protect it with its own authentication (for example, a VLESS UUID).
- Do not commit real configs, certificates, or logs. `.gitignore` excludes the usual names.

## Limitations

- No authentication, multiplexing, or connection pooling. Each local connection opens its own TLS connection.
- No protocol awareness. Protocol and identity must match between client-side and backend.
- One listener per process.

## License

MIT. See [LICENSE](LICENSE).
EOF