# 提交计划

用户已确认清单，功能已提交为 `2a2b235`；随后明确授权合并主分支、删除开发分支并推送。

## 功能提交

提交信息：`feat: 增加规则流量速度统计与手动重置`

仅纳入以下本会话实现、测试、中文说明和契约变更：

- [.trellis/spec/backend/contracts.md](/home/user/projects/porter/.trellis/spec/backend/contracts.md)
- [.trellis/spec/frontend/contracts.md](/home/user/projects/porter/.trellis/spec/frontend/contracts.md)
- [README.zh-CN.md](/home/user/projects/porter/README.zh-CN.md)
- [cmd/porter/main.go](/home/user/projects/porter/cmd/porter/main.go)
- [internal/forward/engine.go](/home/user/projects/porter/internal/forward/engine.go)
- [internal/forward/tcp.go](/home/user/projects/porter/internal/forward/tcp.go)
- [internal/forward/traffic.go](/home/user/projects/porter/internal/forward/traffic.go)
- [internal/forward/traffic_test.go](/home/user/projects/porter/internal/forward/traffic_test.go)
- [internal/forward/udp.go](/home/user/projects/porter/internal/forward/udp.go)
- [internal/httpapi/server.go](/home/user/projects/porter/internal/httpapi/server.go)
- [internal/httpapi/traffic_test.go](/home/user/projects/porter/internal/httpapi/traffic_test.go)
- [internal/model/rule.go](/home/user/projects/porter/internal/model/rule.go)
- [internal/rules/combined_test.go](/home/user/projects/porter/internal/rules/combined_test.go)
- [internal/rules/runtime_group.go](/home/user/projects/porter/internal/rules/runtime_group.go)
- [internal/rules/runtime_group_test.go](/home/user/projects/porter/internal/rules/runtime_group_test.go)
- [internal/rules/service.go](/home/user/projects/porter/internal/rules/service.go)
- [internal/rules/traffic.go](/home/user/projects/porter/internal/rules/traffic.go)
- [internal/rules/traffic_test.go](/home/user/projects/porter/internal/rules/traffic_test.go)
- [internal/store/migration_test.go](/home/user/projects/porter/internal/store/migration_test.go)
- [internal/store/migrations/003_traffic.sql](/home/user/projects/porter/internal/store/migrations/003_traffic.sql)
- [internal/store/store.go](/home/user/projects/porter/internal/store/store.go)
- [internal/store/traffic.go](/home/user/projects/porter/internal/store/traffic.go)
- [internal/store/traffic_test.go](/home/user/projects/porter/internal/store/traffic_test.go)
- [web/src/App.test.ts](/home/user/projects/porter/web/src/App.test.ts)
- [web/src/App.vue](/home/user/projects/porter/web/src/App.vue)
- [web/src/api.ts](/home/user/projects/porter/web/src/api.ts)
- [web/src/components/RuleEditor.vue](/home/user/projects/porter/web/src/components/RuleEditor.vue)
- [web/src/components/RuleTraffic.vue](/home/user/projects/porter/web/src/components/RuleTraffic.vue)
- [web/src/style.css](/home/user/projects/porter/web/src/style.css)
- [web/src/traffic.test.ts](/home/user/projects/porter/web/src/traffic.test.ts)
- [web/src/traffic.ts](/home/user/projects/porter/web/src/traffic.ts)
- [web/src/types.ts](/home/user/projects/porter/web/src/types.ts)

## 不纳入功能提交的已有未跟踪内容

以下为会话开始前已有的配置、模板或其他任务资料，保持原样；其中 `.trellis/tasks/` 包含本任务规划资料，当前任务将于功能提交后按流程单独归档，其他任务不处理。

- `.agents/`
- `.claude/`
- `.codex/`
- `.gitattributes`
- `.opencode/`
- `.trellis/.gitignore`
- `.trellis/.template-hashes.json`
- `.trellis/.version`
- `.trellis/agents/`
- `.trellis/config.yaml`
- `.trellis/scripts/`
- `.trellis/spec/backend/database-guidelines.md`
- `.trellis/spec/backend/directory-structure.md`
- `.trellis/spec/backend/error-handling.md`
- `.trellis/spec/backend/logging-guidelines.md`
- `.trellis/spec/backend/quality-guidelines.md`
- `.trellis/spec/frontend/component-guidelines.md`
- `.trellis/spec/frontend/directory-structure.md`
- `.trellis/spec/frontend/hook-guidelines.md`
- `.trellis/spec/frontend/quality-guidelines.md`
- `.trellis/spec/frontend/state-management.md`
- `.trellis/spec/frontend/type-safety.md`
- `.trellis/spec/guides/`
- `.trellis/tasks/`
- `.trellis/workflow.md`
- `.trellis/workspace/`
- `AGENTS.md`

## 验证与后续

- 完整验证见 [验证记录](validation.md)。
- 已按确认清单提交功能；不 amend，单独完成本任务归档与会话记录后，按用户后续授权合并 main、删除开发分支并推送。
- 不处理其他未跟踪文件或其他未完成任务。
