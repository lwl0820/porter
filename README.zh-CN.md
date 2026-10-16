# Porter

[English](README.md) · [简体中文](README.zh-CN.md)

Porter 是专用于单台 Linux 服务器的 TCP、UDP 端口转发 Web 面板，提供中文界面、SQLite 持久化和单容器部署。

- 转发规则新增、查看、修改、删除、启停及错误重试。
- 按协议和状态筛选，按名称和地址搜索。
- 显式监听 IPv4 或 IPv6，目标支持 IP 地址和域名。
- 重启后恢复启用规则，单条端口占用不阻塞其他规则。
- 管理员凭据通过环境变量设置，无需注册或额外数据库。

Porter 使用普通代理，目标服务看到转发服务器的来源地址，不保留原始客户端 IP，也不管理宿主机防火墙或云安全组。“运行中”表示本机监听正常，不保证目标始终可达。修改转发参数、停用和删除规则会结束该规则的现有会话；只改名称保留会话。重启和升级会中断全部会话。

## 快速部署

使用原生 Linux Docker Engine 和 Docker Compose v2。部署采用 host 网络，新增转发端口不需要修改 Compose。

**下方镜像地址为预定发布地址。首次成功发布并将 GHCR 软件包设为公开之前，镜像可能无法拉取。此时请按照下文从源码构建。**

保存为 `compose.yaml`：

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
      PORTER_ADMIN_PASSWORD: ${PORTER_ADMIN_PASSWORD:?请先设置 PORTER_ADMIN_PASSWORD}
    volumes:
      - porter-data:/data

volumes:
  porter-data:
```

在同目录创建私密的 `.env`，将示例替换为足够长的独立密码。Compose 的 `.env` 使用单引号可保留密码中的 `$`、`#` 等字面字符。

```dotenv
PORTER_ADMIN_PASSWORD='replace-with-a-long-unique-password'
```

```bash
chmod 600 .env
docker compose up -d
docker compose logs -f porter
```

在可信管理网络访问 `http://服务器IP:8080`，使用 `admin` 和配置的密码登录。用户名可直接在 `compose.yaml` 中修改。更改凭据后执行 `docker compose up -d --force-recreate` 重建容器生效，旧登录会话随之失效。

管理端口仅向可信管理网络开放。通过公网管理时，使用外置 HTTPS 反向代理，将 `PORTER_HTTP_ADDR` 绑定到代理可访问的可信后端地址，并设置 `PORTER_COOKIE_SECURE: "true"`。反向代理必须保留浏览器访问的 `Host` 请求头：Porter 会比对 Origin 与 Host，不信任转发请求头。TCP、UDP 业务流量是否加密取决于业务协议。请自行在宿主机防火墙和云安全组放行需要的转发端口。

容器使用 UID 0，删除全部 capability 后仅增加用于低端口绑定的 `NET_BIND_SERVICE`。不需要特权模式、`NET_ADMIN` 或 Docker socket。host 网络无需添加 `ports` 映射。同一个数据卷只运行一个实例，数据目录使用本地文件系统。

## 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `PORTER_ADMIN_USERNAME` | 必填 | 管理员用户名，空值拒绝启动。 |
| `PORTER_ADMIN_PASSWORD` | 必填 | 管理员密码，不提供默认密码。 |
| `PORTER_HTTP_ADDR` | `0.0.0.0:8080` | 显式本机 IPv4/IPv6 地址与 1–65535 端口，例如 `127.0.0.1:8080`、`[::1]:8080`；不接受域名。 |
| `PORTER_DB_PATH` | `/data/porter.db` | SQLite 文件路径，应持久化整个目录。 |
| `PORTER_COOKIE_SECURE` | `false` | 浏览器通过 HTTPS 访问时设为 `true`。 |
| `PORTER_MAX_SESSIONS` | `4096` | 全局 TCP 连接与 UDP 会话合计上限。 |
| `PORTER_MAX_SESSIONS_PER_RULE` | `1024` | 单条规则的合计会话上限。 |
| `PORTER_UDP_IDLE_TIMEOUT` | `60s` | UDP 会话空闲回收时间，使用 Go duration 格式。 |
| `PORTER_DIAL_TIMEOUT` | `5s` | 上游连接超时，使用 Go duration 格式。 |

规则端口为 1–65535 的单个整数，TCP 和 UDP 可以使用相同数字端口。监听地址必须是本机 IP；IPv6 地址在表单中直接填写 `::` 等地址，不加方括号。目标 IP 或域名与端口分别填写。域名在新 TCP 连接或新 UDP 会话创建时解析，不会逐个数据报解析。UDP 没有连接握手，不会因为目标未返回业务响应就切换其他解析地址；域名仅部分地址可达时，请使用明确可达的 IP。非法字段、监听占用和管理端口冲突会在界面提示。监听地址变更涉及自身重叠时，请先停用再修改。

