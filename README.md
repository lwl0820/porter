# Porter

[English](README.md) · [简体中文](README.zh-CN.md)

Porter is a self-hosted web panel for TCP and UDP port forwarding on one Linux server. Manage forwarding rules with SQLite persistence and a single-container deployment.

- Create, inspect, edit, delete, enable, disable, and retry TCP, UDP, or combined TCP+UDP rules.
- Filter by protocol or state, and search names and addresses.
- Listen on explicit IPv4 or IPv6 addresses; forward to IP addresses or hostnames.
- Restore enabled rules after restart, without letting one occupied port block the others.
- Separate administrator and user portals; inspect rule owners, filter by user, and administer every user’s rules.
- Create plans and users, allocate exclusive contiguous ports, and enforce four accounting modes with three UTC reset schedules.
- Configure the administrator through environment variables; ordinary users are created by the administrator, without self-registration or an external database.
- Manage rules from a bilingual interactive menu or parameter commands on native Linux installations.

Porter uses ordinary proxy connections. The destination sees the forwarding server's address. It does not preserve the client's source IP or manage host firewalls and cloud security groups. A running rule means its local listener is ready, not that its destination is reachable. Editing forwarding parameters, disabling or deleting a rule ends its active sessions; renaming alone preserves them. Restarting or upgrading interrupts all sessions.

## Quick start

### Native interactive installation

Run on a Linux server using systemd, such as an SSH terminal:

```bash
curl -fsSL https://raw.githubusercontent.com/lwl0820/porter/main/scripts/install.sh | bash
```

Supports Ubuntu 22.04/24.04, Debian 12/13, RHEL/Rocky Linux/AlmaLinux 9/10 on amd64 and arm64. Requires Bash, curl for the download command, and root or sudo privileges. The installer adds Python 3, curl and CA certificates from the system repositories when needed and downloads a standalone GitHub Release binary. Docker, Go and Node.js are unnecessary. systemd must be PID 1; ordinary distribution containers are unsupported.

The script first offers **中文 / English**, defaulting to Chinese when you press Enter. Specify `bash -s -- --lang en` or `bash -s -- --lang zh-CN` after the pipe to choose directly. Help is available without a terminal through `bash install.sh --help --lang en`. Installer help, prompts, errors and results follow the chosen language; system command diagnostics retain their original language.

First installation asks for an **admin username, hidden password, password confirmation and Web listen address**. Passwords are neither generated nor echoed. Use a complete `IPv4:port` or `[IPv6]:port`, such as `127.0.0.1:18080`, `0.0.0.0:8080` or `[::]:8080`, with a port from 1 to 65535. For remote access to a wildcard address, replace the displayed loopback IP with the server IP. Restrict the Web port to trusted networks; the installer does not configure a firewall or HTTPS.

