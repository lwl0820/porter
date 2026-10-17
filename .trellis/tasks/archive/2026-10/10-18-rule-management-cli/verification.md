# 验证记录

## 实施边界与初始环境

- 已获用户对双模式最终方案的明确实施批准；任务状态为 in_progress，分支为 codex/rule-management-cli。
- 当前行为缺口为原生程序缺少本机规则 CLI；实现覆盖交互/参数入口、本机控制服务、服务查询及生命周期、Docker 禁用、测试与用户文档。
- 业务行为归属既有 rules.Service，CLI 不直写数据库，不改变 Web 登录、数据库 schema、安装 unit，不提供用户/套餐管理。
- 工作区存在其他会话留下的大量未跟踪 Trellis/平台文件，保留原样，不纳入本任务提交。
- Go：go1.27.2-X:nodwarf5 linux/amd64；Node.js：v26.11.1（与 README 固定构建版本不同，实际结果以该版本记录）。
- Docker Engine：29.9.0，可正常查询；本机 UID 为 1000。
- sudo 因 no-new-privileges 不可用；已发现并采用 `unshare --user --map-root-user` 在隔离用户命名空间中完成 UID 0 对端及原生程序正向验证，不修改宿主权限。该结果不能写成真实 systemd 宿主安装实测。
- 所有测试目录、脚本、缓存、编译结果与日志位于仓库 .tmp。

## 验证分工

- 实现代理：新增功能的单元/集成测试及项目 Go 检查，报告实际命令结果。
- 主会话：独立复核产品入口、容器构建与真实转发 smoke、权限正反例及文档与合同一致性。
- 检查代理：实现完成后按 check.jsonl 对全范围代码、测试、双语和跨层合同进行复核及局部修复。

## 验收映射

| 验收 | 重点证据 | 状态 |
| --- | --- | --- |
| A1 | 可信chroot实际 UID 0/1000 对端、伪造身份拒绝、Web 路由隔离 | 通过 |
| A2 | 双模式三协议 CRUD、Web 同步、真实转发 | 通过 |
| A3 | root 本机通道测试已有用户归属、端口约束，既有配额测试及 Docker 真实耗尽 | 通过 |
| A4 | root 本机通道 revision 并发、注入持久化失败；独立程序端口/版本冲突与旧连接保持 | 通过 |
| A5 | CLI 双语和结构化输出测试、真实 PTY 语言选择、独立 JSON 解析 | 通过 |
| A6 | 真实进程停机拒绝操作、帮助/版本无服务执行 | 通过 |
| A7 | porter_docker 标签测试、实际镜像 CLI 拒绝和双控制路径不存在、原生二进制放入 Docker 拒绝 | 通过 |
| A8 | 参数端到端重启恢复、安装卸载 33 项、amd64/arm64 编译；真实 systemd 宿主及 arm64 运行未实测 | 已验证范围通过 |
| A9 | 入口合同测试无用户/套餐动作，独立长连接改名不断、停用/删除释放监听 | 通过 |
| A10 | 无终端报错、取消/EOF 保护、JSON 和管道隔离测试，真实 PTY 无效选择重试 | 通过 |
| A11 | 中文 TCP、英文 UDP、显式英文 TCP+UDP 真菜单增删改查/取消；单测展示版本冲突和参数不补问 | 通过 |

## 已完成的独立验证

- `TMPDIR="$PWD/.tmp/tmp" python3 scripts/test-install.py`：33 项安装/卸载检查通过，157.652 秒；日志 `.tmp/cli-install-tests.log`。
- 原生 `CGO_ENABLED=0` 的 Linux amd64、arm64 编译通过，产物 `.tmp/porter-cli-amd64`、`.tmp/porter-cli-arm64`。
- `unshare --user --map-root-user python3 .tmp/cli-native-e2e.py`：以隔离用户命名空间 UID 0 运行真实服务/CLI，三协议参数 CRUD、Web 同步、实际转发、只改名保留连接、revision/端口冲突、启停、正常退出清理、重启恢复及无终端语言要求通过。
- `unshare --user --map-root-user python3 .tmp/cli-menu-e2e.py`：真实伪终端完成中文 TCP、英文 UDP、显式英文 TCP+UDP 的交互 CRUD；验证无效语言选择重试、菜单内不重复选语言、编辑取消、删除确认/取消。
- 当前 UID 1000 调用真实 CLI 返回 root_required，帮助和 --version 可独立执行；英文 JSON 标准错误可解析。
- Docker 固定 Node/Go 镜像 digest 已获取。当前 Docker CLI 缺 Buildx，原 Dockerfile 使用自动 BUILDPLATFORM 导致 legacy builder 无法直接解析；仅在 `.tmp/cli-Dockerfile.verify` 最前补 `ARG BUILDPLATFORM=linux/amd64`，其余逐字使用仓库 Dockerfile，单架构镜像 `porter:cli-local` 构建成功。没有修改产品 Dockerfile 以兼容本机旧构建器。

