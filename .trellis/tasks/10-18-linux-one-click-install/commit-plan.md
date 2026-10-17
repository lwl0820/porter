# 原生方案已确认提交计划

实现与独立复核完成：18组原生回归、unit静态检查和真实程序登录验证通过。用户已授权提交、合并main、删除开发分支、打版本标签并推送；本次发布版本采用v0.1.0（项目现有版本0.1.0，远端尚无标签）。

## 提交批次

1. `feat: 增加 Linux 原生交互式一键部署`

   - `scripts/install.sh`
   - `scripts/test-install.py`
   - `README.md`
   - `README.zh-CN.md`
   - `.github/workflows/ci.yml`
   - `.trellis/spec/backend/contracts.md`
   - `.trellis/spec/backend/index.md`
   - `.trellis/tasks/10-18-linux-one-click-install/`

## 已有未跟踪文件（全部排除）

以下路径在本任务开始前已存在，不纳入本次提交：

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
- `.trellis/tasks/00-bootstrap-guidelines/`
- `.trellis/tasks/10-17-port-forward-panel/`
- `.trellis/tasks/archive/2026-10/10-17-actions-latest/`
- `.trellis/tasks/archive/2026-10/10-17-readme-env-config/`
- `.trellis/tasks/archive/2026-10/10-17-tcp-udp-rule/`
- `.trellis/workflow.md`
- `.trellis/workspace/index.md`
- `.trellis/workspace/lwl0820/index.md`
- `.trellis/workspace/lwl0820/journal-1.md`
- `AGENTS.md`

## 已授权收尾

提交以上本任务文件，再归档本任务并记录中文会话日志；归档和日志按项目流程各自产生独立记录。随后合并到main、删除开发分支、创建v0.1.0标签并推送主分支与标签，触发既有Actions发布流程。其他未跟踪文件保持原状。
