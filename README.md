# Porter

[English](README.md) · [简体中文](README.zh-CN.md)

Porter is a self-hosted web panel for TCP and UDP port forwarding on one Linux server. Manage forwarding rules from a Chinese-language interface, with SQLite persistence and a single-container deployment.

- Create, inspect, edit, delete, enable, disable, and retry TCP, UDP, or combined TCP+UDP rules.
- Filter by protocol or state, and search names and addresses.
- Listen on explicit IPv4 or IPv6 addresses; forward to IP addresses or hostnames.
- Restore enabled rules after restart, without letting one occupied port block the others.
- Configure the administrator through environment variables; no registration or external database.

Porter uses ordinary proxy connections. The destination sees the forwarding server's address. It does not preserve the client's source IP or manage host firewalls and cloud security groups. A running rule means its local listener is ready, not that its destination is reachable. Editing forwarding parameters, disabling or deleting a rule ends its active sessions; renaming alone preserves them. Restarting or upgrading interrupts all sessions.

## Quick start

Use native Linux Docker Engine and Docker Compose v2. The deployment uses host networking so newly added forwarding ports need no Compose changes.

**The image address below is the planned release location. Before the first successful release and public GHCR package configuration, the image may not be available.** Build from source using the instructions below in that case.

Save this as `compose.yaml`:

```yaml
services:
  porter:
    image: ghcr.io/lwl0820/porter:latest
    restart: unless-stopped
    network_mode: host
    cap_drop: [ALL]
    cap_add: [NET_BIND_SERVICE]
    environment:
      PORTER_ADMIN_USERNAME: admin
      PORTER_ADMIN_PASSWORD: ${PORTER_ADMIN_PASSWORD:?Set PORTER_ADMIN_PASSWORD first}
    volumes:
      - porter-data:/data

volumes:
  porter-data:
```

Create a private `.env` alongside it. Replace the example with a long, unique password; single quotes preserve literal `$` and `#` characters in Compose `.env` files.

```dotenv
PORTER_ADMIN_PASSWORD='replace-with-a-long-unique-password'
```

```bash
chmod 600 .env
docker compose up -d
docker compose logs -f porter
```

Open `http://SERVER_IP:8080` on your trusted management network and sign in as `admin` using your configured password. Change the username directly in `compose.yaml`. Changing credentials requires recreating the container (`docker compose up -d --force-recreate`); existing login sessions expire.

Allow the management port only from your management network. For management over the internet, use an external HTTPS reverse proxy, bind `PORTER_HTTP_ADDR` to its trusted backend interface, and set `PORTER_COOKIE_SECURE: "true"`. The reverse proxy must preserve the browser-facing `Host` header: Porter compares the request's Origin with Host and does not trust forwarded headers. Business TCP/UDP traffic is encrypted only when its own protocol provides encryption. Allow required forwarding ports in the server firewall and any cloud security group yourself.

The container runs as UID 0 with all capabilities dropped except `NET_BIND_SERVICE` for low ports. It needs no privileged mode, `NET_ADMIN`, or Docker socket. Host networking is intentional; do not add `ports` mappings. Run one instance per data volume on a local filesystem.

## Configuration

| Environment variable | Default | Meaning |
| --- | --- | --- |
| `PORTER_ADMIN_USERNAME` | Required | Administrator username; empty values prevent startup. |
| `PORTER_ADMIN_PASSWORD` | Required | Administrator password; no built-in password. |
| `PORTER_HTTP_ADDR` | `0.0.0.0:8080` | Explicit local IPv4/IPv6 address and port 1–65535, such as `127.0.0.1:8080` or `[::1]:8080`; hostnames are not accepted. |
| `PORTER_DB_PATH` | `/data/porter.db` | SQLite file path; persist its entire directory. |
| `PORTER_COOKIE_SECURE` | `false` | Set to `true` when the browser accesses the panel over HTTPS. |
| `PORTER_MAX_SESSIONS` | `4096` | Global TCP connections and UDP sessions combined. |
| `PORTER_MAX_SESSIONS_PER_RULE` | `1024` | Combined TCP connection and UDP session limit for each logical rule, including TCP+UDP. |
| `PORTER_UDP_IDLE_TIMEOUT` | `60s` | UDP session idle timeout, using Go duration syntax. |
| `PORTER_DIAL_TIMEOUT` | `5s` | Upstream connection timeout, using Go duration syntax. |

Rules accept individual ports from 1 to 65535. TCP and UDP can share the same numeric port. Listen addresses must be local IP addresses; enter IPv6 addresses such as `::` without brackets in rule forms. Destinations accept an IP or hostname separately from the port. Hostnames resolve for each new TCP connection or UDP session, not each datagram. UDP has no connection handshake and does not try another resolved address just because a destination does not respond; use an explicitly reachable IP if only some of a hostname's addresses are reachable. Invalid fields, occupied listeners, and management-port conflicts are reported by the panel. For changes between overlapping listeners, disable the rule before changing its listen address.

Choose **TCP+UDP** to manage both protocols as one rule with one name, revision, enabled state, listen IP/port, and destination host/port. Both protocols use the same numeric destination port; the destination must provide the intended TCP and UDP services there. The rule counts once and shares its session allowance across TCP connections and UDP sessions. Enable, disable, retry, edit, and delete affect both protocols together. A conflict on either listen port fails the entire operation and preserves the existing configuration and sessions. A combined rule is running only while both listeners are ready; if one fails, the other is stopped and the whole rule reports an error. TCP and UDP filters include combined rules, while the TCP+UDP filter shows only combined rules. Existing separate TCP and UDP rules remain separate and are not automatically merged.

