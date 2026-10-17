# 原生命令行与本机控制契约

## 1. 范围与触发条件

修改原生规则命令、交互终端、语言引导、本机套接字、程序入口或 Docker CLI 禁用边界时适用。业务规则仍以 [后端合同](contracts.md) 的权限、配额、revision 及提交回滚语义为准。

## 2. 签名与职责

- `cli.Run(ctx, args, stdout, stderr) int`：在服务日志、凭据配置及数据库初始化前分流命令；返回进程退出码。
- `native.Available() bool`：Linux 原生运行识别；`porter_docker` 构建固定不可用，环境变量不可解除。
- `localapi.Start(path, service, admin) (*Server, error)`：支持环境启动独立 Unix 控制监听，不支持环境返回 nil 且不创建节点。
- `localapi.NewClient(path)`：HTTP over Unix socket，提供 List/Get/Create/Update/Delete，不接受远程 HTTP 地址替换通道。
- `rules.Service.GetFor(actor, id)`：在服务锁内授权并返回权威运行态及归属；本机与 Web 复用既有 SaveFor/DeleteFor，不另开 Store。
- `porter rules` 进入交互菜单；`porter rules add|list|get|update|delete` 为单次参数操作。无任何子命令仍启动服务，`--version` 保持独立版本查询。

## 3. 请求、输出与环境合同

### 3.1 命令与双语

- 未指定 `--lang` 时，第一步经 `/dev/tty` 选择中文/English；不得按系统语言或历史状态跳过。无效/空白输入重试，取消或 EOF 不执行规则操作。
- 显式 `--lang zh-CN|en` 或等号形式跳过选择；动作、字段值与标识保持稳定，帮助、菜单、校验和业务提示遵循所选语言。
- 语言确定后才完整校验命令、检查权限和连接服务。一次菜单会话只选一次语言。
- 参数模式缺少字段直接报错，不进入向导；完整参数并指定语言允许无人值守。布尔配置用 `--enabled=false`、`--traffic-enabled=false`，省略与 false 不得混同。
- 可读输出为表格/详情；`--json` 仅用于单次参数操作，字段沿用模型 snake_case、流量字节十进制字符串及稳定状态/错误码。
- JSON 成功写 stdout，错误写 stderr，不混入日志或终端选择；退出码成功 0、操作失败 1、用法错误 2。
- 创建默认管理员归属、启用、展示统计关闭；允许 --owner-id 指定已有用户，修改不转移归属。CLI 不新增用户/套餐/端口池管理命令。
- update/delete 读取快照 revision 后提交，可用 --revision 固定先前版本；发生冲突不得自动覆盖或重试。超时提示先查结果，不能自动重发创建。

### 3.2 交互管理

- 菜单覆盖创建、列表、详情、修改、删除和退出；通过稳定 ID 映射当前序号，不持久化序号。
- 创建/修改展示预览并确认后提交；编辑留空保留当前值。删除展示详情并显式确认，默认取消；参数删除按指定 ID 直接执行。
- 菜单、字段、选择和确认均通过控制终端，不消费 stdin 管道。取消只丢弃待提交输入；EOF/中断不能把半行内容当作提交。
- 修改/删除使用展示快照的 revision，确认期间发生其他变更必须拒绝；不得提交前再获取新版本覆盖确认对象。

### 3.3 本机控制与部署

- 服务默认在数据库目录旁创建 `porter-control.sock`；`PORTER_CONTROL_SOCKET` 可覆盖。CLI 路径优先级 `--socket`、该环境变量、`/var/lib/porter/porter-control.sock`。
- 控制协议限定 `/v1/rules` 的 GET/POST 及 `/v1/rules/{id}` 的 GET/PUT/DELETE，使用既有 Rule/Input/AdminRuleCreateInput，写操作携带 If-Match。
- 独立 HTTP over Unix socket handler 不挂到 Web TCP mux；请求头、请求体和客户端 UID 自检不能替代服务端 `SO_PEERCRED` 的实际 UID 0 校验。
- socket 0600，直接父目录及全部祖先必须属于 root 或运行服务的 UID，不能有 group/other 写权限或使用符号链接；外国 owner 即使当前为 0555 也不能信任，因为 owner 能自行 chmod 恢复写权限。拒绝普通文件、未知节点和活动 socket，仅清理已确认归属且确实失效的节点。退出只清理自身创建的 inode。
- 服务关闭先停止本机请求、等待在途请求，再关闭规则服务最终保存和数据库。CLI 服务不可用时不得离线写库或另起服务。
- Dockerfile 以 `-tags porter_docker` 编译；CLI 拒绝且服务不创建控制入口。原生构建也识别已知容器标记；不宣称可检测故意隐藏所有运行线索的环境。
- 不新增 schema 迁移、安装 unit 或全局 PATH 链接；原生安装及自定义程序目录使用对应二进制绝对路径。

## 4. 校验与错误矩阵

| 条件 | 行为 |
| --- | --- |
| 无语言且无控制终端 | 双语 language_required，用法退出码 2，不连接服务。 |
| 非法显式语言 | 双语 language_invalid，退出码 2。 |
| 参数缺失、未知动作、update 无字段、菜单配 JSON | 用法错误，不补问业务字段、不写库。 |
| 非 root 客户端或实际非 root 对端 | 客户端 root_required；服务端 forbidden，拒绝操作。 |
| Docker/不支持环境 | native_only，规则操作失败，服务控制入口禁用。 |
| 服务停止或 socket 不匹配 | service_unavailable，提示检查服务/路径/升级重启，不写库。 |
| 过期版本、规则冲突、用户端口/配额或存储失败 | 沿用既有业务错误与回滚，不自动降权或覆盖。 |
| 请求超时 | 提示先查询结果，不自动重发写请求。 |

## 5. 正常、基础与异常案例

- 正常：无语言进入菜单先选 English，再创建 TCP+UDP；Web 刷新看到同一 ID，两种传输均正常。
- 基础：`porter rules list --lang zh-CN --json` 在无终端时返回结构化列表；帮助无须服务和管理员凭据。
- 异常：菜单展示规则后 Web 更新，原菜单确认修改/删除须报 revision_mismatch；取消、断开终端不提交待处理输入。

## 6. 必须测试及断言

- CLI 参数和伪终端覆盖两语言、先选语言、缺终端/参数、菜单完整 CRUD、取消/EOF/中断、JSON 与管道隔离。
- 本机控制测试实际 UID 对端允许/拒绝、socket 权限/路径/失效节点/替换清理、版本冲突、归属及服务回滚；不能用生产授权绕过替代正向测试。
- 独立进程验证原生 CLI 三种协议真实转发、Web 同步、只改名保留连接、停机拒绝及重启恢复。
- Docker smoke 同时断言 native_only、默认及自定义控制节点均不存在，并继续执行低端口、真实 TCP/UDP 和重建恢复。
- 执行普通及 porter_docker 标签 Go 检查、race、原生 amd64/arm64 构建及安装卸载回归；用户命名空间 UID 0 测试与真实 systemd 宿主实测分开记录。

## 7. 错误与正确写法

错误：没有语言就默认中文；参数不足时进入向导；CLI 另开 SQLite 写配置；仅在客户端判断 root 后信任本机请求；确认删除前取新 revision。

正确：首先确定语言，按明确入口进入菜单或参数模式；由真实 root 对端的受限本机通道调用同一规则服务，提交展示/读取时的版本，失败保留既有业务语义。
