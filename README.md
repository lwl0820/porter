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

Supports Ubuntu 22.04/24.04, Debian 12/13, RHEL/Rocky Linux/AlmaLinux 9/10 on amd64 and arm64. Requires Bash, curl for the download command, and root or sudo privileges. The installer adds Python 3, curl and CA certificates from the system repositories when needed and downloads the GitHub Release programs. Docker, Go and Node.js are unnecessary. systemd must be PID 1; ordinary distribution containers are unsupported.

The script first offers **1 English / 2 中文**, defaulting to English when you press Enter. Specify `bash -s -- --lang en` or `bash -s -- --lang zh-CN` after the pipe to choose directly. Help is available without a terminal through `bash install.sh --help --lang en`, with English as the default. Installer help, prompts, errors and results follow the chosen language; system command diagnostics retain their original language. This selection does not set the CLI's saved language.

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
| Service program | `/opt/porter/porterd` |
| Command-line client | `/opt/porter/porter` |
| System command | `/usr/bin/porter`, a managed link to the client |
| Admin and listen configuration | `/etc/porter/porter.env`, mode `600` |
| CLI language preference | `/etc/porter/cli.json`, mode `600`, created on first language selection |
| Installation record | `/etc/porter/install.json`, mode `600` |
| SQLite and journal files | `/var/lib/porter`, writable by the service user |
| systemd unit | `/etc/systemd/system/porter.service` |

The dedicated non-login `porter` user/group runs the service with only `CAP_NET_BIND_SERVICE`, supporting low ports, restart on failure and startup at boot. Configuration uses systemd `EnvironmentFile` syntax: double-quoted values with backslashes before literal backslashes, double quotes, dollar signs and backticks. For example, write a literal `$` as `\$` and a literal `\` as `\\`. Do not source this file or use Compose-style `$$`. Additional `PORTER_*` settings, such as `PORTER_COOKIE_SECURE="true"`, may be appended with the same quoting. Restart with `sudo systemctl restart porter.service` after editing.

Repeating installation in the same directory preserves credentials, listen address, programs and database without upgrading. Partial installation failures can be retried using saved credentials. Changing versions requires `--upgrade`; a different `--version` alone is rejected. A missing managed system link is repaired, while unknown files or links at `/usr/bin/porter` are never overwritten or adopted. When `--install-dir /usr/bin` is used, the installed client itself provides the system command. Unknown existing users, services, configuration or data are never taken over; Docker data is not migrated. Success requires an active, enabled service and an exact healthy `/healthz` response at the configured address.

The installer prints both user `/user/` and administrator `/admin/` URLs. The root URL opens the ordinary user portal. Administrators create ordinary user accounts in the admin portal.

Uninstall with the current script. Uninstall needs Python 3 already installed, but does not fetch Releases, install dependencies or request administrator credentials. It detects a custom program directory from the installation record; an explicit `--install-dir` must match:

```bash
curl -fsSL https://raw.githubusercontent.com/lwl0820/porter/main/scripts/install.sh | \
  bash -s -- --uninstall --lang en
```

Review the resource list and type `yes` to confirm, or Enter to cancel. By default uninstall stops and disables the service and removes its managed programs, unit and system link. It retains the **entire** `/var/lib/porter` directory, `/etc/porter/porter.env`, `/etc/porter/cli.json`, the restricted installation record and the dedicated service account/group. Reinstall with the current script and the original program directory to reuse credentials, listen address, databases and CLI language. It restores the **same recorded version and layout**, which must still be available as a Release; a failed download preserves configuration and data. Older scripts do not recognize the split layout and must not be used for recovery. After an interrupted uninstall, retry the same uninstall options before reinstalling.

To permanently remove accounts, rules and statistics, add `--purge`. After `yes`, a second confirmation requires typing `DELETE`:

```bash
curl -fsSL https://raw.githubusercontent.com/lwl0820/porter/main/scripts/install.sh | \
  bash -s -- --uninstall --purge --lang en
