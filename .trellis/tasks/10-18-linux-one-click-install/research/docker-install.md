> 状态：此为已弃用Docker安装器方案的历史研究；当前原生方案使用native-install.md，不按本文实施。

# Docker 官方安装依据

查证日期：2026-10-18。仅作为安装器实现依据，不代表实际宿主安装已验证。

- [Ubuntu 官方安装说明](https://docs.docker.com/engine/install/ubuntu/)：目标 22.04/24.04 仍受支持。使用 Docker apt 源与独立 keyring；安装 docker-ce、docker-ce-cli、containerd.io、docker-compose-plugin（buildx 可选于本任务，无构建需求）。
- [Debian 官方安装说明](https://docs.docker.com/engine/install/debian/)：12/13 受支持，安装流程同 apt，发行代号分别 bookworm/trixie。
- [RHEL 官方安装说明](https://docs.docker.com/engine/install/rhel/)：9/10 支持；dnf-plugins-core 提供 config-manager，仓库 https://download.docker.com/linux/rhel/docker-ce.repo；需要显式 systemctl enable --now docker。
- [CentOS 官方安装说明](https://docs.docker.com/engine/install/centos/)：提供 https://download.docker.com/linux/centos/docker-ce.repo；Rocky/Alma 的兼容映射属于本项目兼容目标，不是 Docker 官方对其宿主的测试承诺。

官方文档要求先处理冲突包。本任务禁止自动卸载，故先检测并报错，不能使用 apt 自动移除、dnf --allowerasing 等隐式破坏性处理。已有可用 Docker/Compose 则复用，不擅自迁移依赖来源。

安装器要防止外部 DOCKER_HOST/DOCKER_CONTEXT 指向远端而意外部署；使用明确本机 daemon，并拒绝不符合目标的 rootless/非 Linux Engine。无需在开发宿主上实际安装软件包。

独立复核发现：仅配置 Engine 缺失分支的官方源，会使“已有发行版 Engine、缺 Compose 且无 Docker 源”的插件安装失败。决定在 Engine 或插件任一缺失时配置/复用官方源，但已有 Engine 时仅安装插件，不替换 Engine。2026-10-18 查证 [Debian trixie 官方包索引](https://download.docker.com/linux/debian/dists/trixie/stable/binary-amd64/Packages)：当前 docker-compose-plugin 没有 Depends，只有 buildx 推荐依赖；apt 使用 `--no-install-recommends` 防止额外推荐链引入不需要的软件。
