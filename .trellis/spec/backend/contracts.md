# 后端与部署契约

## 1. 范围与触发条件

适用于修改 Go 管理接口、规则运行态、SQLite、环境变量和容器集成。实际入口为 `cmd/porter/main.go`，边界模型由 `internal/model/rule.go` 维护；以真实代码和测试为依据。

## 2. 签名

- `config.Load() (Config, error)`：必填管理员环境变量，显式管理 IP 与端口。
- `store.Open(path string) (*Store, error)`：取得 `.lock` 排他锁、迁移后返回，不能多实例共享数据库。
- `rules.Service.Save(ctx, id, revision, input)`：创建时 id 为空，更新要求当前 revision。
- `model.Transports(protocol) []string`：`tcp`、`udp` 映射单个传输，`tcp+udp` 映射两个传输，未知值返回 nil；冲突使用传输集合交集。
- `forward.Engine.Prepare(model.Rule) (*Runtime, error)`：只接收物理 TCP 或 UDP 配置，绑定端口但不转发；`Start()`、`Update(rule)`、`Stop()`、`Error()` 分离准备与生效。`Failed() <-chan struct{}` 只通知不可恢复监听故障，正常 Stop 和目标不可达不触发。
- `rules.runtimeGroup`：每个逻辑规则关联一个运行组，以协议为键持有物理 Runtime；Service 串行控制组的准备、替换、监控和停止。
- SQL 迁移放在 `internal/store/migrations/` 并内嵌，以 `PRAGMA user_version` 管理。保留 `001_init.sql`；`002_tcp_udp.sql` 将协议 CHECK 扩展为三个值；`003_traffic.sql` 增加默认关闭的统计开关和累计表，当前版本为 3。v1→v2 事务复制全部字段并保留自定义索引和触发器；未知列、外部外键依赖或迁移失败须拒绝并回滚，不能静默丢弃。v2→v3 增量 ALTER，不重建 rules 表。
- `.github/workflows/ci.yml`：PR 与 `workflow_call` 验证入口；`release.yml`：main push 和 v* tag push 的验证及发布入口。同一次 main 推送只执行一套验证。

## 3. 请求、响应与环境契约

规则输入只包含 `name/protocol/listen_host/listen_port/target_host/target_port/enabled/traffic_enabled?`，不得让客户端设置 id、revision、时间或统计快照。返回 Rule 包含这些配置及 `id/revision/created_at/updated_at/status/last_error_code?/last_error?/traffic`，统计契约见下方场景。

`protocol` 只接受 `tcp`、`udp`、`tcp+udp`。组合规则使用一条记录、一个 id/revision、同一监听 IP/数字端口及同一目标主机/数字端口；启停、重试和删除整体执行。现有两条单协议规则不自动合并。组合内两个 Runtime 按同一非空 Rule.ID 共享单规则会话预算；预备实例也持有预算引用，最后实例停止且会话释放后才回收预算。

`GET /api/rules` 返回 `{rules: Rule[]}`；新增返回 201，修改返回 200，删除完成后返回 204。更新、删除和重试携带 `If-Match: "<revision>"`。登录与当前会话返回 `{username, csrf_token}`，其余修改类请求携带 `X-CSRF-Token` 和同源 Origin。

管理员环境变量 `PORTER_ADMIN_USERNAME`、`PORTER_ADMIN_PASSWORD` 必填；`PORTER_HTTP_ADDR` 默认 `0.0.0.0:8080`，`PORTER_DB_PATH` 默认 `/data/porter.db`。管理监听按 IPv4/IPv6 显式选择 tcp4/tcp6；转发监听同样固定地址族，不能依赖系统双栈默认值。

写操作由 Service 串行化。先准备新监听，再提交数据库，成功后切换运行态；失败必须关闭预备 socket 并保留旧配置、监听和会话。相同监听器仅修改目标时换代并取消旧会话，只改名称不得重置连接。数据库与 socket 不能被描述为同一原子事务，崩溃后以已提交配置恢复。

协议切换按集合差量准备：监听地址和端口相同的公共成员复用，新成员先绑定，数据库提交后才停止移除成员及原会话。旧组监控先退休，再让新组接管复用成员；故障协程须核对组身份，防止旧通知停止新组。任一预期成员故障须主动停止整组并标错，不依赖列表轮询。监听循环只发通知，由独立协调协程 Stop，避免等待自身；Service.Close 在锁内停止所有组、解锁后等待监控退出。

发布镜像必须依赖双架构验证成功；main 推送无需版本标签即可发布 `ghcr.io/<小写仓库名>:latest`，程序与 OCI version 使用 `main-<提交SHA前12位>`，不创建 GitHub Release。正式 `vMAJOR.MINOR.PATCH` 标签发布版本镜像和 latest；对应预发布标签只发布版本镜像，不覆盖 latest。版本标签发布才编译发行压缩包并创建 Release，且在镜像发布成功后执行。GHCR job 仅使用 `contents:read/packages:write`，Release job 使用 `contents:write`；认证沿用 GITHUB_TOKEN，action 继续固定 SHA。

