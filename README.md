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
| `log_level` | both | `debug`, `info`, `warn`, or `error` |
| `log_path` | both | Log file; output also goes to stdout |

Examples are in `configs/server.example.json` and `configs/client.example.json`. Copy one and edit it. Don't commit your real config.

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

## Security notes

- Portman does not authenticate clients. Anyone who can reach the server's listen port can open a TLS connection and send data to the backend. Keep the backend bound to `127.0.0.1` and protect it with its own authentication (for example, a VLESS UUID).
- Do not commit real configs, certificates, or logs. `.gitignore` excludes the usual names.

## Limitations

- No authentication, multiplexing, or connection pooling. Each local connection opens its own TLS connection, so each new connection pays the handshake cost. Enabling multiplexing in the backend (for example, Xray mux) avoids this.
- No protocol awareness. Protocol and identity must match between client-side and backend.
- One listener per process.

## License

MIT. See [LICENSE](LICENSE).