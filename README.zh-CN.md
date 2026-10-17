# Porter

[English](README.md) · [简体中文](README.zh-CN.md)

Porter 专用于单台 Linux 服务器的 TCP、UDP 端口转发，支持 SQLite 持久化。原生安装可选择 Web 面板、命令行或两者同时使用；Docker 部署使用 Web 面板。

- TCP、UDP、TCP+UDP 组合规则的新增、查看、修改、删除、启停及错误重试。
- 按协议和状态筛选，按名称和地址搜索。
- 显式监听 IPv4 或 IPv6，目标支持 IP 地址和域名。
- 重启后恢复启用规则，单条端口占用不阻塞其他规则。
- 按规则开启上下行流量与速度统计，支持累计量持久化及手动重置，默认关闭。
- 独立管理端和用户端，管理员可查看所属用户、按用户筛选并直接操作全部规则。
- 管理员添加套餐和用户，按套餐分配连续独占端口，支持四种流量计费和三种 UTC 周期重置。
- 启用 Web 时通过环境变量设置管理员凭据，普通用户由管理员创建，无需注册或额外数据库。
- 原生 Linux 支持双语交互菜单和参数命令管理规则。

Porter 使用普通代理，目标服务看到转发服务器的来源地址，不保留原始客户端 IP，也不管理宿主机防火墙或云安全组。“运行中”表示本机监听正常，不保证目标始终可达。修改转发参数、停用和删除规则会结束该规则的现有会话；只改名称保留会话。重启和升级会中断全部会话。

## 快速部署

### 原生一键交互安装

在使用 systemd 的 Linux 服务器（例如 SSH 终端）执行：

```bash
curl -fsSL https://raw.githubusercontent.com/lwl0820/porter/main/scripts/install.sh | bash
```

支持 Ubuntu 22.04/24.04、Debian 12/13、RHEL/Rocky Linux/AlmaLinux 9/10，架构为 amd64、arm64。需要 Bash、下载入口使用的 curl，以及 root 或 sudo 权限。安装器按需从系统仓库补齐 Python 3、curl 和 CA 证书，直接下载 GitHub Actions 编译的服务端与客户端，不需要 Docker、Go 或 Node.js；systemd 必须是 PID 1，普通发行版容器不适用。

脚本先提供“1 English / 2 中文”语言选择，回车默认英文；也可在管道后使用 `bash -s -- --lang zh-CN` 或 `bash -s -- --lang en` 指定语言。帮助支持 `bash install.sh --help --lang en`，不要求交互终端，未指定语言时默认英文。安装器自己的帮助、交互、错误和结果均随语言切换，系统命令诊断保持原样。安装器的选择不会预设 CLI 保存的语言。

语言选择后，首次安装分别询问是否启用 **Web 管理**和**命令行管理**，两项回车均默认开启，至少保留一种；如果两项都关闭，会在提交安装前要求重新选择。

| 选择 | 安装的管理入口 | Web 配置 |
| --- | --- | --- |
| Web＋命令行（默认） | Web 面板和 `sudo porter` | 需要管理员凭据和监听地址 |
| 仅 Web | Web 面板，不安装托管客户端、PATH 入口或本机控制套接字 | 需要管理员凭据和监听地址 |
| 仅命令行 | `sudo porter`，不启动 Web 监听、页面或 HTTP API | 不询问用户名、密码或 Web 监听地址 |

三种组合都会运行后台 `porterd` 服务负责转发。仅命令行模式也需要服务运行，并使用 root 或 sudo 管理规则。

启用 Web 时，依次输入**管理员用户名、管理员密码、密码确认、Web 监听地址**。密码隐藏输入，不自动生成或回显。监听必须是 `IPv4:端口` 或 `[IPv6]:端口`，如 `127.0.0.1:18080`、`0.0.0.0:8080`、`[::]:8080`，端口范围为 1–65535。通配监听的远程入口需将输出中的回环地址替换为服务器 IP。管理端口只应开放给可信网络；安装器不配置防火墙或 HTTPS。

