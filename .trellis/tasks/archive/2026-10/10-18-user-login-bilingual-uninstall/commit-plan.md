# 提交计划

## 工作提交

建议一个逻辑提交：`feat: 默认用户登录并支持双语安装与卸载`

仅包含以下本任务修改：

1. `web/embed.go`：生产根地址进入用户端。
2. `web/embed_test.go`：用户根地址 GET/HEAD 与独立双端回归。
3. `web/src/main.ts`：开发根页加载用户端。
4. `scripts/install.sh`：双语交互、卸载、彻底清理、同版本重装及状态保护。
5. `scripts/test-install.py`：隔离安装、双语、卸载与恢复回归。
6. `README.md`：同步英文部署与卸载说明。
7. `README.zh-CN.md`：同步中文部署与卸载说明。
8. `.trellis/spec/backend/contracts.md`：脚本命令、生命周期、故障和检查契约。
9. `.trellis/spec/frontend/contracts.md`：生产/开发默认用户入口。

## 后续收尾

用户已确认上述提交，并明确要求提交后合并主分支、删除开发分支并推送。先按 Trellis 流程归档本任务及其规划/研究/验证记录，再记录本次会话；归档和会话记录分别生成记账提交，不与工作提交交错。随后同步远端状态，合并至 main，删除本地开发分支，并推送 main；若远端存在本任务开发分支，同步删除。未要求创建标签。

## 排除的已有改动

会话开始时已有 33 项未跟踪顶层路径，均保留，不纳入本次工作提交：

```text
.agents/
.claude/
.codex/
.gitattributes
.opencode/
.trellis/.gitignore
.trellis/.template-hashes.json
.trellis/.version
.trellis/agents/
.trellis/config.yaml
.trellis/scripts/
.trellis/spec/backend/database-guidelines.md
.trellis/spec/backend/directory-structure.md
.trellis/spec/backend/error-handling.md
.trellis/spec/backend/logging-guidelines.md
.trellis/spec/backend/quality-guidelines.md
.trellis/spec/frontend/component-guidelines.md
.trellis/spec/frontend/directory-structure.md
.trellis/spec/frontend/hook-guidelines.md
.trellis/spec/frontend/quality-guidelines.md
.trellis/spec/frontend/state-management.md
.trellis/spec/frontend/type-safety.md
.trellis/spec/guides/
.trellis/tasks/00-bootstrap-guidelines/
.trellis/tasks/10-17-port-forward-panel/
.trellis/tasks/archive/2026-10/10-17-actions-latest/
.trellis/tasks/archive/2026-10/10-17-readme-env-config/
.trellis/tasks/archive/2026-10/10-17-tcp-udp-rule/
.trellis/workflow.md
.trellis/workspace/index.md
.trellis/workspace/lwl0820/index.md
.trellis/workspace/lwl0820/journal-1.md
AGENTS.md
```

`.tmp/` 中的测试日志、缓存与预览数据库为忽略产物，不提交。当前新任务目录由本会话创建，将随任务归档记录，不作为未知改动处理。
