# v0.4.1 配额测试同步修复独立检查

## 范围与结论

本次仅检查 v0.4.0 发布失败的后续差异：`internal/rules/quota_runtime_test.go`、后端合同 9.6 节及 `actions-v040-followup.md`。原任务恢复活动目录是归档状态调整；已交付 CLI、安装器、前端和发布工作流没有产品代码变化，不重复执行无关完整安装回归。

结论：检查通过。确认原失败属于异步测试过早读取；另补齐一处推进测试时钟前的旧周期入账等待，并检查了压力测试暴露的目标端口夹具修复。最终修改仅一个 Go 测试文件，保留全部业务断言、原有三秒轮询超时和生产逻辑。

## 已修复发现

- 文件：`internal/rules/quota_runtime_test.go`，`TestQuotaCycleRestoresOnlyPausedRulesAndDoesNotDropHealthyConnections`。
- 问题：健康连接首次 `sendTCP` 返回后，原测试没有确认旧周期双向记账即在后续推进可控时钟。等待另一用户的规则因耗尽暂停，不保证健康用户的计量协程已经执行 `Add`；延迟计量可能进入新周期，导致新增的新周期精确字节断言偶发失败。
- 修复：在首次 `sendTCP` 后使用既有 `eventuallyQuota` 等待同一快照上下行均为 100，再继续后续流程。未增加固定休眠、超时、生产钩子或新的公共接口。

## 根因与其他差异核对

1. `internal/forward/udp.go` 的上传和回复由不同协程执行，成功 `Write`/`WriteToUDP` 后才分别调用 `Meter.Add`。目标收到上传并返回、客户端收到回复时，上传协程仍可能尚未执行自己的记账。失败日志中的上传 100、下行 110 与缺少 UDP 上传 10 的过早快照一致。
2. TCP 的 `trafficWriter.Write` 同样在底层 `Write` 后调用 `Meter.Add`；客户端收到回复不保证回复计量已经完成。
3. `userMeter.Add` 与 `Snapshot` 使用同一互斥锁，`Add` 在锁内同时更新方向字节与 `UsedBytes`；未发现需要修改生产计量逻辑的数据竞争证据。
4. 聚合测试现在等待同一快照上下行均为 110，仍独立检查四种计费结果、关闭展示统计、不退款及退出持久化。另两项周期测试等待新周期双向预期后仍执行原断言。
5. 检查了同文件其余快照条件：具有明确单向目的、终止转发后保存或仅验证周期身份的等待，不因本修复机械扩展。
6. 后端合同 9.6 节已记录完整快照条件及禁止用固定延时或单纯重跑掩盖失败；排查文档正确区分已通过的 race 与随后失败的 Docker 标签测试。发布成功仍须由主会话观察远端结果。

## 目标端口夹具追加检查

实施者的中间源码普通 GOMAXPROCS=2 压力测试在发流量前，于 `q.rule` 创建阶段出现一次「目标不能指向规则自身的监听地址」。独立复核完整日志 `.tmp/quota-sync-intermediate-normal-p2-50.log:1577`，确认与计量断言失败不同。

最终夹具改为 `q.targetPair`：直接持有 `127.0.0.1:0` 的 TCP 监听，排除 `q.base..q.base+15` 完整规则端口段，再绑定同端口 UDP；失败分支关闭本轮 TCP 后重选，最多 200 轮。成功后两种监听均由原有 cleanup 关闭，返回实际监听端口。已搜索核对全部九处调用迁移，规则校验与生产实现没有变化。后端合同和排查文档已同步这项限制。

## 验证

命令统一使用仓库内 `TMPDIR=.tmp/tmp`、`GOTMPDIR=.tmp/tmp`、`GOCACHE=.tmp/go-cache`、`GOPATH=.tmp/go-path`、`GOMODCACHE=.tmp/go-mod`。

| 命令 | 结果 | 证据 |
| --- | --- | --- |
| `go vet ./...` | 通过；补齐前置等待后再次通过 | `.tmp/actions-v041-check-vet.log`、`.tmp/actions-v041-check-vet-final.log` |
| `go test -race ./...` | 通过；规则包 53.947 秒，其余包可能复用既有缓存 | `.tmp/actions-v041-check-race.log` |
| `go test -tags porter_docker ./...` | 通过；规则包 23.869 秒，其余包可能复用既有缓存 | `.tmp/actions-v041-check-docker-tags.log` |
| `go test -count=1 ./tests/integration/...` | 通过，0.027 秒 | `.tmp/actions-v041-check-integration.log` |
| `go test -race -count=1 ./internal/rules` | 第一阶段源码通过，44.656 秒 | `.tmp/actions-v041-check-rules-race-final.log` |
| `go test -tags porter_docker -count=1 ./internal/rules` | 第一阶段源码通过，22.569 秒 | `.tmp/actions-v041-check-rules-tags-final.log` |
| `gofmt -l internal/rules/quota_runtime_test.go` | 通过，无输出 | 本次终端检查 |
| `git diff --check` | 通过 | 本次终端检查 |

表中名称带 `final` 的第一阶段规则包检查针对补齐旧周期等待、尚未修复目标端口夹具的源码 SHA256 `edfd39d7d42cd03a2b90c160df7ba0df4d4c7ad964a9a9ac954f77ed02943307`。首轮全仓测试在该前置等待补齐前启动。没有把分阶段结果标为最终哈希的一次性全仓无缓存运行。

目标端口夹具修复后，再对唯一受影响的规则包执行 fresh race 与 Docker 标签测试，并重新执行全仓 vet；以下记录最终源码结果。Go 测试编译与 vet 均通过，类型检查通过；本轮不涉及 TypeScript。

| 命令 | 结果 | 证据 |
| --- | --- | --- |
| `go vet ./...` | 通过，退出 0 | `.tmp/actions-v041-check-vet-final2.log` |
| `go test -race -count=1 ./internal/rules` | 通过，52.653 秒，退出 0 | `.tmp/actions-v041-check-rules-race-final2.log` |
| `go test -tags porter_docker -count=1 ./internal/rules` | 通过，23.364 秒，退出 0 | `.tmp/actions-v041-check-rules-tags-final2.log` |
| `gofmt -l internal/rules/quota_runtime_test.go`、`git diff --check` | 通过 | 本次终端检查 |

最终测试源码 SHA256：`65957afdffae81b5fca0897698f957e0f60f5b317dd02066fda650204503dedf`。

## 未修复发现与边界

无已定位且未修复的本次范围内缺陷。当前独立验证为本机 amd64；未将其视为 arm64 实机或远端发布成功。主会话负责继续观察 v0.4.1 双架构 Actions、镜像与 Release 状态。没有提交 Git 或更改已有标签。
