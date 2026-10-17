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
- SQL 迁移放在 `internal/store/migrations/` 并内嵌，以 `PRAGMA user_version` 管理。保留 `001_init.sql`；`002_tcp_udp.sql` 将协议 CHECK 扩展为三个值；`003_traffic.sql` 增加默认关闭的统计开关和累计表，当前版本为 4，`004_accounts.sql` 增加用户、套餐、系统端口池、用户独占端口及当前周期用量，规则增加 owner_id。v1→v2 事务复制全部字段并保留自定义索引和触发器；未知列、外部外键依赖或迁移失败须拒绝并回滚，不能静默丢弃。v2→v3 增量 ALTER，不重建 rules 表。
- `.github/workflows/ci.yml`：PR 与 `workflow_call` 验证入口；`release.yml`：main push 和 v* tag push 的验证及发布入口。同一次 main 推送只执行一套验证。

## 3. 请求、响应与环境契约

规则输入只包含 `name/protocol/listen_host/listen_port/target_host/target_port/enabled/traffic_enabled?`，不得让客户端设置 id、revision、时间或统计快照。返回 Rule 包含这些配置及 `id/revision/created_at/updated_at/status/last_error_code?/last_error?/traffic/owner_id`，管理端另含权威 `owner:{id,username,role}`，统计契约见下方场景。

`protocol` 只接受 `tcp`、`udp`、`tcp+udp`。组合规则使用一条记录、一个 id/revision、同一监听 IP/数字端口及同一目标主机/数字端口；启停、重试和删除整体执行。现有两条单协议规则不自动合并。组合内两个 Runtime 按同一非空 Rule.ID 共享单规则会话预算；预备实例也持有预算引用，最后实例停止且会话释放后才回收预算。

`GET /api/admin/rules` 和 `/api/user/rules` 返回 `{rules: Rule[]}`；管理端支持 `owner_id` 查询。新增返回 201，修改返回 200，删除完成后返回 204。更新、删除和重试携带 `If-Match: "<revision>"`。两端分别使用自己的 `auth/login|session|logout`，响应包含 `{user_id, username, role, csrf_token}`，其余修改请求携带本端 `X-CSRF-Token` 和同源 Origin。旧共享 `/api/auth/*`、`/api/rules*` 业务路由返回 404，不能作为兼容别名。

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
- `POST /api/admin/rules/{id}/traffic/reset` 和 `/api/user/rules/{id}/traffic/reset`：受 protect、Origin、CSRF、If-Match 保护，成功 200 返回最新 Rule 与 ETag。

### 8.3 请求、响应与存储契约

`traffic_enabled` 新增省略或 null 为 false，修改省略或 null 保留旧值；客户端不能提交 traffic。输出 `traffic` 固定含 `upload_bytes/download_bytes` 非负十进制字符串与 `upload_bytes_per_second/download_bytes_per_second` 有限非负数，速度单位 B/s。累计量使用 int64，达到上限饱和而非溢出。

流量只计成功写出的应用载荷，上行为客户端到目标，下行为目标到客户端；部分写入按实际 n，丢包/失败部分不计数。TCP 持续连接每次成功写出即累计，不等 io.Copy 结束。数据路径不持有服务锁、不访问数据库；改变统计开关或重置不换代、不主动断开连接。

后台每秒采样，默认每 5 秒批量保存变化的累计值，保存间隔由 `PORTER_TRAFFIC_FLUSH_INTERVAL` 控制（默认 `5s`，Go duration，`1ms`–`24h`）；GET 不推进采样基线。统计关闭、规则停用或监听失败时输出速度为零，累计量保留。改名、目标、监听或协议修改沿用规则 ID 累计，删除时通过外键级联清理 rule_traffic。

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


## 9. 场景：双端多用户、端口分配及套餐配额

### 9.1 范围与触发条件

修改认证、账号/套餐、端口分配、规则归属、配额数据路径、周期或保存配置时适用。本场景的用户配额与第 8 节可关闭、可清零的规则展示统计是两个独立账本。

### 9.2 签名与职责

- model.User、Plan、Settings、Usage、Account 定义跨层资源；Rule.OwnerID 是服务端归属，AdminRuleCreateInput 才允许管理员创建时指定 owner_id，更新不转移归属。
- Store.EnsureAdmin(ctx,username) 维护固定 system-admin 身份；CreateUser(ctx,user,usage,managementPort) 同事务创建账号、连续端口分配和当前周期。
- auth.Manager.SetUsers 注入普通用户凭据查询；LoginPortal、GetPortal、LogoutPortal 同时约束端和角色，HashPassword/VerifyPassword 使用带盐 PBKDF2-SHA256 编码，普通用户派生结果不得输出。
- rules.Options{FlushInterval,Now} 注入保存间隔与可控时钟；SaveFor、DeleteFor、RetryFor、ResetTrafficFor 先授权再校验规则版本，ListFor 管理端可指定归属筛选。
- rules.userMeter 为每用户跨规则、协议和连接的共享计量；forward.Meter.Wait(ctx)/Add(upload,n) 在成功写出点执行，独立协调过程停止用户运行组，数据协程不得同步 Stop 自身。
- quota.Cycle(plan,user,now) 使用 UTC 和原始日号计算周期；Used(mode,U,D) 实现四种计费，SaturatingAdd 保护整数字节上限。

