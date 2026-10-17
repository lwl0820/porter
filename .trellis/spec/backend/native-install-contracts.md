# 原生双程序安装、卸载与升级契约


## 10.1 范围与触发条件

修改 `scripts/install.sh`、README 一键入口、安装器验证、GitHub 发行包或 systemd 集成时适用。安装器下载原生程序，不安装或调用 Docker。现有手工 Docker 部署与镜像发布保持独立。

三模式安装与切换新增合同见 [Web与CLI组件选择](optional-management-contracts.md)，它扩展本文schema1/2的兼容并覆盖无条件要求Web/双程序的旧行为。路径、锁、备份、归属和卸载保护继续适用。

## 10.2 命令签名

`bash scripts/install.sh [--install-dir /opt/porter] [--version v1.0.0] [--lang zh-CN|en]` 默认安装；`--upgrade [--version v1.0.0]` 显式升级或将旧单程序迁移为双程序；`--uninstall [--purge]` 卸载，purge 不可独用，卸载拒绝 version/upgrade。`--help` 按语言输出，不要求权限、宿主检测或终端，lang 位置无关。远程入口 `curl -fsSL https://raw.githubusercontent.com/lwl0820/porter/main/scripts/install.sh | bash`，传参用 `bash -s -- --lang en`。宿主操作用 root/sudo。默认最新正式 Release，显式版本允许已发布预发行版。

## 10.3 输入、文件及服务契约