Logs go to standard output. `/healthz` reports process and database readiness. The SQLite database stores rule configuration; administrator passwords and login sessions are not persisted in it. Browser login sessions expire after 12 hours or a process restart.

## Build from source

The pinned build environment uses Go 1.27.2 and Node.js 24.22.0 LTS. Local Docker builds require Docker Buildx and need no host Go or Node.js installation.

```bash
git clone https://github.com/lwl0820/porter.git
cd porter
docker build -t porter:local .
```

In the quick-start Compose file, replace the image line with `image: porter:local`, then follow the same `.env` and startup steps. After source changes, rebuild the image and recreate the container.

To build without Docker, keep temporary files and caches in the repository:

```bash
mkdir -p .tmp/tmp .tmp/go-cache .tmp/go-path .tmp/go-mod .tmp/npm-cache
export TMPDIR="$PWD/.tmp/tmp" GOTMPDIR="$PWD/.tmp/tmp"
export GOCACHE="$PWD/.tmp/go-cache" GOPATH="$PWD/.tmp/go-path" GOMODCACHE="$PWD/.tmp/go-mod"
export npm_config_cache="$PWD/.tmp/npm-cache"
npm --prefix web ci
npm --prefix web run build
CGO_ENABLED=0 go build -trimpath -ldflags '-X main.version=dev' -o .tmp/porter ./cmd/porter
```

The resulting program embeds the web interface. No separate frontend server is required. Run it as follows, or extract the matching Linux archive from [GitHub Releases](https://github.com/lwl0820/porter/releases):

```bash
mkdir -p data
export PORTER_ADMIN_USERNAME=admin
read -rsp 'Administrator password: ' PORTER_ADMIN_PASSWORD
echo
export PORTER_ADMIN_PASSWORD
export PORTER_DB_PATH="$PWD/data/porter.db"
./.tmp/porter
```

For a release archive, use `./porter` instead. Download `SHA256SUMS` alongside the archive and verify it with `sha256sum --ignore-missing -c SHA256SUMS`. A normal user can bind high ports; low ports require suitable service privileges or `CAP_NET_BIND_SERVICE`. Use a service supervisor for automatic restarts.

## Upgrade, backup, and rollback

Pin a version tag such as `v1.0.0` for repeatable deployments once that release exists. Before upgrading, stop Porter and back up the **entire data volume**, including SQLite journal files. Do not copy a live database file in isolation or share its directory between replicas.

TCP+UDP support upgrades the SQLite schema from version 1 to version 2 on startup, preserving existing rule IDs, revisions, timestamps, configuration, and enabled states. Older Porter programs reject a version 2 database. To downgrade, restore the complete backup taken before upgrading together with the older program; do not open the migrated database with an older binary.

Find your Compose volume name with `docker volume ls` (usually `<project>_porter-data`). For example, replace `YOUR_PORTER_VOLUME` below with that name:

```bash
docker compose stop porter
mkdir -p backups
docker run --rm -v YOUR_PORTER_VOLUME:/data:ro -v "$PWD/backups:/backup" \
  alpine:3.24.2 tar -czf /backup/porter-before-upgrade.tar.gz -C /data .
docker compose pull
docker compose up -d
```

For a locally built image, rebuild instead of pulling. After upgrading, check logs, login, rule state, and actual TCP/UDP forwarding. To roll back, stop Porter, select the previous image or binary, and restore the complete pre-upgrade backup into an empty data volume. A newer database schema may require restoring the old data together with the old program. Never use `docker compose down -v` unless you intend to delete saved rules.

## Compatibility and validation

Production targets are native Linux Docker Engine with Compose v2, on `amd64` and `arm64`: Debian 12/13, Ubuntu 22.04/24.04, RHEL 9/10, and compatible Rocky Linux/AlmaLinux 9/10. Rootless Docker, Docker Desktop, Windows, and macOS are outside the initial production scope.

These are compatibility targets, not claims that every host/architecture combination has been tested. CI is configured to run native amd64 and arm64 Linux checks, real TCP/UDP integration tests, and Docker smoke tests. A successful CI run confirms those runner environments only. Debian, Ubuntu, and Red Hat family host deployment results must be recorded separately; running a distribution's container does not validate that distribution as a host. See the repository's actual workflow results before relying on a release.

## Development and publishing

After setting the cache environment above:

```bash
npm --prefix web run lint
npm --prefix web run type-check
npm --prefix web run test
npm --prefix web run build
go vet ./...
go test -race ./...
go test -count=1 ./tests/integration/...
docker build -t porter:local .
python3 scripts/docker-smoke.py porter:local
VERSION=v1.0.0 bash scripts/build-release.sh
```

The final command only builds local Linux amd64/arm64 archives and `SHA256SUMS` under `.tmp/release`; it does not publish them. Pull requests validate the application without publishing. Each push to `main` runs validation and, if it succeeds, publishes `ghcr.io/lwl0820/porter:latest` for Linux amd64/arm64 without requiring a version tag. These builds identify their source commit and do not create a GitHub Release.

Pushing a version tag such as `v1.0.0` runs validation before publishing the versioned multi-architecture GHCR image, updating `latest`, and creating GitHub Release archives. Tags such as `v1.0.0-rc.1` publish prereleases and do not replace `latest`. Since `latest` can follow an untagged `main` build, use an explicit release tag for deployments that should stay on a fixed version. Workflow names, steps, and workflow-owned messages use English. Actions are pinned to commit SHAs and publish using the repository's `GITHUB_TOKEN`.

Before the first public release, enable the repository's required Actions permissions, verify package publishing is permitted, set the GHCR package visibility to public, and confirm an unauthenticated image pull works. The repository configuration does not itself publish a release.
