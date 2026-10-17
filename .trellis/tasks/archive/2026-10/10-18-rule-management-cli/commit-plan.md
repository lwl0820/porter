# 提交计划

## 批次一

`feat: 增加原生双语规则管理命令行工具`

- cmd/porter/main.go：复用二进制，CLI 分流、本机监听启动及退出。
- internal/cli/：参数和交互两种规则管理方式、双语、终端隔离及测试。
- internal/localapi/：本机控制协议、真实 root 对端授权、路径和版本保护及测试。
- internal/native/：原生运行识别与 Docker 构建禁用及测试。
- internal/rules/accounts.go：沿用授权和服务锁的规则详情查询。
- go.mod：明确本机通道使用的直接依赖。
- Dockerfile、scripts/docker-smoke.py：禁用容器本机管理并验证现有容器功能。
- README.md、README.zh-CN.md：交互/参数使用方法、语言、权限与部署说明。
- .trellis/spec/backend/index.md、cli-contracts.md：记录可执行的 CLI 和本机控制合同。

## 批次二

`docs: 记录规则命令行任务设计与验收`

- .trellis/tasks/10-18-rule-management-cli/：已批准需求、设计、实施计划、上下文、调研、验收和提交计划。

## 排除项

- .tmp/ 内测试程序、产物、日志和缓存：由 .gitignore 排除。
- 开始会话前已存在的 .agents/、.claude/、.codex/、.opencode/、.gitattributes、AGENTS.md、其他未跟踪 Trellis 工具/模板/历史任务/旧日志：保留，不纳入提交。
- 当前任务归档和会话日志在上述工作提交完成后按 finish-work 独立记录，不提前混入。

## 授权状态

用户已明确确认上述两批次本地提交，并授权提交后合并到主分支、删除开发分支、打标签推送。当前远端最新标签为 v0.2.0，本轮使用新增功能版本 v0.3.0。仅提交本任务文件，保留无关文件。