程序目录可包含空格、`$` 和 `%`；受 systemd 路径限制，不接受引号或反斜线；也不能位于 `/var/lib/porter` 数据目录或 `/etc/porter` 配置目录内。默认安装最新正式 [GitHub Release](https://github.com/lwl0820/porter/releases)，也可指定已发布版本及程序目录：

```bash
curl -fsSL https://raw.githubusercontent.com/lwl0820/porter/main/scripts/install.sh | \
  bash -s -- --version v1.0.0 --install-dir /opt/porter
```

**公开 Release 尚未发布时，安装器会明确报错，latest API 可能返回 404。** 当前发布流程仅在版本标签通过验证和镜像发布后创建二进制 Release；推送 main 发布的镜像不等于可下载的二进制。安装器下载 `porter-版本-linux-架构.tar.gz` 与 `SHA256SUMS`，严格校验对应文件摘要后才安装，不在服务器编译。

默认文件和服务如下；`--install-dir` 只改变程序目录：

| 项目 | 位置 |
| --- | --- |
| 服务程序 | `/opt/porter/porterd` |
| 命令行客户端（启用时） | `/opt/porter/porter` |
| 系统命令（启用命令行时） | `/usr/bin/porter`，指向实际客户端的托管链接 |
| 服务配置，含组件选择 | `/etc/porter/porter.env`，权限 `600` |
| CLI 语言偏好 | `/etc/porter/cli.json`，权限 `600`，首次选择语言后创建 |
| 安装记录 | `/etc/porter/install.json`，权限 `600` |
| SQLite 及日志文件 | `/var/lib/porter`，专用用户可写 |
| systemd unit | `/etc/systemd/system/porter.service` |

服务以禁止登录的专用系统用户和组 `porter` 运行，限制能力为 `CAP_NET_BIND_SERVICE`，支持低端口绑定、异常重启和开机启动。配置按 systemd `EnvironmentFile` 格式保存，值统一使用双引号，内部的反斜线、双引号、美元符和反引号需要加反斜线；例如字面 `$` 写作 `\$`、字面 `\` 写作 `\\`。不要 `source` 此文件，也不要按 Compose 规则写 `$$`。需要额外环境变量时，可按相同双引号格式追加 `PORTER_COOKIE_SECURE="true"` 等 `PORTER_*` 项。修改配置后执行 `sudo systemctl restart porter.service`。

重新执行同一安装命令即可更改管理入口。已有安装询问“**是否修改管理组件？[no]**”，回车保留当前组合；输入 `yes` 后，再分别选择 Web 与命令行，每项默认值沿用当前设置。三种组合可相互切换；选择不变时保留程序、配置和数据，不下载新版本，也不重启服务，原本停止或未设开机启动的状态保持不变。

切换前会展示当前组合、目标组合并要求确认。切换会重启服务，中断现有转发连接；规则、统计、用户、套餐及 CLI 语言偏好保留。启用 Web 时复用已有合法凭据和监听设置，只有缺少可用配置才要求输入；关闭 Web 保留其设置，关闭命令行保留其语言偏好。补装客户端下载**当前已安装版本**，不隐式升级。确认前取消不会提交改变，切换失败会恢复原程序、配置、数据和服务状态；强制中断后先重跑安装器恢复，再请求下一次变更。

开启 Web 时，其监听地址不能与已有启用的 TCP 或 TCP+UDP 转发规则重叠。检查到冲突会拒绝切换，保留原组合和规则；请改用其他 Web 地址，或调整规则后重试。

旧安装需要先用 `--upgrade` 升级到支持组件选择的版本，再重跑脚本切换。升级保留原管理入口：旧单程序布局保留仅 Web，旧双程序布局保留两种入口。不支持组件配置的目标版本会在停服前被拒绝。

首次局部失败可重试并复用已保存设置。版本变化必须使用 `--upgrade`，单独指定不同的 `--version` 会被拒绝。启用命令行时，已托管的系统入口缺失可补齐；未知文件、链接或悬空链接不会覆盖，同目标手工链接也不会自动认领。仅 Web 安装不会接管系统上其他来源的 `porter` 命令。启用命令行且使用 `--install-dir /usr/bin` 时，由该目录中的客户端本身提供命令。发现未知的同名账号、服务、配置或数据时会退出，不自动接管；也不迁移已有 Docker 数据。首次安装成功要求服务已启动、已启用，且所选入口均通过就绪检查：Web 检查 HTTP `/healthz`，命令行检查 root 本机规则查询，后者不会触发语言选择或保存偏好。

启用 Web 时，安装就绪后分别输出普通用户 `/user/` 与管理员 `/admin/` 地址；直接访问根地址也进入普通用户端。普通用户账号由管理员在管理端创建。启用命令行时输出 `sudo porter` 用法；仅命令行安装没有 Web 地址。

卸载使用同一份新版脚本；无需下载 Release、输入管理员凭据或安装依赖，但宿主需已提供 Python 3。未传 `--install-dir` 时从安装记录识别自定义目录，显式指定时必须与记录一致：

```bash
curl -fsSL https://raw.githubusercontent.com/lwl0820/porter/main/scripts/install.sh | \
  bash -s -- --uninstall --lang zh-CN
```

卸载列出资源并要求输入 `yes`，回车取消；默认停止服务、取消开机启动并删除托管程序、unit 和系统命令链接，保留完整 `/var/lib/porter`、`/etc/porter/porter.env`、`/etc/porter/cli.json`、受限安装记录以及专用账号/组。使用新版脚本按原程序目录重新安装，会复用原凭据、监听、数据库及 CLI 语言，并恢复记录的**同一版本、布局和组件组合**；该版本 Release 必须仍可下载，下载失败不会清空配置或数据。旧版脚本不认识双程序布局，请勿用于恢复。卸载中断会保留恢复记录，先以相同卸载选项重试，再重新安装。

彻底删除需另加 `--purge`，在 `yes` 后再次输入 `DELETE`；账号、规则和统计将永久丢失：

```bash
curl -fsSL https://raw.githubusercontent.com/lwl0820/porter/main/scripts/install.sh | \
  bash -s -- --uninstall --purge --lang zh-CN
```

彻底删除清理服务配置 `porter.env`、CLI 配置 `cli.json`、完整数据目录、安装记录和专用账号/组；两种卸载方式都会在验证路径和文件类型后，清理安装器未提交的临时文件。程序目录和配置目录中的其他额外文件保留。取消或无交互终端不会开始卸载。脚本在停服前拒绝未知服务、被修改的 unit、自定义 systemd 覆盖配置、被替换的配置节点及不匹配的专用账号，仅删除已验证托管的系统链接，不删除共享日志、系统软件包或 Docker 资源。`--purge` 必须配合 `--uninstall`，卸载不接受 `--version` 或 `--upgrade`。

强制中断可能在程序目录的历史 `.tmp` 内容中留下临时配置或凭据副本。两种卸载方式都会保留这些无法确认归属的文件；请在人工清理前逐项核对，彻底删除也不会清除未知历史副本。

以下命令供 sudo 账号使用，root 用户去掉 `sudo`：

```bash
sudo systemctl status porter.service
sudo journalctl -u porter.service -f
sudo systemctl stop porter.service
sudo systemctl start porter.service
```

托管安装升级或从旧单程序布局迁移时，使用新版安装器并显式添加 **`--upgrade`**。不指定 `--version` 时选择最新正式 Release；也可固定为已经发布的双程序版本：

```bash
curl -fsSL https://raw.githubusercontent.com/lwl0820/porter/main/scripts/install.sh | \
  bash -s -- --upgrade --lang zh-CN
# 固定目标时追加：--version vX.Y.Z
```

升级保留当前组件组合。安装器在停服前验证当前托管安装、目标包摘要、程序版本及候选配置，显示源版本、目标版本及停机范围并要求确认。升级会短暂中断转发；停服后备份旧程序、unit、原始安装记录、配置和**完整数据目录**，再成套切换程序和 unit。启用命令行时，客户端与服务程序必须来自同一版本；仅 Web 模式只部署服务程序。降级、单程序目标包、定制 unit 或冲突的目标文件会被拒绝；已支持组件选择且相同版本的完整安装仅做幂等检查。

数据目录 `/var/lib/porter` 自身不能是挂载点（包括绑定挂载），否则安装器会在停服或修改安装前拒绝升级或组件切换，以保证完整目录回退可用；其父目录（例如 `/var`）位于独立文件系统不受此限制。

失败时恢复匹配的旧程序、配置、完整数据和原服务状态。强制中断后，重新执行新版安装器命令；这次先恢复稳定安装并退出，恢复成功后，再次执行所需的组件切换、升级或卸载命令。

升级和组件切换的备份保存在程序目录的 `.tmp/upgrade.<标识>` 下，默认位置为 `/opt/porter/.tmp/upgrade.<标识>`，安装器会输出准确路径。备份包括旧程序、unit、安装记录、服务及 CLI 配置和完整数据，可能包含凭据，须保持受限权限；purge 也会保留历史升级及组件切换备份。回滚隔离的新版数据位于数据目录父目录 `.porter-failed-upgrade.<标识>`（默认 `/var/lib/.porter-failed-upgrade.<标识>`），同样保留且可能含业务数据，需核对后人工清理。恢复失败时保留安装记录、备份及错误中指出的恢复目录，不要用另一轮安装覆盖它们，也不要单独恢复某个数据库文件。

普通重跑旧安装仍保留原版本和服务布局，不会宣称已安装独立客户端。**v0.2.0 不含规则 CLI**：执行其中的 `porter rules` 可能提示必须设置 `PORTER_ADMIN_USERNAME` 和 `PORTER_ADMIN_PASSWORD`。先用 `/opt/porter/porter --version` 确认版本，双程序 Release 可用后显式升级；导出 Web 密码不能解决旧版缺少 CLI 的问题。

手工安装或定制 unit 应先停服，备份完整数据、配置、unit 和旧程序。校验目标归档的 `SHA256SUMS` 后安装 `porterd`，使用命令行时再安装 `porter`，将 unit 的 `ExecStart` 从旧 `porter` 改为 `porterd` 再启动。安装器不会接管定制 unit，不要手工修改安装记录绕过检查。验证已启用的管理入口及实际 TCP/UDP 收发；回退时同时恢复旧程序与匹配的**完整数据及配置备份**，不要只替换一个程序。上述原生操作与下方 Docker 卷备份相互独立。

### 原生命令行规则管理

`porterd` 是常驻服务，`porter` 是独立命令行客户端。选择命令行时，一键安装将 `porter` 加入系统 PATH，默认及自定义安装目录均可直接调用。仅 Web 安装需先重跑安装器启用命令行。以 root 或 `sudo` 身份获得管理员规则权限，无须输入 Web 管理员账号密码。执行规则操作时服务必须正在运行；命令通过本机 Unix 套接字访问服务，操作即时生效；同时启用 Web 时也会同步到 Web 后台。此功能仅支持原生 Linux：Docker 镜像仅包含 `/porterd`，在构建时禁用本机控制入口；原生客户端在识别到容器环境时也会拒绝规则管理。Docker 部署请使用 Web 后台；外部命令若硬编码了旧 `/porter` 路径，服务操作需改为 `/porterd`。

进入交互菜单：

```bash
sudo porter
# 原有入口也保留：
sudo porter rules
sudo porter rules --lang zh-CN
```

首次未指定 `--lang` 时，**先选择 1 English 或 2 中文，回车默认英文**。选择保存在 `/etc/porter/cli.json`，后续启动直接复用，root 与 sudo 共用。`--lang en` 或 `--lang zh-CN` 仅覆盖本次语言，不修改保存值；交互菜单的“切换语言”选项用于更改并保存偏好。语言配置独立于服务、Web 凭据和数据库；写入失败会明确提示，本次仍使用所选语言，损坏配置会给出诊断而非静默覆盖。普通卸载保留该文件，purge 删除。

菜单提供创建、列表、详情、修改、删除、切换语言及退出；从当前列表选择规则，修改时可保留当前值，删除需确认且默认取消。取消未完成操作不会提交，已完成操作继续保留。语言选择和菜单输入使用控制终端（`/dev/tty`），不混入标准输出，也不消费重定向的标准输入。交互菜单即使指定语言仍需要终端，不接受 `--json`。

直接参数调用及无人值守脚本应提供动作及完整必填参数；显式 `--lang` 在尚未保存语言时也可使用：

```bash
sudo porter rules add --lang zh-CN \
  --name 网站转发 --protocol tcp --listen-host 0.0.0.0 --listen-port 18081 \
  --target-host example.com --target-port 80 --enabled=true --traffic-enabled=false
sudo porter rules list --lang zh-CN
sudo porter rules get 规则ID --lang zh-CN --json
sudo porter rules update 规则ID --lang zh-CN --enabled=false
sudo porter rules delete 规则ID --lang zh-CN
```

将 `规则ID` 替换为创建或列表返回的稳定 ID。协议值为 `tcp`、`udp`、`tcp+udp`。参数命令缺少字段会直接报错，不进入向导补问；无控制终端、无保存偏好且未指定 `--lang` 时，在执行规则操作前退出。**参数方式的 `delete` 立即删除，没有二次确认。**

| 命令 | 参数及行为 |
| --- | --- |
| `rules add` | 必填 `--name`、`--protocol`、`--listen-host`、`--listen-port`、`--target-host`、`--target-port`；可选 `--enabled=true\|false`、`--traffic-enabled=true\|false`、`--owner-id ID`。默认启用、展示统计关闭、归属管理员。 |
| `rules list` | 默认全部规则，可用 `--owner-id ID` 筛选；显示稳定 ID、归属 ID、状态及版本。 |
| `rules get ID` | 完整规则详情，包括配置、归属、版本及展示统计。 |
| `rules update ID` | 至少指定一个创建时的配置字段，未传字段及归属保留；支持 `--revision N`。 |
| `rules delete ID` | 删除指定规则并释放监听和会话；支持 `--revision N`。 |

通用参数 `--lang zh-CN|en`、`--socket PATH`、`--json`、`--help` 可放在动作前后，语言也支持 `--lang=en`。布尔字段必须使用等号形式，如 `--enabled=false`、`--traffic-enabled=false`。`--json` 只用于单次参数操作：结果写标准输出，错误写标准错误，JSON 字段名及错误码保持稳定，字节计数为精确的十进制字符串。成功退出码为 `0`，操作失败为 `1`，用法错误为 `2`。帮助无须服务运行：`porter rules --help --lang zh-CN`；`porter --version` 用法不变。

管理员可管理所有归属的规则。创建时通过 `--owner-id 用户ID` 指定已有用户，仍须遵守该用户的分配端口及流量额度；已有用户 ID 可从已登录的 Web 管理 API 获取（`GET /api/admin/users` 返回的账号中取 `user.id`）。修改不转移归属。命令行仅管理规则，管理用户、套餐、端口池、重试及统计清零需启用 Web 后台。关闭 Web 后，已有用户规则及套餐额度仍正常生效。修改和删除执行版本检查，并发变更时失败，须重新查询后再操作；脚本可用 `--revision N` 固定先前读取的版本。请求超时后应先查询结果，再决定是否重新提交。

原生安装服务默认使用 `/var/lib/porter/porter-control.sock`，只接受真实 root 对端，不在 Web 端口暴露入口。启用命令行时，服务启动在数据库目录旁创建 `porter-control.sock`，可用 `PORTER_CONTROL_SOCKET` 覆盖；仅 Web 模式不创建该入口。CLI 路径优先级为 `--socket`、`PORTER_CONTROL_SOCKET`、`/var/lib/porter/porter-control.sock`。父目录及全部祖先目录必须归属 root 或服务用户，不能对组或其他用户开放写权限，也不能是符号链接。其他用户拥有的目录即使当前只读也会被拒绝，因为其所有者仍能修改权限。自定义程序目录也可使用系统命令；手动启动服务或使用自定义数据库目录时，显式指定匹配的控制路径：

```bash
sudo porter rules list --lang zh-CN --socket /custom/data/porter-control.sock
```

连接失败时，检查服务状态、控制路径和升级后是否已重启。命令不会另起服务或离线修改数据库。

### 手工 Compose 部署

使用原生 Linux Docker Engine 和 Docker Compose v2。Docker 仅提供 Web 管理，不包含命令行客户端或本机控制套接字，需要保持 Web 开启。部署采用 host 网络，新增转发端口不需要修改 Compose。

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
      PORTER_HTTP_ADDR: "0.0.0.0:8080"
      PORTER_TRAFFIC_FLUSH_INTERVAL: "5s"
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

在可信管理网络访问 `http://服务器IP:8080/admin/`，使用 `admin` 和配置的密码登录。普通用户入口为 `http://服务器IP:8080/user/`；根地址跳转普通用户端。用户名可直接在 `compose.yaml` 中修改。若要将 Web 端口改为 18080，将 `PORTER_HTTP_ADDR` 改为 `"0.0.0.0:18080"`，然后访问 `http://服务器IP:18080`。host 网络直接使用配置的端口，无需添加 `ports` 映射。

管理端口仅向可信管理网络开放。通过公网管理时，使用外置 HTTPS 反向代理，将 `PORTER_HTTP_ADDR` 绑定到代理可访问的可信后端地址，并设置 `PORTER_COOKIE_SECURE: "true"`。反向代理必须保留浏览器访问的 `Host` 请求头：Porter 会比对 Origin 与 Host，不信任转发请求头。TCP、UDP 业务流量是否加密取决于业务协议。请自行在宿主机防火墙和云安全组放行需要的转发端口。

容器使用 UID 0，删除全部 capability 后仅增加用于低端口绑定的 `NET_BIND_SERVICE`。不需要特权模式、`NET_ADMIN` 或 Docker socket。host 网络无需添加 `ports` 映射。同一个数据卷只运行一个实例，数据目录使用本地文件系统。

## 环境变量

下表列出 Porter 的全部运行时环境变量。可选变量未设置或设为空字符串时使用默认值。启用 Web 时管理员用户名和密码均必填：空值或仅含空白字符时拒绝启动，用户名会去除首尾空白。原生服务关闭 Web 后忽略 Web 凭据、HTTP 地址及 Cookie 设置，不创建 Web 监听。至少需要一种实际可用的管理入口，Docker 不支持仅命令行运行。组件布尔值或生效配置的布尔值、地址、整数、时长格式无效，或数值超出范围时，程序拒绝启动。托管原生安装请通过安装器切换组件，使程序、配置和安装记录保持一致。

需要修改其他变量时，将其加入 `compose.yaml` 中服务的 `environment`，例如 `PORTER_UDP_IDLE_TIMEOUT: "2m"`。同目录的 `.env` 用于 Compose 变量替换，仅在其中写入变量不会自动传入容器。修改环境变量后执行 `docker compose up -d --force-recreate` 重建容器生效；重建会中断全部转发会话，并使旧登录会话失效。

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `PORTER_WEB_ENABLED` | `true` | 启用 Web 面板、HTTP API 及健康端点。 |
| `PORTER_CLI_ENABLED` | `true` | 在原生 Linux 启用本机控制套接字；不能解除 Docker 或其他不支持环境的禁用限制。 |
| `PORTER_ADMIN_USERNAME` | Web 开启时必填 | 管理员用户名。 |
| `PORTER_ADMIN_PASSWORD` | Web 开启时必填 | 管理员密码。 |
| `PORTER_HTTP_ADDR` | `0.0.0.0:8080` | 显式本机 IPv4/IPv6 地址与 1–65535 端口，例如 `127.0.0.1:8080`、`[::1]:8080`；不接受域名。 |
| `PORTER_DB_PATH` | `/data/porter.db` | SQLite 文件路径，应持久化整个目录。 |
| `PORTER_CONTROL_SOCKET` | 数据库目录旁的 `porter-control.sock` | 原生 Linux 服务控制路径；CLI 未设置该变量或 `--socket` 时默认使用 `/var/lib/porter/porter-control.sock`。Docker 构建禁用本机规则管理，忽略此变量。 |
| `PORTER_COOKIE_SECURE` | `false` | 布尔值；浏览器通过 HTTPS 访问时设为 `true`，使登录 Cookie 仅通过 HTTPS 发送。 |
| `PORTER_MAX_SESSIONS` | `4096` | 全局 TCP 连接与 UDP 会话合计上限，取值为 1–1000000 的整数。 |
| `PORTER_MAX_SESSIONS_PER_RULE` | `1024` | 单条逻辑规则的 TCP 连接与 UDP 会话合计上限，取值为 1–1000000 的整数；TCP+UDP 两协议共享额度。 |
| `PORTER_UDP_IDLE_TIMEOUT` | `60s` | UDP 会话空闲回收时间，使用 Go duration 格式，范围 `1ms`–`24h`，例如 `500ms`、`60s`、`2m`。 |
| `PORTER_DIAL_TIMEOUT` | `5s` | 上游连接超时，使用 Go duration 格式，范围 `1ms`–`24h`，例如 `500ms`、`5s`、`1m`。 |
| `PORTER_TRAFFIC_FLUSH_INTERVAL` | `5s` | 用户套餐用量和规则展示累计的保存间隔，Go duration，范围 `1ms`–`24h`，例如 `1s`、`30s`。越短磁盘写入越频繁，越长异常退出可能丢失的未保存用量越多；不改变实时超额停转发、UTC 周期和页面刷新间隔。 |

规则端口为 1–65535 的单个整数，TCP 和 UDP 可以使用相同数字端口。监听地址必须是本机 IP；IPv6 地址在表单中直接填写 `::` 等地址，不加方括号。目标 IP 或域名与端口分别填写。域名在新 TCP 连接或新 UDP 会话创建时解析，不会逐个数据报解析。UDP 没有连接握手，不会因为目标未返回业务响应就切换其他解析地址；域名仅部分地址可达时，请使用明确可达的 IP。非法字段、监听占用和管理端口冲突会在界面提示。监听地址变更涉及自身重叠时，请先停用再修改。

选择 **TCP+UDP** 可用一条规则管理两种协议，共享名称、版本、启停状态、监听 IP 与端口、目标主机与端口。两种协议使用相同的目标数字端口，目标服务需要在该端口分别提供相应的 TCP 和 UDP 服务。组合规则只计一条，TCP 连接与 UDP 会话共用单规则额度；启停、重试、编辑和删除统一生效。任一协议监听端口冲突时整次操作失败，原配置和已有会话保留。只有两个监听都正常时才显示运行中；一侧失效时另一侧也会停止，整条规则显示错误。TCP 和 UDP 筛选均包含组合规则，TCP+UDP 筛选仅显示组合规则。已有两条独立 TCP、UDP 规则保持独立，不会自动合并。

日志输出到标准输出。启用 Web 时，`/healthz` 提供进程和数据库就绪检查；仅命令行安装可用 `sudo porter rules list --lang zh-CN` 检查访问。SQLite 保存用户、套餐、端口分配、规则和累计流量；普通用户密码仅保存带随机盐的派生编码，不保存管理员密码或登录会话。浏览器登录会话在 12 小时后或进程重启后失效。

## 用户、套餐与独立后台

启用 Web 时，管理端 `/admin/` 与用户端 `/user/` 由同一服务提供，但入口、登录、导航、Cookie 和 CSRF 独立；可在同一浏览器同时登录，退出一端不影响另一端。管理员在管理端规则列表查看所有规则及所属账号，可按用户筛选，并直接新增、编辑、启停、删除、重试和重置任意规则的展示统计；普通用户只能操作自己的规则。

管理员先在系统配置设置用户端口池，再创建套餐并为用户选择套餐。套餐设置端口数量和流量额度（GiB）。创建用户时从池内分配一段连续独占数字端口，TCP、UDP 和不同监听地址共用该范围；目标服务端口不受此范围限制。分配避开管理端口、管理员已有规则和已分配端口，连续空间不足则创建失败。修改端口池只影响后续用户，已有分配与规则保持。管理员自有规则无套餐限制，但不能侵占用户已分配端口；管理员操作用户规则同样受其端口和额度限制。

用户套餐用量始终计量，与可选的规则展示统计独立。关闭或清零展示统计、修改或删除规则不会退还套餐用量。以当前用户本周期全部规则的成功转发上行 U、下行 D 为基础：

| 套餐计费方式 | 计费用量 |
| --- | --- |
| 双向 | U + D |
| 单向 | max(U, D) |
| 只计上行 | U |
| 只计下行 | D |

单向先汇总全用户上下行再取较大值，不是每条规则取较大值后相加。达到额度后立即标记耗尽并主动停止该用户全部转发和已有会话，页面无需保持打开，其他用户不受影响；并发在途写入仍可能产生少量超额。周期重置后自动恢复仍设置为启用的规则，手动停用或删除的不恢复。

每个套餐可选择自然月（每月 1 日）、按开户 UTC 日期或固定 1–31 日重置，均在 **UTC 00:00** 生效。短月份没有对应日期时取月末，后续月份恢复原定日期。首次开户取得完整额度，未用额度不结转；停机跨周期后只进入当前周期，不累积补发额度。用户概览显示套餐、端口范围、本周期用量、额度及下次 UTC 重置时间。

管理 API 使用 `/api/admin/*`，用户 API 使用 `/api/user/*`，两端各自提供 `auth/login`、`auth/session` 和 `auth/logout`。管理规则接口为 `/api/admin/rules`（可用 `owner_id` 筛选），用户规则接口为 `/api/user/rules`。升级后旧共享 `/api/auth/*`、`/api/rules*` 不再提供业务接口，调用脚本需同步更新；管理员原有环境变量凭据和规则保留，重新从管理端登录。

当前提供用户和套餐的添加、查看，不包含编辑删除、套餐切换、密码重置、自助注册或支付。

## 流量统计

在新增或编辑规则时打开“流量统计”，即可查看上下行累计流量和当前速度；新规则及升级前的已有规则默认关闭统计。上行表示客户端到目标服务，下行表示目标服务到客户端，TCP+UDP 规则汇总两种协议。统计只计算成功转发的应用载荷，不包含协议头、握手和 TCP 重传，与网卡显示的总流量可能不同。

页面每 5 秒刷新，持续连接不需要断开即可查看统计变化。关闭统计会暂停计数并保留原有累计量，再次开启后继续累加。规则停用、没有活动流量或关闭统计时速度归零；改名、修改目标、监听地址、端口或协议不清空累计量。

点击规则统计区域的“重置流量”并确认，可清空该规则上下行累计量并重新计算速度，不影响现有连接、转发配置或统计开关。统计关闭或规则停用时也可重置。持续传输中的规则重置后会立即累计新流量，因此数值可能很快重新大于零。删除规则会同时移除其统计。

累计量默认每 5 秒保存到 SQLite，可通过 `PORTER_TRAFFIC_FLUSH_INTERVAL` 调整，正常退出时再保存一次；重启后恢复累计量，速度重新采样。强制终止或断电可能丢失最近尚未保存的增量；存储持续失败会扩大未保存范围，需查看错误日志并恢复存储。

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
CGO_ENABLED=0 go build -trimpath -ldflags '-X main.version=dev' -o .tmp/porterd ./cmd/porterd
```

服务端 `porterd` 已内嵌前端，`porter` 是独立客户端，无需独立前端服务器。也可从 [GitHub Releases](https://github.com/lwl0820/porter/releases) 下载相应架构的 Linux 压缩包，解出两个程序。启动服务的方式如下：

```bash
mkdir -p data
export PORTER_ADMIN_USERNAME=admin
read -rsp '管理员密码：' PORTER_ADMIN_PASSWORD
echo
export PORTER_ADMIN_PASSWORD
export PORTER_DB_PATH="$PWD/data/porter.db"
./.tmp/porterd
```

手工以仅命令行模式启动时，设置 `PORTER_WEB_ENABLED=false`、`PORTER_CLI_ENABLED=true`，省略上例 Web 用户名和密码的导出步骤；仅 Web 模式设置 `PORTER_CLI_ENABLED=false`。`porterd --check-config` 可校验当前配置及可用管理方式，不打开数据库或创建监听，也不检查端口是否被占用。

使用发布压缩包时，将最后一行改为 `./porterd`。源码构建的客户端可用 `sudo ./.tmp/porter rules --lang zh-CN --socket "$PWD/data/porter-control.sock"`，发行包客户端使用 `sudo ./porter rules ...`；本机控制目录须满足上文的可信归属限制。同时下载 `SHA256SUMS`，使用 `sha256sum --ignore-missing -c SHA256SUMS` 校验已下载的压缩包，包内两个程序版本一致。普通用户可监听高端口；低端口需要适当的服务权限或 `CAP_NET_BIND_SERVICE`。长期运行时请配置服务管理器以自动重启，手工 unit 的启动程序应为 `porterd`。

## 升级、备份和回退

正式版本发布后，建议固定 `v1.0.0` 之类的版本标签，便于重复部署。升级前停止 Porter，备份**完整数据卷**，包括 SQLite 日志文件。不要单独复制运行中的数据库文件，不要让多个副本共享数据库目录。

程序启动时会将旧 SQLite 数据库依次迁移至版本 4：版本 2 增加 TCP+UDP，版本 3 增加统计开关及累计流量存储，版本 4 增加多用户、套餐、端口分配和用户周期用量。迁移保留已有规则的标识、版本、时间、配置及启停状态，旧规则统计默认关闭。旧程序会拒绝打开超出其支持版本的数据库。降级时必须同时恢复升级前的完整数据备份与旧程序，不要直接用旧二进制打开已迁移数据库。

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

生产目标包括原生二进制 + systemd，以及手工 Linux Docker Engine + Compose v2 部署，架构为 `amd64`、`arm64`；发行版目标包括 Debian 12/13、Ubuntu 22.04/24.04、RHEL 9/10，以及同系 Rocky Linux、AlmaLinux 9/10。原生安装要求 systemd 为 PID 1。首版不承诺 rootless Docker、Docker Desktop、Windows 和 macOS 的生产支持。

这些是兼容目标，不代表所有宿主系统与架构组合均已实测。CI 配置了原生 amd64、arm64 Linux 检查、真实 TCP/UDP 集成测试及 Docker 冒烟测试；CI 成功只代表对应执行环境通过。Debian、Ubuntu、红帽系真实宿主机部署需要单独记录，不能用发行版容器代替宿主机验证。请结合仓库实际工作流结果判断具体版本。

原生安装器检查包含模拟发行包、伪终端、systemctl 命令替身及生成 unit 的静态验证，另以独立程序验证凭据和 HTTP 行为；这些检查不能替代真实 systemd 用户切换、低端口能力或完整发行版宿主安装实测。

## 开发与发布

设置上文缓存环境变量后执行：

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

最后一条命令仅在 `.tmp/release` 生成 Linux amd64、arm64 压缩包与 `SHA256SUMS`，不会发布。PR 只验证，不发布。每次推送 `main` 时执行验证，通过后自动发布 Linux amd64/arm64 多架构镜像 `ghcr.io/lwl0820/porter:latest`，无需版本标签；这些构建的版本信息可定位到源提交，不创建 GitHub Release。

推送 `v1.0.0` 等版本标签时，验证通过后发布对应版本的多架构 GHCR 镜像、更新 `latest`，并创建 GitHub Release 压缩包。`v1.0.0-rc.1` 等预发布标签发布预发行版，不会覆盖 `latest`。由于 `latest` 可能来自尚未打标签的 `main` 构建，需要固定版本的部署请使用明确的正式版本标签。Actions 固定 commit SHA，发布使用仓库 `GITHUB_TOKEN`。

首次公开发布前，需要启用仓库所需的 Actions 权限、确认允许发布软件包、将 GHCR 软件包设为公开，并实际验证匿名拉取。写入工作流配置本身不会产生远端发行版。