- Web启用时通过 `/dev/tty` 输入管理员用户名、隐藏密码、密码确认、完整 Web 监听地址，不从脚本管道读取，不生成默认密码。先关闭终端回显再显示密码提示，EOF、INT、TERM 和正常退出均恢复回显。
- 安装器未指定 lang 时显示1English/2中文，回车或无终端默认英文。自有帮助、提示、校验、确认和结果均翻译，外部命令诊断除外。集中消息用参数插值，所有交互读控制终端；CLI独立的首次语言保存见 cli-contracts.md，安装器不得预填或覆盖该偏好。
- 用户名去除首尾空白，密码保留原始值但不能全为空白；监听为显式 IPv4:端口或 [IPv6]:端口，范围 1–65535。通配监听通过同族回环地址检查健康，禁止固定使用 8080。
- 目标 Ubuntu 22.04/24.04、Debian 12/13、RHEL/Rocky/AlmaLinux 9/10，架构 amd64/arm64；要求 systemd 为 PID 1。必要 Python 3、curl 和 CA 依赖通过系统 apt/dnf 补齐，不安装编译工具或 Docker，不自动卸载软件包。
- GitHub API 获取正式 latest 或指定 tag；精确选择 `porter-${version}-linux-${arch}.tar.gz` 与 `SHA256SUMS`。校验唯一的对应 SHA256 项，新装/升级按组件读取压缩包中名为 `porter`、`porterd` 的唯一普通文件，不抽取任意路径/链接；分别执行暂存程序 `--version` 核对同一发布版本。仅旧schema=1同版本恢复可接受单个porter成员。
- 默认服务 `/opt/porter/porterd`、客户端 `/opt/porter/porter`（目录可指定）、服务配置 `/etc/porter/porter.env`、CLI配置 `/etc/porter/cli.json`、数据 `/var/lib/porter`、unit `/etc/systemd/system/porter.service`。配置与安装状态权限600，数据目录700，程序755；新unit启动porterd，旧布局仍启动旧porter。
- 环境文件逐项使用双引号，编码反斜线、双引号、反引号和美元符号；生产只通过标准输入和受限解析读取，禁止 shell source，禁止 Compose 的 `$$` 替换。文件内的 `PORTER_DB_PATH` 固定为 `/var/lib/porter/porter.db`。
- `porter.service` 使用专用禁止登录的系统用户/组 porter、`Restart=on-failure`、开机自启。`AmbientCapabilities` 和 `CapabilityBoundingSet` 仅提供 `CAP_NET_BIND_SERVICE`，配合 `NoNewPrivileges`、`ProtectSystem=strict`、`StateDirectory=porter` 允许完整数据库目录写入。
- unit 按各字段语法编码：`EnvironmentFile=/etc/porter/porter.env` 不包外层引号，避免指令被忽略；ExecStart 冒号禁用变量展开，百分号写 `%%`。目录拒绝控制字符/引号/反斜线，空格/美元符/百分号须验证。FragmentPath 禁止覆盖 vendor 同名服务。
- `/etc/porter/install.json`：兼容schema=1旧单程序和schema=2双程序记录；新schema=3在split布局、directory、arch、version、status及受限command_link之外增加严格features组件选择（见组件合同）；状态含initializing/complete/uninstalling/uninstalled与显式升级恢复状态。目录/全局锁仅释放本进程取得者；暂存仅安装目录.tmp。状态/凭据/unit在目标目录受限.new写完再原子改名。产物校验后才记initializing；普通重试复用配置/程序/数据及原版本布局，不隐式升级。CLI保存使用同一全局.install-lock并重校安装快照。
- 已托管安装取得全局锁后重新校验 directory/arch/version/status，记录变化、消失或无效即拒绝重跑，避免沿用已卸载快照。FragmentPath/DropInPaths 查询必须检查退出码，失败不能视为空。
- 已有完整安装须在锁内核对实际程序版本与记录一致（split布局按组件核对所需程序），缺失关键文件、数据目录或 `porter.db` 时拒绝继续，不能自动创建空库冒充恢复；目录/架构不匹配、未知账号/配置/unit/数据库或符号链接等也明确拒绝接管。已托管用户校验 home、nologin 与主组，不能把未知普通用户变为服务用户。
- 系统命令固定 `/usr/bin/porter`，绝对链接指向托管客户端；程序直接安装/usr/bin时使用本体而非自链接。不改PATH/profile/sudoers，不认领未知同目标链接，不覆盖悬空或其他节点；先记录受限管理意图，再非强制原子创建，锁内重校归属。缺失已托管链接可幂等补齐，不能把缺失完整程序伪装为链接修复。
- 新装就绪需 unit 已启用且运行；Web所选地址 `/healthz` 返回 `{"status":"ok"}`，CLI按组件合同执行root Unix只读查询，探测后再次确认运行。按所选组件输出实际版本、布局、可用Web入口、配置/数据/所需程序位置、可用系统CLI及systemctl/journalctl命令；旧布局明确提示显式升级方法，不执行rules探测而启动旧服务。根地址进入用户端，失败不得报就绪。
- 卸载从记录识别自定义目录，显式目录须匹配；拒绝未知 FragmentPath/drop-in、不匹配 unit/账号及含中间路径的符号链接。无需网络、下载、补装依赖或凭据输入。控制终端列出删除/保留资源后输入 `yes`，purge 另输入 `DELETE`；默认取消，全部确认先于状态写入和停服。
- 卸载写 uninstalling，停服、禁用并确认不活跃后删除托管 unit/程序，再 reload；失败停止，保留恢复信息。默认保留服务/CLI正式配置、完整数据、权限600记录及服务账号/组，写uninstalled；purge清理两种配置、完整数据及账号/组，安装记录最后删；普通及purge均精确删除托管CLI入口，未知替换在停服前拒绝。额外文件、共享 journal、系统包和 Docker 资源保留，程序目录仅空时删除。
- 预检现存配置/数据库为普通文件、数据为目录；两种卸载均核验并精确清理 `$CONFIG.new`、`$STATE.new`、`$UNIT.new`、`$INSTALL_DIR/.porter.new`、split布局的`.porterd.new`及可确认归属的CLI配置暂存，拒绝类型冲突及链接。禁止递归清理程序目录或任意 `.tmp`；README 须说明强制中断后的历史 `.tmp/install.XXXXXXXX` 可能含配置/凭据副本，保留并需人工核对。
- uninstalled重装要求原服务配置/数据库存在，保留原CLI偏好，下载校验记录的原版本和原布局；恢复文件前转initializing使失败可重试；历史版本不可下载时报错，不升级。uninstalling 禁止安装、允许续卸载，容忍已删资源及缓存的原托管 FragmentPath，仍拒绝其他来源。complete 缺库/文件保护不变。

## 10.4 校验与错误矩阵

