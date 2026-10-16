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
- SQL 迁移放在 `internal/store/migrations/` 并内嵌，以 `PRAGMA user_version` 管理。保留 `001_init.sql`；`002_tcp_udp.sql` 将协议 CHECK 扩展为三个值，当前版本为 2。v1→v2 事务复制全部字段并保留自定义索引和触发器；未知列、外部外键依赖或迁移失败须拒绝并回滚，不能静默丢弃。

## 3. 请求、响应与环境契约

规则输入只包含 `name/protocol/listen_host/listen_port/target_host/target_port/enabled`，不得让客户端设置 id、revision、时间或运行态。返回 Rule 包含这些配置及 `id/revision/created_at/updated_at/status/last_error_code?/last_error?`。

`protocol` 只接受 `tcp`、`udp`、`tcp+udp`。组合规则使用一条记录、一个 id/revision、同一监听 IP/数字端口及同一目标主机/数字端口；启停、重试和删除整体执行。现有两条单协议规则不自动合并。组合内两个 Runtime 按同一非空 Rule.ID 共享单规则会话预算；预备实例也持有预算引用，最后实例停止且会话释放后才回收预算。

`GET /api/rules` 返回 `{rules: Rule[]}`；新增返回 201，修改返回 200，删除完成后返回 204。更新、删除和重试携带 `If-Match: "<revision>"`。登录与当前会话返回 `{username, csrf_token}`，其余修改类请求携带 `X-CSRF-Token` 和同源 Origin。

管理员环境变量 `PORTER_ADMIN_USERNAME`、`PORTER_ADMIN_PASSWORD` 必填；`PORTER_HTTP_ADDR` 默认 `0.0.0.0:8080`，`PORTER_DB_PATH` 默认 `/data/porter.db`。管理监听按 IPv4/IPv6 显式选择 tcp4/tcp6；转发监听同样固定地址族，不能依赖系统双栈默认值。

写操作由 Service 串行化。先准备新监听，再提交数据库，成功后切换运行态；失败必须关闭预备 socket 并保留旧配置、监听和会话。相同监听器仅修改目标时换代并取消旧会话，只改名称不得重置连接。数据库与 socket 不能被描述为同一原子事务，崩溃后以已提交配置恢复。

协议切换按集合差量准备：监听地址和端口相同的公共成员复用，新成员先绑定，数据库提交后才停止移除成员及原会话。旧组监控先退休，再让新组接管复用成员；故障协程须核对组身份，防止旧通知停止新组。任一预期成员故障须主动停止整组并标错，不依赖列表轮询。监听循环只发通知，由独立协调协程 Stop，避免等待自身；Service.Close 在锁内停止所有组、解锁后等待监控退出。

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

目标可离线；`running` 只表示监听正常，不能把目标偶发不可达变成本机监听错误。DNS 解析后、拨号前再次检查自身回环及非法目标。

## 5. 正常、基础与异常案例

- 正常：TCP、UDP 可以分别监听同一个数字端口；组合规则也在同一数字端口实际转发两种协议，两个 UDP 客户端的响应不串包。
- 基础：停用规则保留配置，重启仍停用；启用规则重启逐条恢复。
- 异常：一条规则端口被外部占用，只有该规则标错；其他规则正常转发，释放占用后可重试。
- 部署：Linux host 网络、单容器、命名数据卷，删除全部 capability 后仅增加 `NET_BIND_SERVICE`。不能用 privileged 或 `NET_ADMIN` 绕过问题。

## 6. 必需验证

`internal/rules/service_test.go` 通过故障注入断言提交失败后旧连接仍可收发、预备端口已释放；`internal/httpapi/server_test.go` 验证同 revision 并发更新仅一个成功、CSRF 拒绝和重启后旧会话失效；`internal/forward/engine_test.go` 验证半关闭、UDP 隔离、更新取消与限额回收。

`internal/rules/combined_test.go` 与 `internal/httpapi/combined_test.go` 以真实 socket 验证六方向协议切换、整体 CRUD、占用/存储失败回滚、名称保留会话和目标变更断开；`runtime_group_test.go` 验证无需查询列表即可停止故障对侧以及并发故障/Close 不死锁。`internal/forward/budget_test.go` 验证跨协议共享额度、预备引用与清理；`internal/store/migration_test.go` 验证 v1 全字段和时间文本无损、失败回滚、索引触发器及组合重开恢复。容器 smoke 验证一条组合记录、低端口双协议、整体操作和重建恢复；升级另用真实 v1 数据卷验证。

先构建 web/dist 再 `go vet ./...`、`go test -race ./...`；无 CGO 二进制另行编译。`scripts/docker-smoke.py` 实测低端口、TCP/UDP 及容器重建。不能把跨架构编译或发行版容器运行当成该架构、发行版宿主的实测。

## 7. 错误与正确写法

错误：HTTP handler 先关闭旧监听，再尝试数据库写入；忽略写入失败后页面显示成功。

正确：handler 只调用 Service；Service 准备监听、提交存储后才改变运行态；失败返回统一 `{error: {code, message, fields?}}`，保留旧状态。

错误：更新只覆盖目标字符串，旧 goroutine 或拨号在函数返回后继续存活。

正确：使用 generation context，取消、关闭并等待旧会话完成；测试挂起 DNS 与并发 Stop。

错误：把组合规则拆成两次独立保存，或先关闭公共 TCP 监听再尝试新增 UDP。

正确：逻辑上只保存一次 `protocol: "tcp+udp"`；复用公共监听并预备新增成员，提交失败只释放新增成员。回退旧程序须恢复升级前完整数据卷备份，不能对 v2 库自动降级。

所有自写注释、错误文案和内部文档使用中文；Go 工具维护的机械标记不翻译。测试临时目录使用 `internal/testutil.TempDir`，工具缓存和 TMPDIR/GOTMPDIR/GOPATH 均位于仓库 `.tmp/`。
