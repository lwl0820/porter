# Linux 原生交互式一键部署

## 目标

在 README 提供 curl + bash 入口，下载 GitHub Actions 编译并发布的 Linux 程序，以 systemd 服务运行，用户无需安装 Docker。

## 已确认背景

- 用户明确改为原生二进制配合 systemctl；继续采用交互输入管理员用户名、隐藏密码及确认、完整 Web 监听地址，不自动生成密码。
- 当前 `scripts/build-release.sh` 编译无 CGO 的 Linux amd64/arm64 压缩包，名称为 `porter-${version}-linux-${arch}.tar.gz`，包含独立 porter 程序和 README，配套 SHA256SUMS。
- `.github/workflows/release.yml` 的二进制 Release 目前只在 v* 标签时发布，并依赖镜像发布；main 推送只发布镜像。
- 2026-10-18 公开 latest Release API 查询返回404，不能承诺已有匿名可下载安装包。
- 先前 Docker 安装器及17组测试仅为未提交草稿，后续应替换；原有手工 Docker 部署内容继续保留。

## 需求

1. 一键脚本不安装或调用 Docker，直接下载 Actions 生成的发布压缩包与 SHA256SUMS，校验后安装；目标服务器不编译源码。
2. 覆盖 Ubuntu 22.04/24.04、Debian 12/13、RHEL/Rocky/AlmaLinux 9/10 的 systemd 宿主，架构 amd64/arm64；非 systemd 环境明确报错。
3. 保留终端交互、输入重试、IPv4/IPv6 校验、密码不回显和凭据受限权限；没有终端时首次安装明确报错。
4. 配置 systemd 的自动重启和开机启动，提供 systemctl/journalctl 管理命令；以专用低权限用户运行，只赋予绑定低端口所需能力。
5. 再次执行保留现有凭据、监听设置及数据库，不隐式升级或迁移已有 Docker 数据；失败不覆盖已有可用程序或清空数据。
6. 默认下载最新正式 GitHub Release，允许显式指定版本；没有可下载版本、校验失败、systemd启动失败或健康未就绪时返回非零状态。
7. 在 README 解释二进制发布渠道、初次发布条件、配置与数据路径，以及备份和显式升级方式。

## 验收标准

- curl管道仍能从终端完整交互，特殊字符密码进入实际程序后保持原始值。
- 按机器架构下载正确的压缩包，严格验证对应校验和；损坏或缺失产物不进入服务启动。
- 成功后 porter.service 已启用，进程由专用用户运行，所选监听地址的 /healthz 返回就绪。
- 重复运行保留配置、程序版本及数据库；systemd/下载/解包/权限失败均不假报成功。
- 自动化测试覆盖发行版、架构、交互、产物校验、systemd配置、重复执行和失败恢复；真实发行版宿主未实测范围明确记录。

## 范围外

不修改转发业务、认证及前端，不增加卸载、自动升级或Docker数据自动迁移，不配置防火墙/HTTPS，不部署到用户服务器，不改变现有镜像发布功能。

## 规划状态

用户已明确批准原生方案，2026-10-18 重新进入实施。
