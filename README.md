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

Porter uses ordinary proxy connections. The destination sees the forwarding server's address. It does not preserve the client's source IP or manage host firewalls and cloud security groups. A running rule means its local listener is ready, not that its destination is reachable. Editing forwarding parameters, disabling or deleting a rule ends its active sessions; renaming alone preserves them. Restarting or upgrading interrupts all sessions.

## Quick start

### 原生一键交互安装

在使用 systemd 的 Linux 服务器（例如 SSH 终端）执行：

```bash
curl -fsSL https://raw.githubusercontent.com/lwl0820/porter/main/scripts/install.sh | bash
```

支持 Ubuntu 22.04/24.04、Debian 12/13、RHEL/Rocky Linux/AlmaLinux 9/10，架构为 amd64、arm64。需要 Bash、下载入口使用的 curl，以及 root 或 sudo 权限。安装器按需从系统仓库补齐 Python 3、curl 和 CA 证书，直接下载 GitHub Actions 编译的独立程序，不需要 Docker、Go 或 Node.js；systemd 必须是 PID 1，普通发行版容器不适用。

首次依次输入**管理员用户名、管理员密码、密码确认、Web 监听地址**。密码隐藏输入，不自动生成或回显。监听必须是 `IPv4:端口` 或 `[IPv6]:端口`，如 `127.0.0.1:18080`、`0.0.0.0:8080`、`[::]:8080`，端口范围为 1–65535。通配监听的远程入口需将输出中的回环地址替换为服务器 IP。管理端口只应开放给可信网络；安装器不配置防火墙或 HTTPS。

程序目录可包含空格、`$` 和 `%`；受 systemd 路径限制，不接受引号或反斜线；也不能位于 `/var/lib/porter` 数据目录或 `/etc/porter` 配置目录内。默认安装最新正式 [GitHub Release](https://github.com/lwl0820/porter/releases)，也可指定已发布版本及程序目录：

```bash
curl -fsSL https://raw.githubusercontent.com/lwl0820/porter/main/scripts/install.sh | \
  bash -s -- --version v1.0.0 --install-dir /opt/porter
```

**公开 Release 尚未发布时，安装器会明确报错，latest API 可能返回 404。** 当前发布流程仅在版本标签通过验证和镜像发布后创建二进制 Release；推送 main 发布的镜像不等于可下载的二进制。安装器下载 `porter-版本-linux-架构.tar.gz` 与 `SHA256SUMS`，严格校验对应文件摘要后才安装，不在服务器编译。

默认文件和服务如下；`--install-dir` 只改变程序目录：

| 项目 | 位置 |
| --- | --- |
| 程序 | `/opt/porter/porter` |
| 管理员及监听配置 | `/etc/porter/porter.env`，权限 `600` |
| 安装记录 | `/etc/porter/install.json`，权限 `600` |
| SQLite 及日志文件 | `/var/lib/porter`，专用用户可写 |
| systemd unit | `/etc/systemd/system/porter.service` |

服务以禁止登录的专用系统用户和组 `porter` 运行，限制能力为 `CAP_NET_BIND_SERVICE`，支持低端口绑定、异常重启和开机启动。配置按 systemd `EnvironmentFile` 格式保存，值统一使用双引号，内部的反斜线、双引号、美元符和反引号需要加反斜线；例如字面 `$` 写作 `\$`、字面 `\` 写作 `\\`。不要 `source` 此文件，也不要按 Compose 规则写 `$$`。需要额外环境变量时，可按相同双引号格式追加 `PORTER_COOKIE_SECURE="true"` 等 `PORTER_*` 项。修改配置后执行 `sudo systemctl restart porter.service`。

重复运行相同目录的安装命令会保留账号、监听地址、程序和数据库，不自动升级；首次局部失败可重试，已保存凭据会复用。已有安装不接受不同的 `--version`。发现未知的同名账号、服务、配置或数据时会退出，不自动接管；也不迁移已有 Docker 数据。仅在服务已启动、已启用且用户指定地址的 `/healthz` 就绪后报告成功。

以下命令供 sudo 账号使用，root 用户去掉 `sudo`：

```bash
sudo systemctl status porter.service
sudo journalctl -u porter.service -f
sudo systemctl stop porter.service
sudo systemctl start porter.service
```

原生实例升级前先停服务并备份完整数据目录、配置、unit 和原程序；不要复制运行中的单个数据库文件。默认安装位置可使用：

```bash
sudo systemctl stop porter.service
install -d -m 0700 "$PWD/backups"
sudo tar -czf "$PWD/backups/porter-before-upgrade.tar.gz" -C / \
  etc/porter var/lib/porter etc/systemd/system/porter.service
sudo chmod 0600 "$PWD/backups/porter-before-upgrade.tar.gz"
sudo cp /opt/porter/porter "$PWD/backups/porter.previous"
```

随后从目标版本 Release 下载本机架构的压缩包和 `SHA256SUMS`，在独立的 `.tmp/porter-upgrade` 目录用 `sha256sum --ignore-missing -c SHA256SUMS` 确认该压缩包校验成功，再解出其中的 `porter`。不要通过重跑一键命令升级。将下方源路径替换为校验后程序的位置，自定义安装目录时同步修改目标路径：

```bash
sudo install -m 0755 .tmp/porter-upgrade/porter /opt/porter/porter.new
sudo mv /opt/porter/porter.new /opt/porter/porter
sudo systemctl start porter.service
sudo journalctl -u porter.service -n 30 --no-pager
```

重启后验证登录、健康端点及实际 TCP/UDP 收发。显式升级后如继续使用安装器，请将 `/etc/porter/install.json` 的 `version` 同步为实际版本标签，保留其余字段和 `600` 权限。回退时先停服务，同时恢复原程序与匹配的**完整数据及配置备份**；旧程序不能直接使用已升级的数据库。上述原生操作与下方 Docker 卷备份相互独立。

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

Open `http://SERVER_IP:8080/admin/` on your trusted management network and sign in as `admin` using your configured password. Ordinary users use `http://SERVER_IP:8080/user/`; the root URL redirects to the admin portal. Change the username directly in `compose.yaml`. To use Web port 18080, change `PORTER_HTTP_ADDR` to `"0.0.0.0:18080"`, then access `http://SERVER_IP:18080`. Host networking uses the configured port directly and needs no `ports` mapping.

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
