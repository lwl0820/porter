# 后端与部署契约

## 1. 范围与触发条件

适用于修改 Go 管理接口、规则运行态、SQLite、环境变量和容器集成。实际入口为 `cmd/porter/main.go`，边界模型由 `internal/model/rule.go` 维护；以真实代码和测试为依据。

## 2. 签名

- `config.Load() (Config, error)`：必填管理员环境变量，显式管理 IP 与端口。
- `store.Open(path string) (*Store, error)`：取得 `.lock` 排他锁、迁移后返回，不能多实例共享数据库。
- `rules.Service.Save(ctx, id, revision, input)`：创建时 id 为空，更新要求当前 revision。
- `forward.Engine.Prepare(model.Rule) (*Runtime, error)`：绑定端口但不转发；`Start()`、`Update(rule)`、`Stop()`、`Error()` 分离准备与生效。
- SQL 迁移放在 `internal/store/migrations/` 并内嵌，以 `PRAGMA user_version` 管理。第一版 `rules` 有规则配置、递增 revision、UTC 时间和监听三元组唯一约束。

## 3. 请求、响应与环境契约

规则输入只包含 `name/protocol/listen_host/listen_port/target_host/target_port/enabled`，不得让客户端设置 id、revision、时间或运行态。返回 Rule 包含这些配置及 `id/revision/created_at/updated_at/status/last_error_code?/last_error?`。

`GET /api/rules` 返回 `{rules: Rule[]}`；新增返回 201，修改返回 200，删除完成后返回 204。更新、删除和重试携带 `If-Match: "<revision>"`。登录与当前会话返回 `{username, csrf_token}`，其余修改类请求携带 `X-CSRF-Token` 和同源 Origin。

管理员环境变量 `PORTER_ADMIN_USERNAME`、`PORTER_ADMIN_PASSWORD` 必填；`PORTER_HTTP_ADDR` 默认 `0.0.0.0:8080`，`PORTER_DB_PATH` 默认 `/data/porter.db`。管理监听按 IPv4/IPv6 显式选择 tcp4/tcp6；转发监听同样固定地址族，不能依赖系统双栈默认值。

写操作由 Service 串行化。先准备新监听，再提交数据库，成功后切换运行态；失败必须关闭预备 socket 并保留旧配置、监听和会话。相同监听器仅修改目标时换代并取消旧会话，只改名称不得重置连接。数据库与 socket 不能被描述为同一原子事务，崩溃后以已提交配置恢复。

## 4. 校验与错误矩阵

| 条件 | 行为 |
| --- | --- |
| 缺失启动凭据、损坏数据库或未来版本数据库 | 拒绝启动，日志不能包含凭据。 |
| 非法 JSON、未知字段、非法地址或端口 | 400，字段错误定位到表单字段。 |
| 未登录或会话过期 | 401。 |
| Origin 或 CSRF 无效 | 403，不执行变更。 |
| 不存在的规则 | 404。 |
| 规则重叠、外部进程占用、低端口权限不足 | 409，原规则保持。 |
| revision 过期 | 412，不覆盖其他页面的修改。 |
| 登录限流 | 429，不能信任任意 X-Forwarded-For 来分配限额。 |
| 持久化失败 | 500，旧配置和原连接不变。 |

目标可离线；`running` 只表示监听正常，不能把目标偶发不可达变成本机监听错误。DNS 解析后、拨号前再次检查自身回环及非法目标。

## 5. 正常、基础与异常案例

- 正常：TCP、UDP 可以分别监听同一个数字端口，两个 UDP 客户端的响应不串包。
- 基础：停用规则保留配置，重启仍停用；启用规则重启逐条恢复。
- 异常：一条规则端口被外部占用，只有该规则标错；其他规则正常转发，释放占用后可重试。
- 部署：Linux host 网络、单容器、命名数据卷，删除全部 capability 后仅增加 `NET_BIND_SERVICE`。不能用 privileged 或 `NET_ADMIN` 绕过问题。

## 6. 必需验证

`internal/rules/service_test.go` 通过故障注入断言提交失败后旧连接仍可收发、预备端口已释放；`internal/httpapi/server_test.go` 验证同 revision 并发更新仅一个成功、CSRF 拒绝和重启后旧会话失效；`internal/forward/engine_test.go` 验证半关闭、UDP 隔离、更新取消与限额回收。

先构建 web/dist 再 `go vet ./...`、`go test -race ./...`；无 CGO 二进制另行编译。`scripts/docker-smoke.py` 实测低端口、TCP/UDP 及容器重建。不能把跨架构编译或发行版容器运行当成该架构、发行版宿主的实测。

## 7. 错误与正确写法

错误：HTTP handler 先关闭旧监听，再尝试数据库写入；忽略写入失败后页面显示成功。

正确：handler 只调用 Service；Service 准备监听、提交存储后才改变运行态；失败返回统一 `{error: {code, message, fields?}}`，保留旧状态。

错误：更新只覆盖目标字符串，旧 goroutine 或拨号在函数返回后继续存活。

正确：使用 generation context，取消、关闭并等待旧会话完成；测试挂起 DNS 与并发 Stop。

所有自写注释、错误文案和内部文档使用中文；Go 工具维护的机械标记不翻译。测试临时目录使用 `internal/testutil.TempDir`，工具缓存和 TMPDIR/GOTMPDIR/GOPATH 均位于仓库 `.tmp/`。
