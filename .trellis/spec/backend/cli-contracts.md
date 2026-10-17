# 原生命令行与本机控制契约

## 1. 范围与触发条件

修改原生规则命令、交互终端、语言引导、本机套接字、程序入口或 Docker CLI 禁用边界时适用。业务规则仍以 [后端合同](contracts.md) 的权限、配额、revision 及提交回滚语义为准。

## 2. 签名与职责

- `cmd/porter` 是独立客户端入口，调用 `cli.Run(ctx, args, stdout, stderr) int` 返回退出码，不加载服务配置或数据库；`cmd/porterd` 是服务入口，不依赖 CLI 界面。
- `native.Available() bool`：Linux 原生运行识别；`porter_docker` 构建固定不可用，环境变量不可解除。
- `localapi.Start(path, service, admin) (*Server, error)`：支持环境启动独立 Unix 控制监听，不支持环境返回 nil 且不创建节点。
- `controlapi.NewClient(path)`：HTTP over Unix socket，提供 List/Get/Create/Update/Delete，不接受远程 HTTP 地址替换通道。
- `rules.Service.GetFor(actor, id)`：在服务锁内授权并返回权威运行态及归属；本机与 Web 复用既有 SaveFor/DeleteFor，不另开 Store。
- `porter` 或 `porter rules` 进入交互菜单；`porter rules add|list|get|update|delete` 为单次参数操作。`porterd` 无参数启动服务；两者 `--version` 保持独立版本查询，不读写语言配置。
- `internal/controlapi` 承载模型、错误与 HTTP over Unix socket 客户端，不能反向依赖 localapi/rules/store；CLI 构建依赖不得含 config/store/rules/forward/httpapi/web，服务入口不引用 internal/cli。

Web/CLI可选组件、仅Web不部署客户端/控制socket、仅CLI不要求Web凭据以及安装后切换，见 [组件选择合同](optional-management-contracts.md)。原生授权与独立客户端职责不变。

## 3. 请求、输出与环境合同

### 3.1 命令与双语

- 语言优先级：显式 `--lang` > `/etc/porter/cli.json` 已保存设置 > 首次 `/dev/tty` 选择。首次菜单 `1 English / 2 中文`，回车默认英文；取消、EOF、中断不保存、不执行规则操作，不按系统 locale 猜测。
- 显式 `--lang zh-CN|en` 或等号形式仅覆盖本次、不写保存值；动作、字段值与标识保持稳定，帮助、菜单、校验和业务提示遵循所选语言。菜单“切换语言”确认后更新当前语言并保存。
- 语言确定后才完整校验命令、检查权限和连接服务。一次菜单会话只选一次语言。
- 参数模式缺少字段直接报错，不进入向导；完整参数配显式或已保存语言允许无人值守。布尔配置用 `--enabled=false`、`--traffic-enabled=false`，省略与 false 不得混同。
- 可读输出为表格/详情；`--json` 仅用于单次参数操作，字段沿用模型 snake_case、流量字节十进制字符串及稳定状态/错误码。
- JSON 成功写 stdout，错误写 stderr，不混入日志或终端选择；退出码成功 0、操作失败 1、用法错误 2。
- 创建默认管理员归属、启用、展示统计关闭；允许 --owner-id 指定已有用户，修改不转移归属。CLI 不新增用户/套餐/端口池管理命令。
- update/delete 读取快照 revision 后提交，可用 --revision 固定先前版本；发生冲突不得自动覆盖或重试。超时提示先查结果，不能自动重发创建。

### 3.2 交互管理

- 菜单覆盖创建、列表、详情、修改、删除、切换语言和退出；通过稳定 ID 映射当前序号，不持久化序号。
- 创建/修改展示预览并确认后提交；编辑留空保留当前值。删除展示详情并显式确认，默认取消；参数删除按指定 ID 直接执行。
- 菜单、字段、选择和确认均通过控制终端，不消费 stdin 管道。取消只丢弃待提交输入；EOF/中断不能把半行内容当作提交。
- 修改/删除使用展示快照的 revision，确认期间发生其他变更必须拒绝；不得提交前再获取新版本覆盖确认对象。

### 3.3 本机控制与部署

- 服务默认在数据库目录旁创建 `porter-control.sock`；`PORTER_CONTROL_SOCKET` 可覆盖。CLI 路径优先级 `--socket`、该环境变量、`/var/lib/porter/porter-control.sock`。
- 控制协议限定 `/v1/rules` 的 GET/POST 及 `/v1/rules/{id}` 的 GET/PUT/DELETE，使用既有 Rule/Input/AdminRuleCreateInput，写操作携带 If-Match。
- 独立 HTTP over Unix socket handler 不挂到 Web TCP mux；请求头、请求体和客户端 UID 自检不能替代服务端 `SO_PEERCRED` 的实际 UID 0 校验。
- socket 0600，直接父目录及全部祖先必须属于 root 或运行服务的 UID，不能有 group/other 写权限或使用符号链接；外国 owner 即使当前为 0555 也不能信任，因为 owner 能自行 chmod 恢复写权限。拒绝普通文件、未知节点和活动 socket，仅清理已确认归属且确实失效的节点。退出只清理自身创建的 inode。
- 服务关闭先停止本机请求、等待在途请求，再关闭规则服务最终保存和数据库。CLI 服务不可用时不得离线写库或另起服务。
- Dockerfile 以 `-tags porter_docker` 仅编译 cmd/porterd，镜像入口 /porterd，无 porter 客户端；真实本机监听实现用构建约束排除，stub 不创建控制入口。原生构建也识别已知容器标记；挂入容器的原生 CLI 拒绝规则管理，不宣称可检测故意隐藏所有运行线索的环境。
- 原生发行包含同版本 porter 与 porterd，安装器通过托管 /usr/bin/porter 指向真实客户端，porter.service 启动真实 porterd。安装、升级、卸载与路径归属见 [原生安装与升级契约](native-install-contracts.md)；业务数据库 schema 不因拆分改变。