| 条件 | 行为 |
| --- | --- |
| 不支持系统/架构、非 systemd 宿主、非法参数或不能提权 | 非零退出，不部署服务。 |
| 首次无控制终端 | 明确报错，不读取脚本内容作为用户输入。 |
| 空白用户名/密码、密码确认不一致、无效 IP/端口 | 重试输入，不写入无效配置。 |
| latest 不存在、API/网络失败、缺架构产物或校验和 | 非零退出，不声称已有远端发行版。 |
| 摘要不符、重复校验项、目录/链接代替程序、版本不匹配 | 不安装暂存程序，不覆盖既有可用版本。 |
| 未托管文件/账号/unit、安装状态不一致或路径冲突 | 保留原状，提示人工处理，不能清理数据。 |
| systemd启动/启用或健康检查失败 | 新安装保留初始化状态与配置供重试；显式升级须按持久阶段恢复匹配原程序/unit/状态及完整数据。 |
| 卸载取消/确认不匹配/无终端 | 不停服、不删除、不更新状态；无终端报错。 |
| 卸载未知资源/类型或记录冲突 | 拒绝接管与删除。 |
| 停服/禁用/删除/reload/账号清理失败 | 非零，不报成功，保留续卸载信息。 |
| 重复卸载 | 已卸载提示；purge仍确认。无记录且无资源为空操作，否则拒绝。 |
| 重装缺原配置/数据库/历史包 | 非零，不建空库、不重置、不升级。 |
| 显式升级目标为单程序包/低于当前版本/未知资源冲突 | 停服前拒绝，不覆盖旧实例；版本比较遵循SemVer。 |
| DATA自身为挂载根，或无法可靠读取挂载信息 | 升级停服前拒绝；不能等rename返回EBUSY时才发现无法回滚。 |
| 升级中断或回滚失败 | 按持久阶段恢复，保留唯一备份及明确指引，禁止不完整布局继续启动/卸载。 |

## 10.5 正常、基础与异常案例

- 正常：交互输入包含 `$`、反引号、引号、反斜线、中文和首尾空格的密码，最终程序凭据保持原值；IPv6 通配监听使用 `[::1]` 探测。
- 基础：再次执行不下载新版本、不重置配置；第一次启动失败后复用原始版本及凭据重试。数据库保存整个目录，不能复制运行中的单个 db 文件作为完整备份；备份包含凭据，目录权限700、归档权限600。
- 异常：未知 vendor unit、服务账号、损坏压缩包或不匹配校验和不能被安装器“修复”为新部署；回退旧程序需要匹配的完整数据备份。
- 卸载保留配置/数据库字节和所有权，同版本重装复用凭据；中英文帮助/取消可用，删 unit 后 reload 失败及 purge 已删账号后中断均可续卸载。

## 10.6 必需验证

执行 Bash 语法/帮助、`python3 scripts/test-install.py`、actionlint、`git diff --check`。PTY、模拟发布包及命令替身覆盖系统/架构、sudo、输入/回显、摘要、unit、重试/恢复。生成 unit 用 `systemd-analyze verify` 验证，退出0且 stderr 无忽略/无效指令警告；独立二进制验证生成配置的凭据、健康/登录。

新增断言：双语帮助/参数顺序/语言选择、回车/无终端英文默认；真实依赖边界、双架构双程序包、PATH/链接归属、schema1/2恢复和显式升级故障矩阵；卸载取消/二次确认、精确保留删除、目录识别、额外文件、链接/类型/未知资源拒绝；锁后状态变化拒绝；停服/禁用失败不删除、reload及账号清理中断可重试；同版本重装保留配置/数据库，恢复中断可重试。日志不得有卸载网络、包管理或凭据请求。

开发检查不能创建真实全局账号/服务或使用业务数据库；临时文件仅项目 `.tmp`。开发环境 PID1 不是 systemd 时，明确区分 unit 静态验证、实际程序验证和模拟管理命令，不能声称已经验证真实 systemd 用户切换、能力边界及全部发行版宿主安装。原 Docker 安装器测试结果不能用作原生方案的通过证据。

## 10.7 错误与正确写法

错误：下载 Actions 临时附件或在目标机编译；把 EnvironmentFile 当 Compose 文件转义；先覆盖程序再验证校验和；从标准输入读取密码或给未知 porter.service 写入覆盖文件。

正确：复用公开 Release 资产和 SHA256SUMS，先暂存校验，再创建原生受限服务；从独立终端交互，密码按 systemd EnvironmentFile 语义编码；用安装状态和实际 FragmentPath 判断能否恢复已有部署。