```

Purge removes the service configuration `porter.env`, CLI configuration `cli.json`, entire data directory, installation record and dedicated user/group. Both uninstall modes clean validated, uncommitted installer files. Other extra files in the program and configuration directories are preserved. Cancellation or absence of an interactive terminal does not start uninstall. Unknown services, modified units, systemd drop-ins, replaced configuration nodes and mismatched accounts are rejected before stopping the service. Only the verified managed system link is removed. Shared logs, system packages and Docker resources are retained. `--purge` requires `--uninstall`; uninstall rejects `--version` and `--upgrade`.

A forced interruption may leave historical `.tmp` contents in the program directory, potentially including temporary configuration or credential copies. Both uninstall modes preserve these files because their ownership cannot be confirmed. Inspect them before removing them manually; purge does not remove unknown historical copies.

For sudo users (omit `sudo` when running as root):

```bash
sudo systemctl status porter.service
sudo journalctl -u porter.service -f
sudo systemctl stop porter.service
sudo systemctl start porter.service
```

To upgrade a managed installation or migrate an older single-program installation, use the current installer with **`--upgrade`**. Omitting `--version` selects the latest stable Release; specify it to choose a published split-program release:

```bash
curl -fsSL https://raw.githubusercontent.com/lwl0820/porter/main/scripts/install.sh | \
  bash -s -- --upgrade --lang en
# To pin the target, add: --version vX.Y.Z
```

The installer verifies the current managed installation, target checksum and both program versions before stopping anything. It displays the source and target versions and asks for confirmation. Upgrading briefly interrupts forwarding. After stopping the service it backs up the previous programs, unit, exact installation record, configuration and **complete data directory**, then switches the programs and unit together. Both programs must come from the same release. Downgrades, single-program target archives, customized units and conflicting target files are rejected. A complete split installation at the same version is an idempotent check.

The data directory itself (`/var/lib/porter`) must not be a mount point, including a bind mount: the installer rejects such an upgrade before stopping the service or changing the installation, so whole-directory rollback remains possible. Its parent directory, such as `/var`, may be on a separate filesystem.

On failure the installer restores the matching old programs, configuration and complete data, together with the prior service state. After a forced interruption, rerun the current installer command. That run first restores a stable installation and exits; run the desired upgrade or uninstall command again after recovery succeeds.

Backups are kept under the program directory at `.tmp/upgrade.<id>` (by default `/opt/porter/.tmp/upgrade.<id>`); the installer prints the exact location. They contain the old programs, unit, installation record, service/CLI configuration and complete data, and may include credentials. Keep them protected: purge preserves historical upgrade backups. A rollback also retains the failed new data under the data directory’s parent at `.porter-failed-upgrade.<id>` (by default `/var/lib/.porter-failed-upgrade.<id>`). These copies may contain business data; inspect them before any manual cleanup. If recovery fails, preserve the installation record, backup and any recovery directories reported in the error; do not overwrite them with another installation or restore a database file in isolation.

Ordinary reruns of a legacy installation preserve its original version and service layout, without claiming that a separate CLI has been installed. **v0.2.0 has no rule CLI**: running its `porter rules` may report missing `PORTER_ADMIN_USERNAME` and `PORTER_ADMIN_PASSWORD`. Check `/opt/porter/porter --version` and upgrade after the split-program Release is available; exporting Web credentials is not the fix.

For a manual installation or custom unit, stop the service and save the complete data directory, configuration, unit and old programs first. Verify the downloaded archive using `SHA256SUMS`, install both `porter` and `porterd`, and change the unit's `ExecStart` from the old `porter` to `porterd` before starting it. The managed installer refuses to take over customized units. Do not manually edit its installation record to bypass this check. Verify login, health and real TCP/UDP traffic. Rollback must restore the older programs with their matching **complete configuration and data backup**, not just replace one binary. Native backups are separate from Docker volume backups below.

### Native rule commands

`porterd` runs the service; `porter` is a separate command-line client. The installer exposes `porter` on the system PATH, including for the default and custom installation directories. Use it as root or through `sudo` to manage rules with administrator privileges, without entering Web credentials. The service must be running for rule operations; commands use its local Unix socket and changes appear immediately in the Web panel. These commands support native Linux only: Docker images contain only `/porterd`, with local control disabled at build time. Native clients also reject rule management in detected container environments. Use the Web panel for Docker deployments; commands hard-coding the previous Docker `/porter` path must switch to `/porterd` for service operations.

Open the interactive menu:

```bash
sudo porter
# The existing entry is also supported:
sudo porter rules
sudo porter rules --lang en
```

On first use without `--lang`, **choose 1 English or 2 中文; Enter selects English**. The selection is saved in `/etc/porter/cli.json` and reused on later calls, for both root and sudo. `--lang en` or `--lang zh-CN` overrides the language for that call only; it does not change the saved preference. Use the menu's language-switching option to save a different language. The preference is independent of the service, Web credentials and database. A save failure is reported and the selected language is used for that call; an invalid configuration is diagnosed rather than silently overwritten. Ordinary uninstall preserves this file, while purge deletes it.

The menu offers create, list, details, edit, delete, language switching and exit. Select rules from the displayed list; editing can retain current values, and deletion asks for confirmation with cancellation as the default. Canceling an unfinished operation does not submit it; completed operations remain saved. Language selection and menu input use the controlling terminal (`/dev/tty`), keeping standard output and redirected standard input separate. The menu requires a terminal even with `--lang` and does not accept `--json`.

For direct commands or unattended scripts, provide an action and all required arguments. Explicit `--lang` also works before the first language has been saved:

```bash
sudo porter rules add --lang en \
  --name website --protocol tcp --listen-host 0.0.0.0 --listen-port 18081 \
  --target-host example.com --target-port 80 --enabled=true --traffic-enabled=false