### 3.4 本地语言配置

- 固定 `/etc/porter/cli.json`，JSON `{ "schema": 1, "language": "en" }`，仅支持 en/zh-CN；与服务凭据 porter.env、安装记录 install.json 分开，不写业务数据库。root/sudo 共用主机偏好；安装器选语言不预填 CLI 偏好。
- 限制文件大小4096字节，严格校验schema与语言，拒绝多余JSON/未知字段、不可信目录、错误属主/模式和符号链接。配置为root所有普通文件0600；只读调用、非root、容器不创建配置。
- 首次选择保存及菜单切换保存使用同目录受限临时文件+原子替换，并用安装器同一 `/etc/porter/.install-lock` 串行化；不持锁等待终端输入。锁内重校初次读取时的目录及安装记录快照，卸载或升级状态变化时拒绝写入，不重建刚删除的配置目录。
- 首次并发保存发现已有合法偏好则采用已有值，不悄然覆盖；菜单明确切换才允许覆盖。损坏设置可见报错，不能静默覆盖；显式 --lang 可跳过配置问题完成单次调用。
- 保存失败明确警告仅本次有效，不能宣称已保存；所有提示不得污染 JSON stdout。普通卸载保留、恢复复用；purge 清理CLI配置及服务配置，未知类型冲突须在停服前拒绝。

## 4. 校验与错误矩阵

| 条件 | 行为 |
| --- | --- |
| 无显式或保存语言且无控制终端 | 英文 language_required，用法退出码 2，不连接服务。 |
| 非法显式语言 | language_invalid，退出码 2。 |
| 不可信或损坏语言配置 | preference_invalid，不静默覆盖；可通过 --lang 单次使用。 |
| 偏好保存失败或只读调用 | preference_save_failed/preference_readonly，说明未持久化，当前使用已选语言。 |
| 参数缺失、未知动作、update 无字段、菜单配 JSON | 用法错误，不补问业务字段、不写库。 |
| 非 root 客户端或实际非 root 对端 | 客户端 root_required；服务端 forbidden，拒绝操作。 |
| Docker/不支持环境 | native_only，规则操作失败，服务控制入口禁用。 |
| 服务停止或 socket 不匹配 | service_unavailable，提示检查服务/路径/升级重启，不写库。 |
| 过期版本、规则冲突、用户端口/配额或存储失败 | 沿用既有业务错误与回滚，不自动降权或覆盖。 |
| 请求超时 | 提示先查询结果，不自动重发写请求。 |

## 5. 正常、基础与异常案例

- 正常：首次无语言进入菜单回车选择 English并保存，再创建 TCP+UDP；Web 刷新看到同一 ID，两种传输均正常；下次启动直接复用English。
- 基础：`porter rules list --lang zh-CN --json` 在无终端时返回结构化列表；帮助无须服务和管理员凭据。
- 异常：菜单展示规则后 Web 更新，原菜单确认修改/删除须报 revision_mismatch；取消、断开终端不提交待处理输入。

## 6. 必须测试及断言

- CLI 参数和伪终端覆盖首次两语言/回车默认/保存、后续复用、显式单次覆盖、菜单切换、缺终端/参数、完整 CRUD、取消/EOF/中断、JSON 与管道隔离；配置损坏/链接/权限/大小/并发/卸载中重校及保存失败有断言。
- 本机控制测试实际 UID 对端允许/拒绝、socket 权限/路径/失效节点/替换清理、版本冲突、归属及服务回滚；不能用生产授权绕过替代正向测试。
- 独立进程验证原生 CLI 三种协议真实转发、Web 同步、只改名保留连接、停机拒绝及重启恢复。
- Docker smoke 断言镜像无客户端、服务入口拒绝客户端参数，默认及自定义控制节点均不存在，并继续执行低端口、真实 TCP/UDP 和重建恢复；外部挂入原生客户端另测 native_only。
- 执行普通及 porter_docker 标签 Go 检查、race、原生 amd64/arm64 构建及安装卸载回归；用户命名空间 UID 0 测试与真实 systemd 宿主实测分开记录。

## 7. 错误与正确写法

错误：每次启动都要求重选语言；把CLI偏好写入业务数据库或服务凭据文件；显式--lang悄然覆盖保存值；参数不足时进入向导；CLI 另开 SQLite 写配置；仅在客户端判断 root 后信任本机请求；确认删除前取新 revision。

正确：首先确定语言，按明确入口进入菜单或参数模式；由真实 root 对端的受限本机通道调用同一规则服务，提交展示/读取时的版本，失败保留既有业务语义。
