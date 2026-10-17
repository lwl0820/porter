# 原生安装技术方案

## 修改边界

替换未提交的 Docker 安装器及其测试，更新两份 README、CI检查和部署规范。复用现有无 CGO 二进制构建及发行包格式。复用现有版本标签发布流程，不更改现有镜像功能和发布触发方式。

## 文件与服务

- `bash scripts/install.sh [--install-dir /opt/porter] [--version v1.0.0]`，默认版本选择最新正式Release。
- `/opt/porter/porter`：安装程序，允许通过--install-dir自定义程序安装目录；`/etc/porter/porter.env`：权限600的管理员/监听配置；`/var/lib/porter`：专用服务用户可写的完整数据库目录。
- `/etc/systemd/system/porter.service`：User/Group=porter，EnvironmentFile载入配置，明确PORTER_DB_PATH，Restart=on-failure、WantedBy=multi-user.target。
- 专用系统账号不允许登录，只允许数据目录写入；通过AmbientCapabilities/CapabilityBoundingSet提供CAP_NET_BIND_SERVICE，不以root运行业务。
- 下载暂存位于安装目录下的 `.tmp`；开发验证文件只位于仓库 `.tmp`。

## 流程

1. 校验系统、架构、systemd和权限；必要的curl、CA证书、Python标准库校验依赖使用系统仓库补齐，不安装Docker。
2. 首次通过/dev/tty输入用户名、密码及确认、Web监听地址；沿用先关闭回显再显示密码提示以及退出恢复终端的已验证机制。
3. 查询最新正式Release或明确版本，下载对应Linux压缩包和SHA256SUMS；校验精确文件名和摘要，在暂存目录提取预期的porter普通文件。
4. 在产物验证成功后创建专用用户、数据目录、环境配置与unit；避免对未知已有账号、unit或配置静默覆盖，兼容重复执行。
5. systemctl daemon-reload、enable --now porter；在有限时间内检查unit状态及用户监听地址的健康端点，成功后输出管理入口和systemctl/journalctl命令。
6. 已有有效原生安装复用配置和程序，仅确保服务启动；更换版本由显式升级流程承担。首次中途失败可再次执行恢复，不能把部分配置误认成完整安装。

## 凭据与边界

EnvironmentFile按systemd自身的引号/反斜线语义编码，不经过shell source，不进行Compose转义，密码不作为进程命令行参数。IPv4/IPv6与程序自身契约一致，通配监听使用同族回环探测。数据库保留整个目录，禁止运行中仅复制单个db文件备份。回退旧程序时需恢复匹配的完整数据备份。

## 兼容与发布限制

仅支持以systemd为PID1的目标发行版宿主；模拟测试不替代真实systemd/发行版验证。默认遵循现有正式版本Release发布模式，当前匿名latest查询404，首次正式Release成功公开发布前一键下载会清楚报错；不在本任务擅自增加main滚动发布渠道。