sudo porter rules list --lang en
sudo porter rules get RULE_ID --lang en --json
sudo porter rules update RULE_ID --lang en --enabled=false
sudo porter rules delete RULE_ID --lang en
```

Replace `RULE_ID` with the stable ID returned by create or list. Protocol values are `tcp`, `udp` and `tcp+udp`. Parameter commands never enter a wizard for missing fields. Without a controlling terminal and without a saved preference, omitting `--lang` fails before any rule operation. **Parameter-mode `delete` takes effect immediately, without confirmation.**

| Command | Arguments and behavior |
| --- | --- |
| `rules add` | Requires `--name`, `--protocol`, `--listen-host`, `--listen-port`, `--target-host`, `--target-port`; optional `--enabled=true\|false`, `--traffic-enabled=true\|false`, `--owner-id ID`. Defaults: enabled, display statistics off, administrator ownership. |
| `rules list` | All rules, or filter with `--owner-id ID`. Shows stable IDs, owner IDs, status and revisions. |
| `rules get ID` | Full rule details, including configuration, owner, revision and display statistics. |
| `rules update ID` | Set one or more creation configuration fields; omitted fields and ownership remain unchanged. Supports `--revision N`. |
| `rules delete ID` | Delete the specified rule and release its listeners and sessions. Supports `--revision N`. |

Common options `--lang zh-CN|en`, `--socket PATH`, `--json` and `--help` can appear before or after the action; `--lang=en` is also accepted. Use `--enabled=false` or `--traffic-enabled=false` to switch a field off; boolean field options require the equals form. `--json` is for single parameter operations: results go to standard output, errors to standard error, with stable JSON field names and error codes. Byte counters remain exact decimal strings. Exit codes are `0` for success, `1` for operation failure and `2` for usage errors. For help without a running service, use `porter rules --help --lang en`; `porter --version` remains unchanged.

Administrators can manage all rules. Creation with `--owner-id USER_ID` assigns an existing user; that user's allocated ports and quota still apply. Use the existing user ID from the authenticated Web admin API (`GET /api/admin/users`, `user.id` in each account). Rule updates preserve ownership. The CLI manages rules only; user, plan, port-pool, retry and counter-reset administration remains in the Web panel. Updates and deletions check revisions and fail on concurrent changes; reload before retrying. Scripts can pin a previously observed revision with `--revision N`. After a timeout, inspect the result before resubmitting a change.

The installed service uses `/var/lib/porter/porter-control.sock` by default. The socket accepts only actual root peers and is not exposed on the Web port. Service startup creates it beside the database, unless `PORTER_CONTROL_SOCKET` sets another path. The CLI chooses `--socket`, then `PORTER_CONTROL_SOCKET`, then `/var/lib/porter/porter-control.sock`. The parent directory and every ancestor must belong to root or the service user, must not be writable by group or other users, and must not be symbolic links. A directory owned by another user is rejected even if currently read-only, since its owner can change its permissions. The system command also works with custom program directories. For a manually started service or a custom database directory, pass the matching socket explicitly:

```bash
sudo porter rules list --lang en --socket /custom/data/porter-control.sock
```

A connection failure means you should check service status, the socket path and whether the upgraded service has restarted. Commands do not start a second service or edit an offline database.

### Manual Compose deployment

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
CGO_ENABLED=0 go build -trimpath -ldflags '-X main.version=dev' -o .tmp/porterd ./cmd/porterd
```