错误：删程序却留 complete 导致无法重装；取消 complete 缺库保护；递归删除程序目录。

正确：卸载记录 uninstalling/uninstalled，恢复前转 initializing；complete 缺库仍拒绝；精确删除，目录仅空时清理。

## 10.8 显式升级与单程序迁移

- 签名：`--upgrade [--version TAG]`，必须已有可信托管安装；无version选择最新正式Release，有version精确选择已发布目标。普通安装不把--version解释成升级，显式升级不接受低于当前SemVer的版本或只有旧porter成员的包。已拆分同版本为幂等检查，不重复停服。
- schema=1旧布局普通重跑/卸载恢复保持原版本，不能直接以新CLI覆盖旧服务路径。显式迁移成套切换客户端porter、服务porterd与unit ExecStart；不更改业务账号、规则、统计及服务配置语义。
- 下载、唯一摘要、两程序真实版本、资源类型/归属、原unit和账号、CLI配置/系统入口冲突的验证先于停服。DATA本身必须可整体替换：从/proc/self/mountinfo精确识别挂载根（含同文件系统bind mount，解码内核路径转义），预检及锁内重检发现DATA自身是挂载根则停服前拒绝；升级和组件切换取双锁check_managed后都要显式data-check给具体双语诊断，不能只依赖事务入口泛化错误兜底；DATA父目录或/var单独挂载不受此限制。不能仅用os.path.ismount或st_dev判断，二者会漏同fs bind mount。终端展示目标、备份位置和停机影响，确认后持两把锁重新验证快照；取消不修改程序、配置、状态或业务数据。
- 持久升级记录使用status=upgrading和受限upgrade_id；工作目录为安装目录`.tmp/upgrade.<32hex>`，transaction.json记录prepared/stopped/backup_complete/switching/restoring/restored。路径必须从验证后的安装目录与受限标识推导，不能信任JSON指定任意删除/恢复路径。
- 备份保存原install.json精确字节、原程序、unit、服务/CLI配置与完整数据目录；先停止服务并确认inactive，保留UID/GID、mode与内容，目录0700、含配置的归档/文件0600。仅copy2/copytree不等于保留属主，必须显式验证恢复后服务用户仍能访问。
- 备份完成标记必须先于程序/unit/入口切换。新程序健康后才提交complete，并保留原active/enabled意图；原本停止的实例只临时启动验收后再停止。旧数据文件与新目录不能合并回退。
- 失败/信号/强制中断后，根据受限记录恢复稳定布局。备份未完整不得拿半备份覆盖业务数据；恢复完整备份前必须停止新服务并确认inactive。恢复阶段同样须可重入，回滚失败保留唯一备份与可执行恢复提示，禁止继续正常安装/卸载。
- 普通cleanup不得删除未完成升级备份；历史/未知升级备份不被purge递归删除，文档明确其可能含凭据；DATA父目录`.porter-failed-upgrade.<32hex>`中的失败新版数据也保留，purge双语提示与README须说明实际位置及可能含业务数据。可识别配置文件本体及CLI配置在purge中精确清理。
- 测试必须断言：旧布局与split升级、SemVer/同版/降级/单文件目标、取消、下载/摘要失败不停止旧服务，锁后状态改变，以及停服/备份中/备份完成/每次程序和unit替换/入口/健康/状态提交/回滚各阶段中断；原版本、配置数据字节及owner/mode、active/enabled、托管归属及备份保留均须可观察验证。未知锁与活动进程锁始终保守拒绝，残锁恢复不能误删新锁破坏互斥。

### 安装器残锁与CLI共享互斥

安装器锁仍为目录型.install-lock，新安装器创建root0700目录和受限owner.json，记录pid与/proc/pid/stat的starttime。无owner的CLI短锁、未知/非法owner、额外节点或活动进程锁均不能回收。回收死进程锁需O_NOFOLLOW打开owner并flock独占非阻塞，锁内重校目录与owner的fd/path inode，再确认PID不存在或starttime不符；只unlink已校验owner并rmdir空目录，重新独占创建自己的锁。持旧fd的第二恢复者必须重校失败，不能删除后来创建的新锁。不能用rm -rf删除可能已被替换的锁目录。