### 9.3 数据、HTTP 与环境契约

管理页面 /admin/，用户页面 /user/，根地址跳转管理端；独立 /api/admin/* 和 /api/user/*，共用同一进程、SQLite 与底层服务。

Cookie 为 porter_admin_session（Path=/api/admin）和 porter_user_session（Path=/api/user），CSRF、登录和退出独立。服务端校验会话所属端及角色，复制 Cookie 到另一端同样无效。双端同时登录不覆盖，登出一端不撤销另一端；全局登录限流共用。

管理端 rules 默认全部，owner_id 按稳定 ID 过滤，响应 owner 来自用户表，包含管理员自己。管理员无需用户会话即可完成任意规则操作，但遵守所属用户端口和额度；用户端强制本人，不能通过归属参数筛选他人或指定他人 owner。

管理 plans、users 提供 GET 和 POST，settings 提供 GET 和携带 If-Match 的 PUT；用户 account 提供 GET。列表分别为 {plans:Plan[]}、{users:Account[]}，Account 为 {user,plan,usage}。尚未配置的 Settings 两个端口为 0、revision 为 1；更新必须为有效起止范围。

套餐字段包括 name、port_count、traffic_limit_bytes（正整数字节字符串）、accounting_mode=both|max|upload|download、reset_mode=calendar_month|anniversary|fixed_day，以及仅 fixed_day 可设置的 reset_day=1–31。用户创建只接受 username/password/plan_id；密码不回传。

用户创建由服务操作锁和数据库唯一性共同保护，分配连续数字段，排除所有已有分配、管理员规则端口及管理端口；TCP/UDP 不重复计数，不同 IP 也不能跨用户复用数字。修改池只影响后续用户，不能回收存量分配。管理监听配置与已有分配冲突时拒绝启动。

用户周期持久化独立的 U、D，按成功转发的应用载荷计量。both=U+D，max=max(U,D)，upload=U，download=D，先按全用户汇总再计算。显示关闭、清零、改删规则均不退款。字节响应为十进制字符串，含 cycle_start/end、upload/download/used/limit_bytes 和 exceeded；不得经浮点转换。

周期统一 UTC 00:00；缺失日期取月末且保留原始日号，首次开户完整额度，未用不结转，停机跨周期只进入当前周期。周期推进先提交新周期，再放行新额度，不清零展示账本；活动连接不因正常清零强制断开。周期键入库统一为 UTC 固定九位纳秒文本再比较，不能用 SQLite julianday 的毫秒精度替代，以免误判重置点前一纳秒的首周期。

额度耗尽立即标记并通知独立协调者，停止全部所属监听和已有会话，不依赖 GET、页面轮询或保存时钟。enabled 表示配置意图，quota_exceeded 是运行状态；手动停用及删除不能被周期恢复复活。并发在途写入允许小量超额，不能声称逐字节限速器。

PORTER_TRAFFIC_FLUSH_INTERVAL 默认 5s，Go duration 范围 1ms–24h，空值使用默认、非法值拒绝启动；它控制两个账本保存，不改变每秒采样、实时断流、UTC 边界或前端五秒刷新。正常退出停止转发后最终保存。强制退出可丢未保存增量，连续存储失败扩大丢失窗口，必须日志报告并保留内存重试。

### 9.4 校验与错误矩阵

| 条件 | 行为 |
| --- | --- |
| 本端无有效会话、另一端 token 或角色不符 | 拒绝接口访问，不执行管理操作。 |
| 普通用户访问他人规则 | 404，先授权后验证 revision，避免泄露他人版本。 |
| 普通用户输入 owner_id、更新转移归属、非法枚举或固定日组合 | 400，状态不变。 |
| 重名、无足够连续端口、管理员侵占已分配端口 | 409，创建用户事务整体回滚。 |
| 监听端口超出所属范围 | 字段校验失败，管理端代操作同样拒绝。 |
| 耗尽时启用、重试或保存启用规则 | 409 quota_exceeded；停用、删除及展示清零仍可用。 |
| 周期重置事务失败 | 不发放新额度，暂缓该用户新写入并重试，其他用户独立。启动时单用户重置失败同样隔离，规则显示 error + quota_reset_pending，提交成功后自动恢复仍启用的这类规则；真正 listen_failed 不自动复活。 |
| 定时用量保存失败 | 不推进保存成功基线，保持内存计数并重试；正常退出最终失败向上返回。 |
| 旧周期快照或重复周期转换 | 版本/周期键校验，不覆盖当前周期或重复清零。 |

### 9.5 正常、基础与异常案例

- 正常：管理端查看用户归属并按用户过滤，代操作后用户端刷新获得新 revision；本人规则不泄露其他账号。
- 基础：开户 31 日遇平年二月按 28 日 UTC 零点重置，三月回到 31 日；10 个数字端口供用户 TCP/UDP 共用。
- 异常：重置存储被阻塞期间的在途新周期字节不能被提交覆盖；旧周期通知不得停止已恢复的新周期运行组。

### 9.6 必需测试

覆盖双端 Cookie/CSRF/登出隔离及角色拒绝、管理员本人和两个用户的归属与过滤、所有代操作和越权 ID、并发端口分配与失败回滚、所有旧库迁移、四种模式（特别是两规则反向流量的全用户 max）、UTC 平闰年月末及回锚、周期事务失败和在途累计、实时主动断流与另一用户隔离、保存间隔及最终保存错误。真实 socket 与 race 检查不能由仅调用计量对象替代。

### 9.7 错误与正确写法

错误：用可清零的 rule_traffic 汇总用户配额；对每条规则分别取 max 再相加；handler 只按前端角色显隐控制权限；为停止转发从写入协程同步 Stop 并等待自身。

正确：持久化独立用户周期 U、D，共享计量后按套餐公式计算；两端接口使用独立会话守卫且服务层执行归属和配额检查；数据路径仅标记并发出通知，由独立服务协调过程关闭整个用户的运行组。

## 10. 场景：Linux 原生交互式一键部署

### 10.1 范围与触发条件

修改 `scripts/install.sh`、README 一键入口、安装器验证、GitHub 发行包或 systemd 集成时适用。安装器下载原生程序，不安装或调用 Docker。现有手工 Docker 部署与镜像发布保持独立。

### 10.2 命令签名

`bash scripts/install.sh [--install-dir /opt/porter] [--version v1.0.0]`；`--help` 只输出帮助。远程入口为 `curl -fsSL https://raw.githubusercontent.com/lwl0820/porter/main/scripts/install.sh | bash`，以 root 或通过 sudo 执行宿主写操作。默认选择最新正式 GitHub Release，显式版本允许已发布预发行版。

### 10.3 输入、文件及服务契约

- 首次通过 `/dev/tty` 输入管理员用户名、隐藏密码、密码确认、完整 Web 监听地址，不从脚本管道读取，不生成默认密码。先关闭终端回显再显示密码提示，EOF、INT、TERM 和正常退出均恢复回显。
- 用户名去除首尾空白，密码保留原始值但不能全为空白；监听为显式 IPv4:端口或 [IPv6]:端口，范围 1–65535。通配监听通过同族回环地址检查健康，禁止固定使用 8080。
- 目标 Ubuntu 22.04/24.04、Debian 12/13、RHEL/Rocky/AlmaLinux 9/10，架构 amd64/arm64；要求 systemd 为 PID 1。必要 Python 3、curl 和 CA 依赖通过系统 apt/dnf 补齐，不安装编译工具或 Docker，不自动卸载软件包。
- GitHub API 获取正式 latest 或指定 tag；精确选择 `porter-${version}-linux-${arch}.tar.gz` 与 `SHA256SUMS`。校验唯一的对应 SHA256 项，仅读取压缩包中名为 `porter` 的唯一普通文件，不抽取任意路径/链接；执行暂存程序 `--version` 核对发布版本。
- 默认程序 `/opt/porter/porter`（目录可指定）、配置 `/etc/porter/porter.env`、数据 `/var/lib/porter`、unit `/etc/systemd/system/porter.service`。配置与安装状态权限 600，数据目录权限 700，程序权限 755。
- 环境文件逐项使用双引号，编码反斜线、双引号、反引号和美元符号；生产只通过标准输入和受限解析读取，禁止 shell source，禁止 Compose 的 `$$` 替换。文件内的 `PORTER_DB_PATH` 固定为 `/var/lib/porter/porter.db`。
- `porter.service` 使用专用禁止登录的系统用户/组 porter、`Restart=on-failure`、开机自启。`AmbientCapabilities` 和 `CapabilityBoundingSet` 仅提供 `CAP_NET_BIND_SERVICE`，配合 `NoNewPrivileges`、`ProtectSystem=strict`、`StateDirectory=porter` 允许完整数据库目录写入。
- unit 各字段按其自身语法编码，不能把 shell 引号规则泛用于所有 systemd 指令。固定 `EnvironmentFile=/etc/porter/porter.env` 不包外层引号，否则该指令可能被静默忽略；ExecStart 使用冒号前缀禁用参数变量展开，路径中的百分号仍需写为 `%%`。安装目录的控制字符、引号和反斜线提前拒绝，空格、美元符号和百分号须通过实际 unit 验证。检查 FragmentPath，不能用新 unit 静默覆盖 vendor 目录中的同名服务。
- `/etc/porter/install.json` 保存 schema=1、directory、arch、version、status（initializing/complete）；安装目录和全局配置目录加锁，只清理本进程成功获取的锁，不能删除其他安装进程持有的锁；下载暂存只用安装目录 `.tmp`。状态、凭据和 unit 的最终写入均先在目标目录创建受限 `.new` 文件，再原子重命名；不能在最终路径留下半写文件，使初始化状态无法恢复。首次校验产物后才记录初始化状态，重复执行复用原配置、程序和数据库，不隐式升级；有初始化状态才能安全恢复局部安装。
- 已有完整安装缺失关键文件、数据目录或 `porter.db` 时拒绝继续，不能自动创建空库冒充恢复；目录/架构不匹配、未知账号/配置/unit/数据库或符号链接等也明确拒绝接管。已托管用户校验 home、nologin 与主组，不能把未知普通用户变为服务用户。
- 成功需要 unit 已启用且运行，所选地址 `/healthz` 返回 `{"status":"ok"}`，HTTP 检查后再次确认 unit 仍在运行。输出实际配置、数据和程序位置及 systemctl/journalctl 命令，失败不得输出就绪。

### 10.4 校验与错误矩阵

| 条件 | 行为 |
| --- | --- |
| 不支持系统/架构、非 systemd 宿主、非法参数或不能提权 | 非零退出，不部署服务。 |
| 首次无控制终端 | 明确报错，不读取脚本内容作为用户输入。 |
| 空白用户名/密码、密码确认不一致、无效 IP/端口 | 重试输入，不写入无效配置。 |
| latest 不存在、API/网络失败、缺架构产物或校验和 | 非零退出，不声称已有远端发行版。 |
| 摘要不符、重复校验项、目录/链接代替程序、版本不匹配 | 不安装暂存程序，不覆盖既有可用版本。 |
| 未托管文件/账号/unit、安装状态不一致或路径冲突 | 保留原状，提示人工处理，不能清理数据。 |
| systemd 启动/启用或健康检查失败 | 非零退出，保留初始化状态与配置，允许后续重试。 |

### 10.5 正常、基础与异常案例

- 正常：交互输入包含 `$`、反引号、引号、反斜线、中文和首尾空格的密码，最终程序凭据保持原值；IPv6 通配监听使用 `[::1]` 探测。
- 基础：再次执行不下载新版本、不重置配置；第一次启动失败后复用原始版本及凭据重试。数据库保存整个目录，不能复制运行中的单个 db 文件作为完整备份；备份包含凭据，目录权限700、归档权限600。
- 异常：未知 vendor unit、服务账号、损坏压缩包或不匹配校验和不能被安装器“修复”为新部署；回退旧程序需要匹配的完整数据备份。

### 10.6 必需验证

执行 Bash 语法、帮助、`python3 scripts/test-install.py`、actionlint 和 `git diff --check`。测试通过伪终端、模拟发布包和命令替身覆盖发行版/架构、sudo、输入校验、隐藏/恢复回显、摘要校验、原生服务配置、重复执行及失败恢复。使用可用 `systemd-analyze verify` 校验实际生成 unit，除退出码外还须检查 stderr 的忽略/无效指令警告，不能把退出0当作配置均已生效；独立启动当前二进制，验证生成配置的凭据及健康/登录行为。

开发检查不能创建真实全局账号/服务或使用业务数据库；临时文件仅项目 `.tmp`。开发环境 PID1 不是 systemd 时，明确区分 unit 静态验证、实际程序验证和模拟管理命令，不能声称已经验证真实 systemd 用户切换、能力边界及全部发行版宿主安装。原 Docker 安装器测试结果不能用作原生方案的通过证据。

### 10.7 错误与正确写法

错误：下载 Actions 临时附件或在目标机编译；把 EnvironmentFile 当 Compose 文件转义；先覆盖程序再验证校验和；从标准输入读取密码或给未知 porter.service 写入覆盖文件。

正确：复用公开 Release 资产和 SHA256SUMS，先暂存校验，再创建原生受限服务；从独立终端交互，密码按 systemd EnvironmentFile 语义编码；用安装状态和实际 FragmentPath 判断能否恢复已有部署。