The program directory supports spaces, `$` and `%`, but cannot contain quotes or backslashes due to systemd path restrictions. It cannot be inside `/var/lib/porter` or `/etc/porter`. By default the latest stable [GitHub Release](https://github.com/lwl0820/porter/releases) is installed. To specify a published version and directory:

```bash
curl -fsSL https://raw.githubusercontent.com/lwl0820/porter/main/scripts/install.sh | \
  bash -s -- --version v1.0.0 --install-dir /opt/porter --lang en
```

**The installer reports an error if no public Release exists; the latest API may return 404.** Binary Releases are published only after version tags pass validation and image publication. A main-branch container image does not imply a downloadable binary exists. The installer downloads `porter-VERSION-linux-ARCH.tar.gz` and `SHA256SUMS`, validates the exact archive checksum, and never compiles on the server.

Default files and service; `--install-dir` changes only the program directory:

| Resource | Location |
| --- | --- |
| Program | `/opt/porter/porter` |
| Admin and listen configuration | `/etc/porter/porter.env`, mode `600` |
| Installation record | `/etc/porter/install.json`, mode `600` |
| SQLite and journal files | `/var/lib/porter`, writable by the service user |
| systemd unit | `/etc/systemd/system/porter.service` |

The dedicated non-login `porter` user/group runs the service with only `CAP_NET_BIND_SERVICE`, supporting low ports, restart on failure and startup at boot. Configuration uses systemd `EnvironmentFile` syntax: double-quoted values with backslashes before literal backslashes, double quotes, dollar signs and backticks. For example, write a literal `$` as `\$` and a literal `\` as `\\`. Do not source this file or use Compose-style `$$`. Additional `PORTER_*` settings, such as `PORTER_COOKIE_SECURE="true"`, may be appended with the same quoting. Restart with `sudo systemctl restart porter.service` after editing.

Repeating installation in the same directory preserves credentials, listen address, program and database without upgrading. Partial installation failures can be retried using saved credentials. An existing installation rejects a different `--version`. Unknown existing users, services, configuration or data are never taken over; Docker data is not migrated. Success requires an active, enabled service and an exact healthy `/healthz` response at the configured address.

The installer prints both user `/user/` and administrator `/admin/` URLs. The root URL opens the ordinary user portal. Administrators create ordinary user accounts in the admin portal.

Uninstall with the current script. Uninstall needs Python 3 already installed, but does not fetch Releases, install dependencies or request administrator credentials. It detects a custom program directory from the installation record; an explicit `--install-dir` must match:

```bash
curl -fsSL https://raw.githubusercontent.com/lwl0820/porter/main/scripts/install.sh | \
  bash -s -- --uninstall --lang en
```

Review the resource list and type `yes` to confirm, or Enter to cancel. By default uninstall stops and disables the service and removes the managed binary and unit. It retains the **entire** `/var/lib/porter` directory, `/etc/porter/porter.env`, the restricted installation record and the dedicated service account/group. Reinstall with the current script and the original program directory to reuse credentials, listen address and databases. It downloads the **same recorded version**, which must still be available as a Release; a failed download preserves configuration and data. Older scripts do not recognize the uninstalled state and must not be used for recovery. After an interrupted uninstall, retry the same uninstall options before reinstalling.

To permanently remove accounts, rules and statistics, add `--purge`. After `yes`, a second confirmation requires typing `DELETE`:

```bash
curl -fsSL https://raw.githubusercontent.com/lwl0820/porter/main/scripts/install.sh | \
  bash -s -- --uninstall --purge --lang en
```

Purge removes managed configuration, the entire data directory, installation record and dedicated user/group. Both uninstall modes remove the installer's uncommitted `porter.env.new`, `install.json.new`, `porter.service.new` and `.porter.new` files after validating their paths and file types. Other extra files in the program and configuration directories are preserved. Cancellation or absence of an interactive terminal does not start uninstall. Unknown services, modified units, systemd drop-ins, symbolic links and mismatched accounts are rejected. Shared logs, system packages and Docker resources are retained. `--purge` requires `--uninstall`; uninstall rejects `--version`.

A forced interruption may leave historical `.tmp` contents in the program directory, potentially including temporary configuration or credential copies. Both uninstall modes preserve these files because their ownership cannot be confirmed. Inspect them before removing them manually; purge does not remove unknown historical copies.

For sudo users (omit `sudo` when running as root):

```bash
sudo systemctl status porter.service
sudo journalctl -u porter.service -f
sudo systemctl stop porter.service
sudo systemctl start porter.service
```

Before a native upgrade, stop the service and back up the complete data directory, configuration, unit and binary. Do not copy just a live database file. For the default locations:

```bash
sudo systemctl stop porter.service
install -d -m 0700 "$PWD/backups"
sudo tar -czf "$PWD/backups/porter-before-upgrade.tar.gz" -C / \
  etc/porter var/lib/porter etc/systemd/system/porter.service
sudo chmod 0600 "$PWD/backups/porter-before-upgrade.tar.gz"
sudo cp /opt/porter/porter "$PWD/backups/porter.previous"
```

Download the target architecture archive and `SHA256SUMS` from the target Release into a separate `.tmp/porter-upgrade` directory. Verify the archive with `sha256sum --ignore-missing -c SHA256SUMS`, then extract its `porter` binary. Rerunning the installer does not upgrade. Replace the source path below with the verified binary, and adjust the destination for a custom installation:

```bash
sudo install -m 0755 .tmp/porter-upgrade/porter /opt/porter/porter.new
sudo mv /opt/porter/porter.new /opt/porter/porter
sudo systemctl start porter.service
sudo journalctl -u porter.service -n 30 --no-pager
```

Verify login, health and real TCP/UDP traffic. After a manual upgrade, update only the `version` field in `/etc/porter/install.json` to the actual release tag, keeping the remaining fields and mode `600`. For rollback, stop the service and restore the previous binary with its matching **complete data and configuration backup**; an older binary cannot use an upgraded database. Native backups are separate from the Docker volume backups below.

### Native rule commands

Use the installed binary as root or through `sudo` to manage rules with administrator privileges, without entering Web credentials. The Porter service must be running; commands connect to its local Unix socket and changes appear immediately in the Web panel. After upgrading the binary, restart the service to enable the control socket. These commands support native Linux only: Docker images disable both rule commands and the control socket at build time, and native binaries also reject rule management in detected container environments. Use the Web panel for Docker deployments.

Open the interactive menu:

```bash
sudo /opt/porter/porter rules
sudo /opt/porter/porter rules --lang en
```

Without `--lang`, **the first step is choosing 中文 or English**. Empty or invalid choices ask again; no language is selected automatically or saved between calls. Explicit `--lang zh-CN` or `--lang en` skips language selection. The menu offers create, list, details, edit, delete and exit. Select rules from the displayed list; editing can retain current values, and deletion asks for confirmation with cancellation as the default. Canceling an unfinished operation does not submit it; completed operations remain saved. Language selection and menu input use the controlling terminal (`/dev/tty`), keeping standard output and redirected standard input separate. The menu requires a terminal even with `--lang` and does not accept `--json`.

For direct commands or unattended scripts, provide an action, all required arguments, and `--lang`:

```bash
sudo /opt/porter/porter rules add --lang en \
  --name website --protocol tcp --listen-host 0.0.0.0 --listen-port 18081 \
  --target-host example.com --target-port 80 --enabled=true --traffic-enabled=false
sudo /opt/porter/porter rules list --lang en
sudo /opt/porter/porter rules get RULE_ID --lang en --json
sudo /opt/porter/porter rules update RULE_ID --lang en --enabled=false
sudo /opt/porter/porter rules delete RULE_ID --lang en
```

Replace `RULE_ID` with the stable ID returned by create or list. Protocol values are `tcp`, `udp` and `tcp+udp`. Parameter commands never enter a wizard for missing fields. Without a controlling terminal, omitting `--lang` fails before any rule operation. **Parameter-mode `delete` takes effect immediately, without confirmation.**

| Command | Arguments and behavior |
| --- | --- |
| `rules add` | Requires `--name`, `--protocol`, `--listen-host`, `--listen-port`, `--target-host`, `--target-port`; optional `--enabled=true\|false`, `--traffic-enabled=true\|false`, `--owner-id ID`. Defaults: enabled, display statistics off, administrator ownership. |
| `rules list` | All rules, or filter with `--owner-id ID`. Shows stable IDs, owner IDs, status and revisions. |
| `rules get ID` | Full rule details, including configuration, owner, revision and display statistics. |
| `rules update ID` | Set one or more creation configuration fields; omitted fields and ownership remain unchanged. Supports `--revision N`. |
| `rules delete ID` | Delete the specified rule and release its listeners and sessions. Supports `--revision N`. |

Common options `--lang zh-CN|en`, `--socket PATH`, `--json` and `--help` can appear before or after the action; `--lang=en` is also accepted. Use `--enabled=false` or `--traffic-enabled=false` to switch a field off; boolean field options require the equals form. `--json` is for single parameter operations: results go to standard output, errors to standard error, with stable JSON field names and error codes. Byte counters remain exact decimal strings. Exit codes are `0` for success, `1` for operation failure and `2` for usage errors. For help without a running service, use `porter rules --help --lang en`; `porter --version` remains unchanged.

Administrators can manage all rules. Creation with `--owner-id USER_ID` assigns an existing user; that user's allocated ports and quota still apply. Use the existing user ID from the authenticated Web admin API (`GET /api/admin/users`, `user.id` in each account). Rule updates preserve ownership. The CLI manages rules only; user, plan, port-pool, retry and counter-reset administration remains in the Web panel. Updates and deletions check revisions and fail on concurrent changes; reload before retrying. Scripts can pin a previously observed revision with `--revision N`. After a timeout, inspect the result before resubmitting a change.

The installed service uses `/var/lib/porter/porter-control.sock` by default. The socket accepts only actual root peers and is not exposed on the Web port. Service startup creates it beside the database, unless `PORTER_CONTROL_SOCKET` sets another path. The CLI chooses `--socket`, then `PORTER_CONTROL_SOCKET`, then `/var/lib/porter/porter-control.sock`. The parent directory and every ancestor must belong to root or the service user, must not be writable by group or other users, and must not be symbolic links. A directory owned by another user is rejected even if currently read-only, since its owner can change its permissions. For a custom program directory, use its binary path. For a manually started service or a custom database directory, pass the matching socket explicitly:

```bash
sudo /custom/porter/porter rules list --lang en --socket /custom/data/porter-control.sock
```

A connection failure means you should check service status, the socket path and whether the upgraded service has restarted. Commands do not start a second service or edit an offline database.

### 手工 Compose 部署

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
      PORTER_HTTP_ADDR: "0.0.0.0:8080"
      PORTER_TRAFFIC_FLUSH_INTERVAL: "5s"
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

Open `http://SERVER_IP:8080/admin/` on your trusted management network and sign in as `admin` using your configured password. Ordinary users use `http://SERVER_IP:8080/user/`; the root URL redirects to the ordinary user portal. Change the username directly in `compose.yaml`. To use Web port 18080, change `PORTER_HTTP_ADDR` to `"0.0.0.0:18080"`, then access `http://SERVER_IP:18080`. Host networking uses the configured port directly and needs no `ports` mapping.

Allow the management port only from your management network. For management over the internet, use an external HTTPS reverse proxy, bind `PORTER_HTTP_ADDR` to its trusted backend interface, and set `PORTER_COOKIE_SECURE: "true"`. The reverse proxy must preserve the browser-facing `Host` header: Porter compares the request's Origin with Host and does not trust forwarded headers. Business TCP/UDP traffic is encrypted only when its own protocol provides encryption. Allow required forwarding ports in the server firewall and any cloud security group yourself.

The container runs as UID 0 with all capabilities dropped except `NET_BIND_SERVICE` for low ports. It needs no privileged mode, `NET_ADMIN`, or Docker socket. Host networking is intentional; do not add `ports` mappings. Run one instance per data volume on a local filesystem.

## Configuration

The following table lists all Porter runtime environment variables. Optional variables use their defaults when unset or set to an empty string. Both administrator variables are required: an empty or whitespace-only username or password prevents startup; surrounding whitespace is trimmed from the username. Invalid boolean, address, integer, or duration formats and out-of-range values prevent startup.

To customize any other variable, add it under the service's `environment` in `compose.yaml`, for example `PORTER_UDP_IDLE_TIMEOUT: "2m"`. The adjacent `.env` file supplies values for Compose substitution; placing a variable there alone does not pass it into the container. After changing environment settings, run `docker compose up -d --force-recreate` to apply them. Recreating the container interrupts all forwarding sessions and expires existing login sessions.

| Environment variable | Default | Meaning |
| --- | --- | --- |
| `PORTER_ADMIN_USERNAME` | Required; no default | Administrator username. |
| `PORTER_ADMIN_PASSWORD` | Required; no default | Administrator password. |
| `PORTER_HTTP_ADDR` | `0.0.0.0:8080` | Explicit local IPv4/IPv6 address and port 1–65535, such as `127.0.0.1:8080` or `[::1]:8080`; hostnames are not accepted. |
| `PORTER_DB_PATH` | `/data/porter.db` | SQLite file path; persist its entire directory. |
| `PORTER_CONTROL_SOCKET` | `porter-control.sock` beside the database | Native Linux service control socket path; the CLI defaults to `/var/lib/porter/porter-control.sock` unless this variable or `--socket` is set. Ignored by Docker builds, which disable local rule management. |
| `PORTER_COOKIE_SECURE` | `false` | Boolean; set to `true` when the browser accesses the panel over HTTPS so login cookies are sent only over HTTPS. |
| `PORTER_MAX_SESSIONS` | `4096` | Global TCP connection and UDP session limit combined; integer from 1 to 1000000. |
| `PORTER_MAX_SESSIONS_PER_RULE` | `1024` | TCP connection and UDP session limit combined for each logical rule; integer from 1 to 1000000. TCP+UDP shares this allowance across both protocols. |
| `PORTER_UDP_IDLE_TIMEOUT` | `60s` | UDP session idle timeout; Go duration from `1ms` to `24h`, for example `500ms`, `60s`, or `2m`. |
| `PORTER_DIAL_TIMEOUT` | `5s` | Upstream connection timeout; Go duration from `1ms` to `24h`, for example `500ms`, `5s`, or `1m`. |
| `PORTER_TRAFFIC_FLUSH_INTERVAL` | `5s` | Persistence interval for user quota usage and per-rule display counters; Go duration from `1ms` to `24h`, such as `1s` or `30s`. Shorter intervals increase disk writes; longer intervals increase possible unpersisted usage loss after an abnormal exit. Independent of live quota enforcement, UTC reset boundaries, and page polling. |

Rules accept individual ports from 1 to 65535. TCP and UDP can share the same numeric port. Listen addresses must be local IP addresses; enter IPv6 addresses such as `::` without brackets in rule forms. Destinations accept an IP or hostname separately from the port. Hostnames resolve for each new TCP connection or UDP session, not each datagram. UDP has no connection handshake and does not try another resolved address just because a destination does not respond; use an explicitly reachable IP if only some of a hostname's addresses are reachable. Invalid fields, occupied listeners, and management-port conflicts are reported by the panel. For changes between overlapping listeners, disable the rule before changing its listen address.

Choose **TCP+UDP** to manage both protocols as one rule with one name, revision, enabled state, listen IP/port, and destination host/port. Both protocols use the same numeric destination port; the destination must provide the intended TCP and UDP services there. The rule counts once and shares its session allowance across TCP connections and UDP sessions. Enable, disable, retry, edit, and delete affect both protocols together. A conflict on either listen port fails the entire operation and preserves the existing configuration and sessions. A combined rule is running only while both listeners are ready; if one fails, the other is stopped and the whole rule reports an error. TCP and UDP filters include combined rules, while the TCP+UDP filter shows only combined rules. Existing separate TCP and UDP rules remain separate and are not automatically merged.

Logs go to standard output. `/healthz` reports process and database readiness. SQLite stores users, plans, port allocations, rules, and traffic counters. Ordinary-user passwords are stored only as salted password derivations; administrator passwords and login sessions are not persisted. Browser login sessions expire after 12 hours or a process restart.

## Users, plans, and separate portals

The admin portal `/admin/` and user portal `/user/` share one server while keeping their entry points, login flows, navigation, cookies, and CSRF state separate. Both can be signed in within one browser; signing out of one does not sign out of the other. Administrators see all rule owners, filter by user, and directly create, edit, enable, disable, delete, retry, and reset display counters for any user’s rules. Ordinary users can access only their own resources.

Configure a user port pool, create a plan with a port count and traffic allowance in GiB, then create a user with that plan. A user receives an exclusive contiguous numeric port segment, shared across TCP, UDP, and listen IPs; destination ports remain unrestricted. Allocation excludes management and existing administrator rule ports. Insufficient contiguous space fails the entire user creation. Changing the pool affects only later allocations. Administrator-owned rules have no plan quota but cannot occupy already assigned user ports; administrator actions on user-owned rules respect the owner’s port and quota limits.

Quota accounting is always active and independent of optional rule display statistics. Disabling or resetting display counters, editing a rule, or deleting it does not refund quota usage. Let U and D be successful upstream and downstream payload bytes aggregated across all of the user’s rules in the current cycle:

| Accounting mode | Charged usage |
| --- | --- |
| Bidirectional | U + D |
| Single direction | max(U, D) |
| Upload only | U |
| Download only | D |

Single-direction accounting takes the maximum after aggregation, not the sum of per-rule maxima. Exhaustion actively stops all of that user’s listeners and sessions without waiting for polling or persistence; other users keep forwarding. Writes already in flight can cause a small overrun. The next cycle resumes rules still configured as enabled; manually disabled or deleted rules stay stopped.

Plans reset on the first of each calendar month, monthly on the account’s UTC opening date, or on a fixed day 1–31. All boundaries use **00:00 UTC**. Missing dates use the month’s last day without moving the original anchor in later months. New users receive the full allowance until their next reset; unused allowance does not roll over. Restarting after missed cycles enters the current cycle without granting multiple allowances. The user overview shows the plan, allocated ports, cycle usage, allowance, and next UTC reset time.

API namespaces are `/api/admin/*` and `/api/user/*`, each with separate `auth/login`, `auth/session`, and `auth/logout`. Rule endpoints are `/api/admin/rules` (optional `owner_id` filter) and `/api/user/rules`. The old shared `/api/auth/*` and `/api/rules*` routes no longer serve business requests; update scripts when upgrading. Existing environment-configured administrator credentials and rules are preserved.

Quota and display counters are persisted every five seconds by default, configurable through `PORTER_TRAFFIC_FLUSH_INTERVAL`, with a final save on normal shutdown. An abnormal exit can lose unpersisted increments; sustained storage failures enlarge this window. Rule display statistics can be enabled and reset independently without affecting plan usage.

This release supports creating and viewing users and plans, without editing/deleting them, switching plans, password resets, self-registration, or payments.

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

On startup, older SQLite databases migrate through schema version 4: version 2 adds TCP+UDP, version 3 adds display statistics, and version 4 adds users, plans, port allocations, and current-cycle quota usage. Existing rules retain their IDs, revisions, timestamps, configuration, enabled states, and display counters, and are assigned to the environment-configured administrator. Older programs reject databases newer than their supported schema. To downgrade, restore the complete pre-upgrade backup together with the older program; do not open a migrated database with an older binary.

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

生产目标包括原生二进制 + systemd，以及手工 Linux Docker Engine + Compose v2 部署，架构为 `amd64`、`arm64`；发行版目标包括 Debian 12/13、Ubuntu 22.04/24.04、RHEL 9/10，以及同系 Rocky Linux、AlmaLinux 9/10。原生安装要求 systemd 为 PID 1。首版不承诺 rootless Docker、Docker Desktop、Windows 和 macOS 的生产支持。

These are compatibility targets, not claims that every host/architecture combination has been tested. CI is configured to run native amd64 and arm64 Linux checks, real TCP/UDP integration tests, and Docker smoke tests. A successful CI run confirms those runner environments only. Debian, Ubuntu, and Red Hat family host deployment results must be recorded separately; running a distribution's container does not validate that distribution as a host. See the repository's actual workflow results before relying on a release.

原生安装器检查包含模拟发行包、伪终端、systemctl 命令替身及生成 unit 的静态验证，另以独立程序验证凭据和 HTTP 行为；这些检查不能替代真实 systemd 用户切换、低端口能力或完整发行版宿主安装实测。

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

Pushing a version tag such as `v1.0.0` runs validation before publishing the versioned multi-architecture GHCR image, updating `latest`, and creating GitHub Release archives. Tags such as `v1.0.0-rc.1` publish prereleases and do not replace `latest`. Since `latest` can follow an untagged `main` build, use an explicit release tag for deployments that should stay on a fixed version. Actions are pinned to commit SHAs and publish using the repository's `GITHUB_TOKEN`.

Before the first public release, enable the repository's required Actions permissions, verify package publishing is permitted, set the GHCR package visibility to public, and confirm an unauthenticated image pull works. The repository configuration does not itself publish a release.
