# v0.4.0 发布失败排查与修复

## 证据

- 标签运行：https://github.com/lwl0820/porter/actions/runs/39069719066；arm64 job 117504624222 失败，amd64 成功，镜像和 Release 被依赖门禁跳过。
- 同一提交 `6bcc55723f7537d923c7a6b12ff518ac22196d46` 的 main 运行 39069719056 成功，说明不是稳定编译错误。
- 同一失败 job 的全仓 race 检查已通过（internal/rules 48.641 秒）；报错发生在随后 `go test -tags porter_docker ./...`（internal/rules 14.593 秒），没有数据竞争检测报错。
- 完整原始日志保留在仓库忽略目录 `.tmp/actions-v040-arm64.log`。失败用例 `TestQuotaAccountingAggregatesAcrossRulesAndProtocols/download`，快照 UploadBytes=100、DownloadBytes=110，预期上下行均110。

## 根因与修复边界

`internal/forward/udp.go` 的上传和下行由不同协程转发，成功写出后分别执行 `Meter.Add`。网络响应先到不保证上传记账已完成。原测试只等待下行110，立即断言上行110，存在过早断言。`userMeter` 的 Add 和 Snapshot 有锁保护，当前证据不支持修改生产计量顺序。

修复应等待同一次快照的上下行均达到预期，保留 UsedBytes、规则展示关闭、删规则不退款及最终持久化断言。不得增加固定休眠或删除测试。采用多轮普通、race 和 Docker 标签验证并独立检查。

同文件的周期恢复和周期失败保留在途字节两项测试也在收到 TCP 响应后直接检查双向用量，采用同样的完整快照等待；不改其他具有明确单向目标的等待。

独立检查补齐周期恢复测试的旧周期前置同步：首次健康连接响应之后，必须等待旧周期 UploadBytes/DownloadBytes 均为100，才能推进测试时钟。等待另一用户的规则暂停不构成健康用户已记账的保证，避免延迟 Add 被归入新周期。最终共四处等待条件修改，仍只有一个 Go 测试文件变动。

## 重复测试发现的第二处夹具漏洞

普通 GOMAXPROCS=2 三用例50轮在聚合/both 的规则创建阶段出现一次「目标不能指向规则自身的监听地址」（`.tmp/quota-sync-intermediate-normal-p2-50.log:1577`）。这不是计量断言失败：`availableRange` 只探测后释放端口，而目标端口可能随机落入用户已分配但尚未监听的范围。

修复限定在测试夹具：目标服务器须避开 `q.base..q.base+15` 整个规则范围，直接保持 TCP 随机端口监听并绑定同端口 UDP；冲突则关闭重选，不改生产规则校验。保留失败原始记录并在最终源码上重新验证。

## 发布策略

沿用用户已授权的提交、合并主分支、删除开发分支及打标签推送流程，保留已推送 v0.4.0，发布补丁 v0.4.1。需要实际观察双架构检查、镜像和 GitHub Release 成功，才能报告发布完成。

## 最终本地验收

源码 `internal/rules/quota_runtime_test.go` SHA256：`65957afdffae81b5fca0897698f957e0f60f5b317dd02066fda650204503dedf`，修复提交 `c95a363`。

| 测试 | 条件 | 轮数 | 结果 |
| --- | --- | --- | --- |
| 聚合、周期恢复、周期失败三项 | 普通 GOMAXPROCS=1 | 50 | 通过，118.167 秒 |
| 同上 | 普通 GOMAXPROCS=2 | 50 | 通过，118.362 秒 |
| 同上 | race GOMAXPROCS=2 | 10 | 通过，84.842 秒 |
| 聚合四模式 | porter_docker GOMAXPROCS=2 | 100 | 通过，134.567 秒 |

日志 `.tmp/quota-sync-final-*.log`。中间版本普通 p2 的端口冲突失败日志完整保留，不将其称为通过。中间版本 race50 和最终版本 race50 因验证配置调整被主动中止（exit143），不记为成功或测试失败；最终配置采用 race10，并已有独立最终规则包完整 race 检查。

独立检查的完整分阶段命令、结果与最终哈希见 `actions-v041-check.md`：最终全仓 vet、规则包 fresh race 和 Docker 标签均通过；较早全仓 race/标签及 fresh integration 也通过。最终只改一个 Go 测试文件及知识文档，没有修改生产计量、规则校验或工作流。

合并前远端 main 仍为 `6bcc557`，没有 v0.4.1 标签。后续按授权合并 main、删除本轮开发分支、原子推送 main 与注释标签 v0.4.1；远端运行和发行资产验证由当前会话持续执行，尚未在此文档声称发布完成。
