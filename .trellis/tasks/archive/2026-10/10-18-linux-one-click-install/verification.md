# 原生安装验证记录

## 当前方案

用户已批准 GitHub Release 二进制下载 + systemd；旧 Docker 安装器的17组测试与容器验证不作为本方案通过证据。

## 主会话实际验证

- 使用当前源码及 `CGO_ENABLED=0 go build` 构建 `.tmp/native-install-check/porter`，通过。
- 使用安装器实际 `env-write` 生成凭据，输入包含美元符号、连续美元符号、反引号、单双引号、反斜线、井号、中文和首尾空格。按官方文档一致的POSIX双引号规则读取可信合成测试文件（仅QA，生产禁止source），实际原生程序 `/healthz` 与 `/api/admin/auth/login` 通过，密码保持原始值；进程已SIGTERM清理。
- 使用安装器实际 `unit` 生成普通路径和含空格、美元符号、百分号的路径，`systemd-analyze verify` 全部退出0且stderr为空。
- 初步验证曾仅检查unit退出码，漏看EnvironmentFile被静默忽略的警告。实施者修正了该指令写法、ExecStart路径编码，主会话已按上述更严格方式复验，规范已记录该陷阱。

## 实施与独立复核

- trellis-implement完成原生实现及首轮15组测试；trellis-check独立全范围复核后，最终18组测试全部通过，耗时86.539秒。
- 覆盖目标发行版与架构矩阵、最新正式/指定版本、模拟发行包下载、摘要/包结构/程序版本验证、特殊字符密码、EOF/INT回显恢复、缺失依赖、sudo、未知资源保护、重复安装、局部失败恢复及健康失败。
- 独立复核补充配置/unit半写失败后的原子重试、完整安装数据目录或数据库丢失时拒绝创建空库、两级锁持有者保护，以及HTTP就绪探测期间服务退出的拒绝成功回归。
- Bash语法与帮助、Python AST、`.tmp/bin/actionlint .github/workflows/ci.yml`、`git diff --check`全部通过。
- 最终实际生成的普通和空格/$/%路径unit均通过systemd-analyze verify，退出0且stderr为空。
- README原生兼容说明、备份700目录/600归档和部署规范均已同步。独立复核无未修问题或阻断项。

## 实测边界

- 开发环境systemd-analyze为262，PID1为codex，无法在本环境宣称真实systemd已启动服务、切换专用用户或应用低端口能力。
- 发行版/架构、软件包依赖和systemctl命令使用隔离替身验证；不得创建开发宿主全局账号、unit或修改现有业务数据。
- 尚未在各个Ubuntu/Debian/RHEL/Rocky/AlmaLinux真实宿主执行完整安装。全部开发临时文件位于仓库.tmp。
- 当前仅本地分支，未推送/发布。公开latest Release API当前返回404，需要存在公开的Actions发布资产后才能真实下载。README raw main入口也需代码合入远端main。
