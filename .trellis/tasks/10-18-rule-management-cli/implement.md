# 实施计划

## 启动前门禁

- [x] 任务创建和 root/sudo 免登录方案已由用户确认。
- [x] 用户已确认：未指定 --lang 时，首先选择中文或 English。
- [x] 用户已要求：交互菜单与参数两种方式均完整覆盖规则管理。
- [x] 完成现有实现调研、PRD 收敛、技术设计及上下文清单。
- [x] 已展示双模式最终规划摘要，用户回复“是”，批准实施。
- [x] 已执行 task.py start，进入实施；按 Trellis 默认调度实现和检查子代理。

## 实施顺序

1. 阅读 trellis-before-dev、后端契约及任务设计；主会话负责边界和任务记录，实施代理负责产品代码及对应测试，避免覆盖现存未提交文件。
2. 实现原生/Docker 构建边界、运行环境识别，以及受限本机控制服务。覆盖实际对端 UID、socket 权限、路径冲突、失效节点清理及关闭顺序。
3. 接入现有管理员规则服务，补齐 list/get/create/update/delete 本机合同及 revision 校验；核对用户归属、字段和业务错误。
4. 增加共用 CLI 操作层，接入语言引导、参数模式与交互菜单；交互完成创建向导、规则选择、详情、修改、删除确认、返回及退出。共用部分更新、版本校验、双语与渲染；参数模式支持 JSON、完整参数无交互、缺失参数不补问。未指定 --lang 首先选择语言，保留服务启动和版本查询。
5. 接入 main 生命周期与可选控制路径；保证 CLI 不读取 Web 管理密码、不打开数据库、不初始化服务日志。
6. 调整 Dockerfile 禁用构建方式及 smoke 覆盖；原生构建 amd64/arm64，复核安装卸载路径兼容。
7. 更新 README.md 与 README.zh-CN.md，记录交互/参数两种入口、命令语法、提权、首次语言选择、脚本显式 --lang、服务依赖、容器限制、指定归属及版本冲突。内部文档和注释用中文，英文用户界面遵循双语需求。
8. 派发 trellis-check 子代理，完成问题修复和全范围复核；主会话更新实际代码契约、记录验证局限并按项目流程收尾。

## 必须验证的行为

- 真正通过 CLI 进程及本机 socket，以交互和参数两种方式完成 TCP/UDP/TCP+UDP CRUD，Web 查询同步，真实 socket 验证转发与重启恢复。
- 验证实际非 root 对端被拒绝、root 允许、普通请求头不能伪造管理员；测试不得依赖放宽生产授权。
- 默认测试进程若无 root 权限，可用授权判断单测及非 root 拒绝集成测试，并明确 root 正向用例未实测，不能伪造权限测试已通过。
- 新建默认归属管理员，指定已有用户时受端口及配额约束；修改保留归属、未传布尔字段及其他配置。
- 客户端 read-merge-write 的真实并发冲突、显式 revision、不存在 ID、无修改参数、未知字段、非法端口及地址、端口占用、数据库失败回滚。
- 使用伪终端验证未指定 --lang 时首先出现语言选择，中文/English 均可选；无效/空白输入重新提示，取消或输入结束无操作，不复用上次选择或系统语言。
- 两种语言下走完整交互菜单：创建、选择、详情、修改留空保留、删除确认/取消、返回、退出；一个会话只选择一次语言。验证无效序号、空列表、编辑取消、EOF/中断不提交待处理输入，已成功操作不回滚。
- 交互修改/删除在展示后被 Web 修改必须报 revision 冲突；参数模式完整字段和 --lang 全程不提问，缺失字段不进入向导；无终端交互入口、菜单配 --json 均明确拒绝。
- 显式 --lang 的中文/英文及等号语法均跳过选择；未知命令、参数错误、权限拒绝和帮助必须在语言确定后输出。无终端缺少 --lang 双语报错，传入后支持自动化调用。
- 双语帮助/业务失败/服务停止提示；验证 JSON 及管道输出不混入语言提示，stdin 重定向不被选择流程消费，精确字节计数、stdout/stderr 与退出码符合约定。
- 普通文件、符号链接、活动及残留 socket、多实例及退出清理，控制服务与 Web 并发请求和关闭不死锁。
- Docker 标签构建不创建控制入口，即使传入路径也拒绝 CLI；原生二进制在可识别 Docker 环境的禁用路径；正常镜像 Web 管理与真实转发不回归。
- 服务启动和 --version 兼容，默认/自定义原生目录不增加新 unit 或全局链接，现有安装卸载测试通过。

## 验证命令

所有临时数据、编译产物、测试目录和缓存放在仓库 `.tmp/`；Go 测试使用 internal/testutil.TempDir。先确认本机实际工具版本及可用权限。

```bash
mkdir -p .tmp/tmp .tmp/go-cache .tmp/go-path .tmp/go-mod .tmp/npm-cache
export TMPDIR="$PWD/.tmp/tmp" GOTMPDIR="$PWD/.tmp/tmp"
export GOCACHE="$PWD/.tmp/go-cache" GOPATH="$PWD/.tmp/go-path" GOMODCACHE="$PWD/.tmp/go-mod"
export npm_config_cache="$PWD/.tmp/npm-cache"
npm --prefix web run build
go vet ./...
go test -race ./...
go test -count=1 ./tests/integration/...
go test -tags porter_docker ./...
bash -n scripts/install.sh
python3 scripts/test-install.py
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o .tmp/porter-cli-amd64 ./cmd/porter
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o .tmp/porter-cli-arm64 ./cmd/porter
docker build -t porter:cli-local .
python3 scripts/docker-smoke.py porter:cli-local
```

新增本机控制及 CLI 测试包随最终文件位置补充定向命令。只有修改对应文件后才运行相关检查；不为了本任务安装额外前端依赖。Docker 或 root 条件缺失时记录受阻用例及可替代证据，不宣称已实测；不修改宿主真实 Porter 服务。

## 高风险文件及回退点

- cmd/porter/main.go：分流及退出顺序，CLI 不得意外启动服务或记录英文模式下的中文 JSON 日志。
- internal/localapi：授权边界、socket 文件生命周期；不得复用外部 HTTP 身份头。
- internal/rules：仅最小增量复用，不复制或改变提交和运行态切换规则。
- Dockerfile：仅新增禁用构建参数，不移除现有多架构及 capability 限制。
- scripts/install.sh：预计无需改动；如实施发现必须改 unit 或安装文件，先回到规划说明迁移和卸载影响。
- 无 schema 变更；回退以停止服务、恢复原二进制和已确认的本机节点清理为边界。

## 本轮执行结果

- [x] 完成 CLI 参数和交互管理、双语及 JSON/终端处理。
- [x] 完成本机控制协议、真实 root 对端授权、可信路径及 socket 生命周期。
- [x] 复用规则服务的授权、归属、版本保护与持久化回滚。
- [x] 完成 Docker 构建/运行禁用、中英文说明和冒烟覆盖。
- [x] 全范围独立检查完成，补强全部祖先目录可信归属；真实 root/非 root race 套件在可信 chroot 通过。
- [x] 已完成最终原生双架构构建、参数/PTY E2E、安装卸载和容器回归，详见 verification.md。
- [x] 已更新 .trellis/spec/backend/cli-contracts.md 及后端索引。
- [x] 最终验收记录已封版，用户明确确认 commit-plan.md 两批次提交，并授权后续合并、删除分支、打标签推送。
- [ ] 提交完成后归档本任务及记录本会话；其他历史任务不做收尾。
