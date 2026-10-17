# 实施计划

## 启动门禁与交付组织

- [x] 用户同意创建任务，并确认拆分为 porterd 与 porter。
- [x] 查明旧 v0.2.0 报错及入口、依赖、发行、安装状态、unit 和 Docker 的受影响链路。
- [x] 重写 PRD/design/本计划，更新 implement/check 真实上下文清单。
- [x] 最新完整方案（包括 CLI 独立配置、首次语言保存、purge 清理及显式 --upgrade 与恢复）已展示，用户明确批准“是，开始实现”，并授权完成后合并主分支、删除开发分支、打标签推送；已执行 task.py start。

本任务作为单个整合交付，不再创建子任务：入口、归档、安装器和旧 unit 必须成套发布，单独交付会造成不可用组合。实施内部分三组责任并写明顺序，不将目录树当作依赖关系。

## 实施顺序与所有权

1. 主会话在批准后启动任务，按 codex/ 前缀创建开发分支，保留初始无关未跟踪文件。派 trellis-implement，提示首行 Active task，按 JSONL 注入规范并读规划；主会话负责规范及最终独立验收。
2. 入口与协议组：cmd/porter、cmd/porterd、internal/cli、controlapi/localapi/native 及相关测试。先完成客户端/服务依赖拆分、版本、裸客户端入口、首次语言选择/配置读写/切换入口、Docker stub；业务服务语义保持。验收依赖边界、无凭据帮助、非法服务参数不启动、真实 socket CRUD。
3. 构建与容器组依赖第2步：build-release.sh、Dockerfile、docker-smoke.py、CI 与相关验证。双架构同版本双程序归档、镜像只有服务、完整 smoke。
4. 安装迁移组依赖第2–3步稳定的双程序归档合同：install.sh、test-install.py。依次默认语言与解包、新schema与旧schema恢复、PATH归属、普通卸载保留CLI配置/purge精确清理、显式升级状态机及故障恢复；测试夹具覆盖两布局，所有系统路径重写到 .tmp。
5. 文档/规范组依赖上述最终行为：双语 README 更新服务/客户端命令、裸入口、PATH、安装器与CLI各自首次菜单/保存优先级/配置位置、旧版辨别、显式迁移/升级/中断恢复/卸载/备份/手工unit/Docker入口；主会话同步 backend contracts 与 cli-contracts。
6. 主会话派独立 trellis-check，覆盖最终全部范围并自行修复发现问题；不得派发同角色递归代理，不覆盖他人改动。执行或保存A1–A10证据，失败按合同修复后定向重测。
7. 主会话独立检查真实双程序、PATH/PTY、旧安装迁移/故障恢复与最终镜像，汇总局限。展示提交批次并遵守本任务提交/远端发布授权，不能把上次标签动作自动套到新的标签名。

## 验证计划

- 静态：Bash语法、Python语法、gofmt、diff-check；go vet、全仓race、porter_docker标签全仓测试、fresh integration。
- 依赖：go list -deps cmd/porter 不含 config/store/rules/forward/httpapi/web；cmd/porterd 与 Docker 构建不含 cli，Docker不编译真实本机监听实现。
- 真进程：两个 --version；CLI help无服务/无凭据；porterd不接受rules；中英文PTY与裸入口/原rules入口、参数CRUD、JSON、取消及EOF；真实root/nonroot、revision、Web同步、TCP/UDP/TCP+UDP、改名保连接与重启恢复。
- CLI偏好：首次回车英文/两语言选择保存、后续裸命令和rules复用、--lang仅单次覆盖、菜单切换持久化、安装器语言不预填CLI；取消/EOF/中断不写，版本不读写，无终端有/无偏好，非root/容器不写，写失败警告、损坏schema/非法值/超大文件/符号链接/属主/权限、原子并发与卸载竞态；仅JSON数据写stdout。
- 发行：amd64/arm64产物、同版本、精确普通成员和SHA256SUMS；缺少/重复/链接/错误版本/错误摘要拒绝。
- 安装：英文默认与显式语言，默认/特殊字符/系统目录，受限PATH和传参，schema1旧版保留/恢复，schema2重跑与链接修复；未知节点/属主/目标/悬空链接、原子创建竞态不覆盖。
- 升级：v0.2.0/v0.3.0式旧包夹具迁移、双程序升级、同版幂等、SemVer预发行和降级拒绝、--upgrade参数组合；凭据/用户/规则/统计保留，正常重跑仍不升级。
- 故障：下载、校验、确认取消、锁后变更、停服、备份中/备份完成、双程序各次rename、unit、链接、健康、状态提交、恢复中的失败与强制中断；验证唯一备份保留、原active/enabled状态与恢复前inactive、完整目录回退。
- 卸载：新旧布局、取消、purge、部分失败续卸载、未知替换和缺库拒绝、保留数据原版恢复；普通卸载保留CLI偏好、purge删除服务/CLI配置与安装记录，取消不清理，未知配置替换拒绝；迁移未恢复时不能直接卸载。
- Docker：最终镜像无porter、服务入口/health/两语言页面/CRUD/配额/真实三协议/重启持久化/正常停止，默认与自定义socket不存在；挂入原生CLI仍拒绝。
- 现有前端代码未改时，不新增前端功能测试；发行构建所需现有前端构建照常执行，CI既有检查不删。

```bash
mkdir -p .tmp/tmp
export TMPDIR="$PWD/.tmp/tmp" GOTMPDIR="$PWD/.tmp/tmp" PYTHONPYCACHEPREFIX="$PWD/.tmp/pycache"
bash -n scripts/install.sh scripts/build-release.sh
python3 scripts/test-install.py
go vet ./...
go test -race ./...
go test -tags porter_docker ./...
go test -count=1 ./tests/integration/...
bash scripts/build-release.sh
python3 scripts/docker-smoke.py porter:split-local
git diff --check
```

真实systemd宿主与arm64运行条件不具备时分开报告；不能用模拟测试宣称完成主机部署。所有临时文件、缓存、证据限仓库 .tmp，测试不得写宿主/usr/bin或创建真实服务/账号。

## 高风险与回退点

重点审查旧porter路径被新客户端替换前必须停服、unit与程序同一稳定布局、真实依赖隔离、状态先于变更、备份完整标记、中断重入、完整数据库目录回退、托管链接归属及卸载停服前预检。测试不能镜像实现而漏掉外部可观察的数据/服务状态。

归档使用 task.py archive --no-commit 再精确暂存本任务移动，日志同样 --no-commit 后只提交本轮相关文件，避开已发现的全archive暂存问题；本任务不修Trellis工具。保持所有无关未跟踪平台、历史任务与日志。

## 完成验证

实现与独立检查已结束，无已定位遗留缺陷。基础54项完整安装回归及最终挂载/提示增量、真实CLI与PATH/PTY/网络/权限、Go race/标签/integration、两架构双程序包和最终Docker smoke均通过。详细hash与测试范围见research/独立验收.md及分组检查记录；不将模拟结果称为真实systemd宿主或arm64运行验证。用户已授权提交合并发布，按research/提交与发布计划.md执行收尾。