## 4. 校验与错误矩阵

| 条件 | 行为 |
| --- | --- |
| 缺失启动凭据、损坏数据库或未来版本数据库 | 拒绝启动，日志不能包含凭据。 |
| 非法 JSON、未知字段、非法地址或端口 | 400，字段错误定位到表单字段。 |
| 未登录或会话过期 | 401。 |
| Origin 或 CSRF 无效 | 403，不执行变更。 |
| 不存在的规则 | 404。 |
| 规则重叠、外部进程占用、低端口权限不足 | 409，原规则保持。 |
| TCP+UDP 与任一协议监听范围重叠 | 409，明确指出 TCP 或 UDP；包含 TCP 时同样检查管理端口冲突。 |
| 组合任一监听启动或运行中失败 | 整组 error，释放另一监听，错误明确协议；其余规则继续。 |
| revision 过期 | 412，不覆盖其他页面的修改。 |
| 登录限流 | 429，不能信任任意 X-Forwarded-For 来分配限额。 |
| 持久化失败 | 500，旧配置和原连接不变。 |
| PR、其他分支推送 | PR 只验证，其他分支无发布入口，不能推镜像或创建 Release。 |
| main/标签验证失败、版本标签格式非法 | 不发布镜像或 Release，非法版本须在登录 GHCR 前拒绝。 |

目标可离线；`running` 只表示监听正常，不能把目标偶发不可达变成本机监听错误。DNS 解析后、拨号前再次检查自身回环及非法目标。

## 5. 正常、基础与异常案例

- 正常：TCP、UDP 可以分别监听同一个数字端口；组合规则也在同一数字端口实际转发两种协议，两个 UDP 客户端的响应不串包。
- 基础：停用规则保留配置，重启仍停用；启用规则重启逐条恢复。
- 异常：一条规则端口被外部占用，只有该规则标错；其他规则正常转发，释放占用后可重试。
- 部署：Linux host 网络、单容器、命名数据卷，删除全部 capability 后仅增加 `NET_BIND_SERVICE`。不能用 privileged 或 `NET_ADMIN` 绕过问题。
- 发布：main 无 tag 仍产生 latest；正式 v1.0.0 产生版本镜像、latest 和 Release；v1.0.0-rc.1 仅更新对应版本镜像并创建预发行版。latest 不是固定正式版本的保证，固定部署须选具体正式标签。

## 6. 必需验证

`internal/rules/service_test.go` 通过故障注入断言提交失败后旧连接仍可收发、预备端口已释放；`internal/httpapi/server_test.go` 验证同 revision 并发更新仅一个成功、CSRF 拒绝和重启后旧会话失效；`internal/forward/engine_test.go` 验证半关闭、UDP 隔离、更新取消与限额回收。

`internal/rules/combined_test.go` 与 `internal/httpapi/combined_test.go` 以真实 socket 验证六方向协议切换、整体 CRUD、占用/存储失败回滚、名称保留会话和目标变更断开；`runtime_group_test.go` 验证无需查询列表即可停止故障对侧以及并发故障/Close 不死锁。`internal/forward/budget_test.go` 验证跨协议共享额度、预备引用与清理；`internal/store/migration_test.go` 验证 v1 全字段和时间文本无损、失败回滚、索引触发器及组合重开恢复。容器 smoke 验证一条组合记录、低端口双协议、整体操作和重建恢复；升级另用真实 v1 数据卷验证。

先构建 web/dist 再 `go vet ./...`、`go test -race ./...`；无 CGO 二进制另行编译。`scripts/docker-smoke.py` 实测低端口、TCP/UDP 及容器重建。不能把跨架构编译或发行版容器运行当成该架构、发行版宿主的实测。

仅修改 Actions 与辅助脚本提示时，执行 actionlint、Bash/Python 语法及真实嵌入元数据脚本的事件矩阵（main、PR、正式/预发布/非法标签）。核对 needs 验证门禁、Release 条件、标签、双架构与 job 权限；不用重跑未变应用的全套编译测试。未推送并观察远端运行前，不能把静态验证描述为 GHCR 已发布。

## 7. 错误与正确写法

错误：HTTP handler 先关闭旧监听，再尝试数据库写入；忽略写入失败后页面显示成功。

正确：handler 只调用 Service；Service 准备监听、提交存储后才改变运行态；失败返回统一 `{error: {code, message, fields?}}`，保留旧状态。

错误：更新只覆盖目标字符串，旧 goroutine 或拨号在函数返回后继续存活。

正确：使用 generation context，取消、关闭并等待旧会话完成；测试挂起 DNS 与并发 Stop。

错误：把组合规则拆成两次独立保存，或先关闭公共 TCP 监听再尝试新增 UDP。

正确：逻辑上只保存一次 `protocol: "tcp+udp"`；复用公共监听并预备新增成员，提交失败只释放新增成员。回退旧程序须恢复升级前完整数据卷备份，不能对 v2 库自动降级。

错误：只为 main 增加一个发布步骤但仍要求合法版本标签，或让 PR 获得发布权限。

