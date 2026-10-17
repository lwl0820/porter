# 实施与验收计划

## 进入实施的门禁

- 用户已确认需求选择：默认双入口、至少启用一项、安装后可切换。
- prd/design/implement和implement.jsonl/check.jsonl齐全，完成需求收敛和PRD通读。
- 最终方案已展示，用户明确批准实施并授权提交、合并、删除开发分支及tag推送；进入实施。
- 批准后创建codex/前缀开发分支；保留初始无关未跟踪平台配置与历史任务，不纳入提交。

## 1. 服务与配置

负责文件：internal/config、cmd/porterd、必要的internal/rules及其测试；复用store.GetUser与固定管理员，除非发现实证需要不改业务库。

1. 增加Web/CLI组件布尔配置，条件校验Web字段、拒绝双关；Docker/native实际能力由启动预检校验。
2. 添加--check-config无副作用入口与参数解析测试；版本/帮助仍不依赖服务。
3. 将规则服务管理地址缺省改为明确无Web监听，不保留虚构端口；测试CLI-only下原8080可作规则端口、Web开启时仍拒绝冲突。
4. 启动/关闭按组件拆分，CLI-only不改管理员名、不建HTTP，Web-only不建控制socket；保留错误清理、信号退出、最终持久化顺序。
5. 测试配置矩阵、旧env缺省、非法布尔、忽略禁用Web字段、实际三模式和不支持环境拒绝。

服务实施后的集成补齐：`internal/cli/preferences_linux.go` 的stateAllowsPreferences只认识schema1/2，需要兼容schema3的严格features且仅cli=true、status=complete才允许保存语言（旧schema1/2保留complete/initializing）；对应测试覆盖首次/切换/状态变化，在真实命名空间root race与带schema3的PTY中验证。此为保证既有语言功能在新安装记录下有效的必要修改，不引入服务依赖。

## 2. 安装、组件切换与恢复

负责文件：scripts/install.sh、scripts/test-install.py；先与服务实现统一环境键、预检命令及空管理地址语义。

1. 增加双语逐项选择和默认值，全关重选；只在Web需要时收集凭据。新装/已有/卸载/升级流程明确区分。
2. schema3严格features状态与schema1/2兼容解释；程序清单、PATH归属、完整性检查按组件处理，环境配置与记录一致。
3. 安装/重装/升级采用组件匹配的就绪检查，Unix只读查询不经过CLI语言引导，不污染/创建偏好。
4. 普通重跑支持显式改组合，六方向均支持，同版本补装客户端；不变选择保持幂等。候选旧版不支持组件预检则停服前拒绝。
5. 扩展既有持久事务操作种类及原/目标组件，复用备份/回退和残锁恢复，覆盖env、客户端/PATH、本机管理及原服务意图。
6. 卸载、purge、原版本重装与中断恢复识别schema3及缺省旧模式，保持缺库/未知归属/链接拒绝和历史备份边界。
7. 新增三模式、六方向切换、启用Web配置复用/输入、关闭CLI偏好保留、未知PATH保护、取消与预检失败零提交的回归。
8. 在停服、备份中/完成、env替换、程序增删、链接变更、就绪、状态提交及回退阶段注入失败和强制中断，验证恢复字节/owner/mode及active/enabled意图。

## 3. 文档与规范

更新README.md及README.zh-CN.md：选项默认、三模式、CLI-only无Web输入、安装后重跑切换、首次开启Web、重启影响、旧版本先升级、普通卸载/purge及备份。原英文README作为产品双语例外，其余内部文档和注释中文。

更新backend/contracts、cli-contracts、native-install-contracts对应签名/配置/状态/错误矩阵与必测断言；若接近上下文长度上限独立拆出组件模式合同并更新index及JSONL。不改Trellis运行时。

## 4. 实施协作边界

批准后按Trellis分工派实施子代理，服务/安装器可按上述所有权并行，设计合同先统一；文档在最终行为稳定后同步。提醒各代理不是独自工作，不回退他人改动，不自行提交；主会话负责集成、规范与收尾。最后独立trellis-check全范围检查，不只检查最后一块修改。

## 5. 验证命令与实测

所有工具缓存及TMPDIR/GOTMPDIR/GOPATH/GOMODCACHE放仓库.tmp，Go测试临时目录复用testutil.TempDir。

- Bash语法与双语help、Python安装回归、actionlint（若工作流完全未变也核对发行流程未受破坏）、git diff --check。
- web/dist存在后go vet ./...、go test -race ./...、go test -tags porter_docker ./...、fresh tests/integration。
- scripts/check-build-boundaries.py，CGO_ENABLED=0 amd64/arm64 porter及porterd构建与发行包检查。
- 真实root/nonroot socket授权、三模式真实进程、CLI交互与参数CRUD、三协议转发、双入口一致、Web-only无socket与客户端、CLI-only无Web监听且Web端口占用无影响。
- 六方向切换前后规则ID/统计/归属/偏好、重启恢复及按模式就绪；测试旧版不能静默忽略关闭Web。
- 完整scripts/test-install.py及新故障矩阵，不以mock代替实际网络/程序；systemd-analyze verify与Docker完整smoke。
- 明确区分命名空间root与真实宿主systemd、交叉编译与arm64实机，报告最终源码hash和实际执行结果。

## 6. 完成与发布

独立检查无遗留缺陷、规范同步后按用户会话既有授权处理提交/合并/删除开发分支/标签推送。发布版本在完工时核对远端标签决定，既有标签不移动、不强推。若执行发布，持续观察双架构Actions、镜像及Release实际成功，再核对安装包版本和校验和，不把推送标签等同发布成功。