The `porterd` service embeds the web interface; `porter` is the independent client. No separate frontend server is required. Start the service as follows, or extract both programs from the matching Linux archive in [GitHub Releases](https://github.com/lwl0820/porter/releases):

```bash
mkdir -p data
export PORTER_ADMIN_USERNAME=admin
read -rsp 'Administrator password: ' PORTER_ADMIN_PASSWORD
echo
export PORTER_ADMIN_PASSWORD
export PORTER_DB_PATH="$PWD/data/porter.db"
./.tmp/porterd
```

For a release archive, use `./porterd` instead. Use `sudo ./.tmp/porter rules --lang en --socket "$PWD/data/porter-control.sock"` for the source-built client, or `sudo ./porter rules ...` from an extracted archive. Native control requires the trusted directory ownership described above. Download `SHA256SUMS` alongside the archive and verify it with `sha256sum --ignore-missing -c SHA256SUMS`. Both programs in a release archive have the same version. A normal user can bind high ports; low ports require suitable service privileges or `CAP_NET_BIND_SERVICE`. Use a service supervisor for automatic restarts; a manual unit must start `porterd`.

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

Production targets are native programs with systemd, or manual Linux Docker Engine with Compose v2, on `amd64` and `arm64`. Distribution targets include Debian 12/13, Ubuntu 22.04/24.04, RHEL 9/10, Rocky Linux 9/10 and AlmaLinux 9/10. Native installation requires systemd as PID 1. Rootless Docker, Docker Desktop, Windows and macOS are not supported production targets.

These are compatibility targets, not claims that every host/architecture combination has been tested. CI is configured to run native amd64 and arm64 Linux checks, real TCP/UDP integration tests, and Docker smoke tests. A successful CI run confirms those runner environments only. Debian, Ubuntu, and Red Hat family host deployment results must be recorded separately; running a distribution's container does not validate that distribution as a host. See the repository's actual workflow results before relying on a release.

Installer checks use simulated release archives, pseudo-terminals, a systemctl substitute and static unit validation, with separate program checks for credentials and HTTP behavior. They do not replace real systemd user switching, low-port capability or full distribution host installation tests.

## Development and publishing

After setting the cache environment above:

```bash
npm --prefix web run lint
npm --prefix web run type-check
npm --prefix web run test
npm --prefix web run build
python3 scripts/check-build-boundaries.py
go vet ./...
go test -race ./...
go test -tags porter_docker ./...
go test -count=1 ./tests/integration/...
docker build -t porter:local .
python3 scripts/docker-smoke.py porter:local
VERSION=v1.0.0 bash scripts/build-release.sh
```

The final command only builds local Linux amd64/arm64 archives and `SHA256SUMS` under `.tmp/release`; it does not publish them. Pull requests validate the application without publishing. Each push to `main` runs validation and, if it succeeds, publishes `ghcr.io/lwl0820/porter:latest` for Linux amd64/arm64 without requiring a version tag. These builds identify their source commit and do not create a GitHub Release.

Pushing a version tag such as `v1.0.0` runs validation before publishing the versioned multi-architecture GHCR image, updating `latest`, and creating GitHub Release archives. Tags such as `v1.0.0-rc.1` publish prereleases and do not replace `latest`. Since `latest` can follow an untagged `main` build, use an explicit release tag for deployments that should stay on a fixed version. Actions are pinned to commit SHAs and publish using the repository's `GITHUB_TOKEN`.

Before the first public release, enable the repository's required Actions permissions, verify package publishing is permitted, set the GHCR package visibility to public, and confirm an unauthenticated image pull works. The repository configuration does not itself publish a release.