日志输出到标准输出。`/healthz` 提供进程和数据库就绪检查。SQLite 保存规则配置，不保存管理员密码或登录会话。浏览器登录会话在 12 小时后或进程重启后失效。

## 从源码构建

构建环境固定为 Go 1.27.2、Node.js 24.22.0 LTS。本地 Docker 构建需要 Docker Buildx，宿主机无需安装 Go 或 Node.js。

```bash
git clone https://github.com/lwl0820/porter.git
cd porter
docker build -t porter:local .
```

将快速部署模板中的镜像行改为 `image: porter:local`，然后使用相同的 `.env` 和启动步骤。源码更新后重新构建镜像并重建容器。

不使用 Docker 时，将临时文件和缓存放在仓库目录：

```bash
mkdir -p .tmp/tmp .tmp/go-cache .tmp/go-path .tmp/go-mod .tmp/npm-cache
export TMPDIR="$PWD/.tmp/tmp" GOTMPDIR="$PWD/.tmp/tmp"
export GOCACHE="$PWD/.tmp/go-cache" GOPATH="$PWD/.tmp/go-path" GOMODCACHE="$PWD/.tmp/go-mod"
export npm_config_cache="$PWD/.tmp/npm-cache"
npm --prefix web ci
npm --prefix web run build
CGO_ENABLED=0 go build -trimpath -ldflags '-X main.version=dev' -o .tmp/porter ./cmd/porter
```

生成的程序已内嵌前端，无需独立前端服务器。也可从 [GitHub Releases](https://github.com/lwl0820/porter/releases) 下载相应架构的 Linux 压缩包并解压。运行方式如下：

```bash
mkdir -p data
export PORTER_ADMIN_USERNAME=admin
read -rsp '管理员密码：' PORTER_ADMIN_PASSWORD
echo
export PORTER_ADMIN_PASSWORD
export PORTER_DB_PATH="$PWD/data/porter.db"
./.tmp/porter
```

使用发布压缩包时，将最后一行改为 `./porter`。同时下载 `SHA256SUMS`，使用 `sha256sum --ignore-missing -c SHA256SUMS` 校验已下载的压缩包。普通用户可监听高端口；低端口需要适当的服务权限或 `CAP_NET_BIND_SERVICE`。长期运行时请配置服务管理器以自动重启。

## 升级、备份和回退

正式版本发布后，建议固定 `v1.0.0` 之类的版本标签，便于重复部署。升级前停止 Porter，备份**完整数据卷**，包括 SQLite 日志文件。不要单独复制运行中的数据库文件，不要让多个副本共享数据库目录。

使用 `docker volume ls` 查找 Compose 数据卷名称，通常为 `<项目名>_porter-data`。将下方 `YOUR_PORTER_VOLUME` 替换为实际名称：

```bash
docker compose stop porter
mkdir -p backups
docker run --rm -v YOUR_PORTER_VOLUME:/data:ro -v "$PWD/backups:/backup" \
  alpine:3.24.2 tar -czf /backup/porter-before-upgrade.tar.gz -C /data .
docker compose pull
docker compose up -d
```

本地镜像请重新构建，不执行拉取。升级后检查日志、登录、规则状态及实际 TCP、UDP 收发。回退时停止 Porter，选择旧镜像或旧程序，并将升级前的完整备份恢复到空数据卷。新数据库结构可能要求同时恢复旧程序和旧数据。`docker compose down -v` 会删除已保存规则，仅在确实需要清空数据时使用。

## 兼容范围与验证

生产目标为原生 Linux Docker Engine + Compose v2，架构为 `amd64`、`arm64`；发行版目标包括 Debian 12/13、Ubuntu 22.04/24.04、RHEL 9/10，以及同系 Rocky Linux、AlmaLinux 9/10。首版不承诺 rootless Docker、Docker Desktop、Windows 和 macOS 的生产支持。

这些是兼容目标，不代表所有宿主系统与架构组合均已实测。CI 配置了原生 amd64、arm64 Linux 检查、真实 TCP/UDP 集成测试及 Docker 冒烟测试；CI 成功只代表对应执行环境通过。Debian、Ubuntu、红帽系真实宿主机部署需要单独记录，不能用发行版容器代替宿主机验证。请结合仓库实际工作流结果判断具体版本。

## 开发与发布

设置上文缓存环境变量后执行：

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

最后一条命令仅在 `.tmp/release` 生成 Linux amd64、arm64 压缩包与 `SHA256SUMS`，不会发布。PR 和 `main` 推送执行验证；推送 `v1.0.0` 等版本标签时，验证通过后发布多架构 GHCR 镜像和 GitHub Release 压缩包。`v1.0.0-rc.1` 等预发布标签不会覆盖 `latest`。Actions 固定 commit SHA，发布使用仓库 `GITHUB_TOKEN`。

首次公开发布前，需要启用仓库所需的 Actions 权限、确认允许发布软件包、将 GHCR 软件包设为公开，并实际验证匿名拉取。写入工作流配置本身不会产生远端发行版。