正确：按原始 ref 区分 main 与版本标签，main 输出 latest 和提交版本；仅受信任 push 入口调用验证后发布，版本发行步骤以 refs/tags 条件隔离。

所有自写注释、错误文案和内部文档使用中文；GitHub Actions 名称、工作流自有提示及其发布/冒烟脚本可见提示按用户要求使用英文，Go 工具维护的机械标记不翻译。测试临时目录使用 `internal/testutil.TempDir`，工具缓存和 TMPDIR/GOTMPDIR/GOPATH 均位于仓库 `.tmp/`。

## 8. 场景：按规则流量统计与重置

### 8.1 范围与触发条件

修改流量计数、统计开关、后台采样、累计量存储、重置接口或退出行为时适用。统计属于逻辑规则，不属于单个监听器或 generation；组合规则两种传输共享一个累计对象。

### 8.2 签名

- `forward.NewTraffic(enabled, value, now)` 创建共享统计对象；`Add(upload, n)`、`SetEnabled(enabled, now)`、`Sample(now, active)`、`Snapshot()` 维护短锁状态。
- `rules.Service.ResetTraffic(ctx, id, revision) (model.Rule, error)`：服务锁内协调重置和版本校验。
- `Repository.SaveTraffic(ctx, map[string]model.Traffic) error`：批量提交累计量；`ResetTraffic(ctx, model.Rule) error`：同一事务清零并更新 revision。
- `rules.Service.Close() error` 幂等，停止采样、等待转发退出后最终保存，返回保存错误；main 需报告，数据库必须后关闭。
- `POST /api/rules/{id}/traffic/reset`：受 protect、Origin、CSRF、If-Match 保护，成功 200 返回最新 Rule 与 ETag。

### 8.3 请求、响应与存储契约

`traffic_enabled` 新增省略或 null 为 false，修改省略或 null 保留旧值；客户端不能提交 traffic。输出 `traffic` 固定含 `upload_bytes/download_bytes` 非负十进制字符串与 `upload_bytes_per_second/download_bytes_per_second` 有限非负数，速度单位 B/s。累计量使用 int64，达到上限饱和而非溢出。

流量只计成功写出的应用载荷，上行为客户端到目标，下行为目标到客户端；部分写入按实际 n，丢包/失败部分不计数。TCP 持续连接每次成功写出即累计，不等 io.Copy 结束。数据路径不持有服务锁、不访问数据库；改变统计开关或重置不换代、不主动断开连接。

后台每秒采样，每 5 秒批量保存变化的累计值；GET 不推进采样基线。统计关闭、规则停用或监听失败时输出速度为零，累计量保留。改名、目标、监听或协议修改沿用规则 ID 累计，删除时通过外键级联清理 rule_traffic。

重置临时切分统计代，事务期间新流量进入新代；提交失败合并旧累计和期间增量，成功保留新代并重建采样基线。重置成功增加 revision 与 updated_at，周期保存不变更 revision；服务锁串行化批量保存、重置和删除，禁止旧快照覆盖清零或复活删除数据。

保存成功后才推进持久化基线，失败保留待写数据并记录日志，后续重试。正常退出最终刷新；强制终止可能丢失最近未保存增量，持续存储故障扩大窗口，不能承诺断电零丢失。

### 8.4 校验与错误矩阵

| 条件 | 行为 |
| --- | --- |
| 统计字段非布尔或输入中含 traffic | 400，不修改规则。 |
| 未登录、无效 CSRF/Origin | 401/403，不重置。 |
| 不存在、过期版本 | 404/412，不清零。 |
| 清零事务失败 | 500，累计量加期间新增流量完整保留，revision 与开关不变，连接继续。 |
| 定时保存失败 | 保留内存增量，记录错误，后续重试。 |
| 退出最终保存失败 | Close 返回错误并记录，主程序报告失败。 |

### 8.5 正常、基础与异常案例

- 正常：组合规则真实 TCP/UDP 上下行汇总；同一连接上开关暂停/继续，重置后只累计新流量。
- 基础：旧库升级所有规则默认关闭，正常重启恢复累计值；停用或关闭统计后仍能清零已有累计量。
- 异常：清零遇到存储故障保留累计量和并发新增，旧版本重试不能清除其他页面修改后的数据。

### 8.6 必需测试

转发层断言成功/部分写入计数、关闭期间不计数、可控时钟速度与空闲归零、重置失败合并以及高并发；服务层用真实 socket 断言长连接开关/重置不掉线、组合汇总、重置/批量保存/删除次序、最终保存与双 Close；存储验证 v1/v2/v3 升级、默认值、无损字段、累计恢复、清零事务失败回滚和级联删除；API 验证鉴权、字段、ETag、版本竞争和精确字节字符串。

### 8.7 错误与正确写法

错误：用 io.Copy 最终返回量统计长连接；在 GET 时计算并更新速度；重置内存后忽略持久化错误。

正确：每次成功 Write 后累计，独立后台采样；重置事务失败恢复旧统计并合并并发增量，旧周期快照无法回写。