- `TMPDIR="$PWD/.tmp/tmp" python3 scripts/docker-smoke.py porter:cli-local`：容器 CLI 及默认/自定义控制节点禁用、Web 鉴权、低端口、单协议和组合实际流量、重建恢复、协议切换、双端用户隔离、归属筛选、持久化用量与真实额度耗尽均通过。
- 将未启用 porter_docker 标签的原生二进制挂入容器，`rules list --lang en --json` 返回 native_only / 退出码 1，证明运行环境识别路径同样生效。

## 独立检查与最终补丁

- 实现代理已通过 `go vet ./...`、`go test -race ./...`、`go test -tags porter_docker ./...`；并以 UID 0 定向执行 CLI、本机通道及运行识别的 race 测试。
- 独立检查发现路径 owner 即使当前目录只读也能自行 chmod，现已将父目录与全部祖先统一要求属于 root/服务 UID，继续拒绝组/其他用户可写和符号链接；没有加入未映射 UID 或测试环境放行。
- 新增可信目录 metadata 矩阵；原环境不可信祖先下的真实 socket 测试会明确跳过，另在可信 chroot 中用 root 与非 root 身份执行实际 socket/race，日志 `.tmp/cli-check-root-chroot-race.log`、`.tmp/cli-check-nonroot-chroot-race.log`。
- 最终黑盒测试采用服务置于仓库 `.tmp/cli-native-rootfs` 可信 chroot、CLI 在同一隔离 UID 0 命名空间中正常运行，通过真实 Unix 通道连接。参数与 PTY 两套测试重跑通过，日志 `.tmp/cli-native-e2e-final.log`、`.tmp/cli-menu-e2e-final.log`。不存在测试授权绕过。
- 先前尝试在工作区 chroot 内运行 CLI 的 PTY 时，工作区 nodev 限制使其无法打开 /dev/tty；已通过让真实 CLI 使用原有控制终端、仅隔离服务目录解决。该失败属于测试环境，不计为产品失败或通过证据。
- `go test -count=1 ./tests/integration/...` 新鲜执行通过；日志 `.tmp/cli-integration-fresh.log`。

## 最终构建与质量结论

- 最终源码 Linux amd64/arm64 的 `CGO_ENABLED=0` 静态编译通过；不将 arm64 编译当作 arm64 运行实测。
- 最终二进制下两套黑盒脚本再次通过：三协议参数与真实转发、双语真实 PTY 菜单、Web 同步、版本冲突、取消/确认、服务退出及重启恢复。
- 最终源码镜像 `porter:cli-local` 重建成功（amd64，镜像短 ID `835c0f17b92e`），仍使用前述 legacy builder 的单一 ARG 补充验证副本；完整 Docker smoke 再次通过。日志 `.tmp/cli-docker-build-final.log`、`.tmp/cli-docker-smoke-final.log`。
- 独立 trellis-check 结论为无遗留产品问题；Go vet/race/Docker 标签全套、独立 integration、前端 lint/type-check、格式与脚本语法检查均通过。实际 root/nonroot 的可信 chroot race 补齐普通工作区因安全目录归属而明确跳过的覆盖。
- `git diff --check`、任务上下文引用校验通过。未更改数据库 schema、原生安装 unit 或现有 Web 页面。
- 质量门禁已通过。用户已确认两批次工作提交及后续主分支合并、删除开发分支、打标签推送；功能提交为 c9a37ba。任务设计与验收提交、归档和会话日志按既定顺序继续执行，完整文件分组见 commit-plan.md。

## 验证边界

这些结果覆盖当前 Linux amd64 隔离环境、真实用户命名空间对端、真实 socket/PTY 与 Docker Engine；不代表真实 systemd 用户切换、完整发行版原生安装或 arm64 宿主运行已实测。当前 Docker CLI 无 Buildx，未宣称按原 Dockerfile 完成 BuildKit 多架构构建；原 Dockerfile 及其固定镜像定义保持兼容要求。
