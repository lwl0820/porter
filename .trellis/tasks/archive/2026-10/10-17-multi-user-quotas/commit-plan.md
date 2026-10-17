# 已确认的提交计划

## 提交一

消息：`feat: 支持独立双端、多用户套餐与流量配额`

目标：以一份完整功能提交交付双端与用户套餐资源约束，包含实现、测试、部署文档和同步契约。用户已明确确认提交，并要求合并到主分支、删除开发分支后推送。

本会话及子代理修改的文件如下：

- `.trellis/spec/backend/contracts.md`
- `.trellis/spec/frontend/contracts.md`
- `.trellis/tasks/10-17-multi-user-quotas/check.jsonl`
- `.trellis/tasks/10-17-multi-user-quotas/commit-plan.md`
- `.trellis/tasks/10-17-multi-user-quotas/design.md`
- `.trellis/tasks/10-17-multi-user-quotas/implement.jsonl`
- `.trellis/tasks/10-17-multi-user-quotas/implement.md`
- `.trellis/tasks/10-17-multi-user-quotas/prd.md`
- `.trellis/tasks/10-17-multi-user-quotas/research/实施接口契约.md`
- `.trellis/tasks/10-17-multi-user-quotas/research/现有实现与跨层约束.md`
- `.trellis/tasks/10-17-multi-user-quotas/research/验收结果.md`
- `.trellis/tasks/10-17-multi-user-quotas/task.json`
- `README.md`
- `README.zh-CN.md`
- `cmd/porter/main.go`
- `compose.yaml`
- `internal/auth/auth.go`
- `internal/auth/password.go`
- `internal/auth/portal_test.go`
- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/forward/engine.go`
- `internal/forward/tcp.go`
- `internal/forward/traffic.go`
- `internal/forward/traffic_test.go`
- `internal/forward/udp.go`
- `internal/httpapi/accounts.go`
- `internal/httpapi/accounts_test.go`
- `internal/httpapi/combined_test.go`
- `internal/httpapi/server.go`
- `internal/httpapi/server_test.go`
- `internal/httpapi/traffic_test.go`
- `internal/model/account.go`
- `internal/model/rule.go`
- `internal/quota/policy.go`
- `internal/quota/policy_test.go`
- `internal/rules/accounts.go`
- `internal/rules/quota.go`
- `internal/rules/quota_runtime_test.go`
- `internal/rules/runtime_group.go`
- `internal/rules/service.go`
- `internal/rules/traffic.go`
- `internal/store/accounts.go`
- `internal/store/accounts_test.go`
- `internal/store/migration_test.go`
- `internal/store/migrations/004_accounts.sql`
- `internal/store/store.go`
- `internal/store/traffic_test.go`
- `scripts/docker-smoke.py`
- `web/admin/index.html`
- `web/embed.go`
- `web/embed_test.go`
- `web/src/AdminApp.vue`
- `web/src/App.test.ts`
- `web/src/App.vue`
- `web/src/UserApp.vue`
- `web/src/accounts.test.ts`
- `web/src/admin.ts`
- `web/src/api.ts`
- `web/src/components/AccountOverview.vue`
- `web/src/components/AdminResources.vue`
- `web/src/components/RuleEditor.vue`
- `web/src/i18n.ts`
- `web/src/main.ts`
- `web/src/style.css`
- `web/src/traffic.test.ts`
- `web/src/traffic.ts`
- `web/src/types.ts`
- `web/src/user.ts`
- `web/user/index.html`
- `web/vite.config.ts`

## 不纳入提交的既有文件

以下仅指列出目录内会话开始前已存在的初始化模板、其他任务和日志；前一节明确列出的本任务规范文件仍纳入。其余不清理、不覆盖、不提交：

- `.agents/skills/`
- `.claude/agents/`
- `.claude/commands/`
- `.claude/hooks/`
- `.claude/settings.json`
- `.claude/skills/`
- `.codex/agents/`
- `.codex/config.toml`
- `.codex/hooks.json`
- `.codex/hooks/`
- `.gitattributes`
- `.opencode/agents/`
- `.opencode/commands/`
- `.opencode/lib/`
- `.opencode/package.json`
- `.opencode/plugins/`
- `.opencode/skills/`
- `.trellis/.gitignore`
- `.trellis/.template-hashes.json`
- `.trellis/.version`
- `.trellis/agents/`
- `.trellis/config.yaml`
- `.trellis/scripts/`
- `.trellis/spec/backend/`
- `.trellis/spec/frontend/`
- `.trellis/spec/guides/`
- `.trellis/tasks/00-bootstrap-guidelines/`
- `.trellis/tasks/10-17-port-forward-panel/`
- `.trellis/tasks/archive/`
- `.trellis/workflow.md`
- `.trellis/workspace/`
- `.trellis/workspace/lwl0820/`
- `AGENTS.md`

## 后续顺序

用户已批准：提交功能 → 使用 Trellis 归档本任务 → 记录本会话开发日志 → 同步远端并合并到 main → 删除开发分支 → 推送 main。归档和日志按工作流单独提交，不与功能提交交叉；推送主分支将触发现有 CI 验证与镜像发布。

所有检查通过，用户已明确确认本计划及主分支合并、开发分支删除和推送。
