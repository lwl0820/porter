# 原生安装研究

2026-10-18：

- 当前scripts/build-release.sh使用CGO_ENABLED=0构建amd64/arm64二进制，发行包命名与SHA256SUMS可直接复用，无需目标机Go/Node工具链。
- 当前release.yml只在v*标签创建GitHub Release；本任务拟默认选择最新正式Release，并支持明确版本，不额外引入main滚动二进制发布。
- 公开API https://api.github.com/repos/lwl0820/porter/releases/latest 当前返回404，因此下载脚本必须处理尚无可匿名下载Release的情况。
- [GitHub官方Release API](https://docs.github.com/en/rest/releases/releases#get-the-latest-release)用于获取公开发布资产；下载安装使用Release资产，不依赖有时效或需登录的Actions运行附件。
- 程序已有--version、环境变量读取、显式IPv4/IPv6管理监听及SIGTERM优雅退出，适合systemd直接管理。
- 实施前应以systemd官方文档和本机可用的systemd-analyze验证EnvironmentFile转义、专用用户、能力边界及unit语法；暂不宣称已通过真实systemd宿主验证。

## systemd 依据

[systemd v249 官方执行环境文档](https://raw.githubusercontent.com/systemd/systemd/v249/man/systemd.exec.xml)覆盖目标最低版本环境：EnvironmentFile允许用双引号保留首尾空格，文件在服务执行前读取；AmbientCapabilities配合CapabilityBoundingSet使非root进程仅获得绑定低端口能力。服务用户切换不要求将整个服务以root运行。

当前开发环境提供systemd-analyze 262，PID1为codex，不能用本地systemctl成功替代真实宿主验证。可执行unit静态验证和模拟管理命令，真实程序环境变量与HTTP行为可在仓库.tmp独立进程验证。
