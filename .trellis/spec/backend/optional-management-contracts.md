# Web 与命令行组件选择合同

## 1. 范围与触发

适用于原生安装的三种管理入口组合、配置加载、服务启动、按模式就绪、组件切换、安装记录、升级回退及卸载。相关底层规则/配额仍遵守 [后端合同](contracts.md)，授权与语言偏好遵守 [CLI合同](cli-contracts.md)，路径、锁、备份和归属遵守 [原生安装合同](native-install-contracts.md)。本合同覆盖后两者中“无条件启用Web/部署双程序”的旧假设。

## 2. 签名与职责

- `PORTER_WEB_ENABLED`、`PORTER_CLI_ENABLED` 为服务组件开关，原生缺省均true；CLI配置不能解除native/Docker构建与运行环境限制。
- `porterd --check-config` 只读取并校验配置及当前环境有效管理能力，不开库、不监听、不生成语言偏好；成功0，失败非零，不输出密码。`--version`、帮助继续独立于服务配置。安装器发现预检能力时须检查成功的 `--help` 进程stdout与stderr；Go flag帮助实际写stderr，不能仅凭空stdout误判旧版。
- `rules.New(..., adminAddr, ...)` 接受空adminAddr表示没有Web管理监听；非空仍校验实际地址。不能用127.0.0.1:0之类虚构监听代替。
- `porter` 仍为独立客户端，不读取服务配置或业务数据库；root调用既有本机HTTP over Unix协议。
- `bash scripts/install.sh` 新安装提供两项组件启用选择；已有安装可显式修改选择，默认保留当前组合。`--upgrade`保持组合，`--uninstall [--purge]`不进入组件选择向导。

## 3. 配置、状态与生命周期

### 3.1 三种组合

| Web | CLI | 服务端行为 | 本机安装文件与就绪 |
| --- | --- | --- | --- |
| true | true | Web TCP与原生Unix本机控制共用规则服务 | porterd+porter及托管PATH；Web与本机查询都成功 |
| true | false | 只有Web管理；无本机socket | 只有porterd，无CLI托管入口；Web就绪 |
| false | true | 只有Unix本机管理；无WebTCP/页面/API/HTTP健康入口 | porterd+porter及托管PATH；只检查Unix查询 |
| false | false | 配置拒绝 | 不执行切换/安装提交 |

后台转发始终由porterd运行。Docker/不支持原生控制的环境实际CLI不可用，即使配置true也不能生成socket；若再关闭Web则明确拒绝无有效管理方式运行。Docker默认继续Web服务。

Web启用才要求并校验 `PORTER_ADMIN_USERNAME`、`PORTER_ADMIN_PASSWORD`、`PORTER_HTTP_ADDR`、`PORTER_COOKIE_SECURE` 等Web专用设置。关闭时不因其缺失/无效阻止CLI，新CLI-only环境文件不要求写这些键；共享数据库路径、转发限制与保存间隔仍校验。

Web启用保持EnsureAdmin的显式名称配置；关闭时读取迁移已有固定AdminID和角色，不改名、不创建替代身份或Web密码。新库迁移产生的空管理员名称可以保留，不能以此为由要求Web用户名或更改规则owner。

无Web时不保留管理端口，原HTTP默认端口可用来建转发规则。启Web仍传真实已绑定地址作管理端口冲突检查。退出应先关闭管理请求并等待在途操作，再最终保存规则服务，最后关库；部分启动失败不能漏关闭已取得监听。

### 3.2 schema3与归属

新记录为schema3、layout=split，严格记录 `features:{web:bool,cli:bool}`，必须至少true一项；保留directory/arch/version/status与受限command_link。bool必须实际JSON布尔值，不能将数字/字符串当布尔。env和state的组件设置须一致，不能从文件缺失推测已禁用功能。

- porterd始终必需；porter仅CLI启用时必需。CLI启用且入口由安装器管理时才允许command_link；程序目录直接/usr/bin不产生自链接。
- 仅Web新装不能接管或删除无关/usr/bin/porter，也不因该无关命令拒绝安装。启用CLI才预检命令冲突，关闭CLI只删确认托管的客户端与链接。未知替换在停服前拒绝。
- `/etc/porter/cli.json` 随禁用CLI保留，开启后复用；不被安装器选语言预填。普通卸载保留，purge精确清理。CLI偏好保存的安装状态守卫须兼容schema3，仅严格合法features且cli=true、status=complete时允许写入；旧schema1/2继续兼容complete/initializing；Web-only或不稳定状态拒绝写偏好，锁内快照重校仍适用。
- 关闭Web保留已有合法配置供重开复用；首次开启无完整合法配置才交互收集用户名、隐藏密码及确认、监听地址，不能回显或记录密码。

schema2旧split映射双入口；schema1旧legacy映射原Web入口。旧schema原版本重试/恢复与卸载不能强要求新能力。显式升级到支持组件的版本后保持原有效组合，旧legacy不能直接用CLI覆盖服务路径。普通重跑不隐式升级，补装CLI仅取记录同版本包；目标不支持新能力须在停服前明确拒绝，不能相信旧程序会识别环境开关。

### 3.3 组件切换与恢复

三种组合均支持切换到另外两种，已安装向导默认沿用当前组合。无变化不得反复停服或取latest；取消/EOF/中断输入不得提交模式改变。切换前展示原/目标选择、备份与重启影响。旧停止/禁用实例的active/enabled意图保留，不因验证而永久启动。

复用现有升级持久事务，区分upgrade/reconfigure，保留受限upgrade_id、阶段、原/目标组件快照。必须兼容旧六键transaction记录（id/phase/active/enabled/cli/new_link），其缺少operation/features时按可信旧upgrade恢复，不能因增加字段拒绝旧恢复。

顺序为候选预检/归属检查/确认 → 双锁重校 → 可恢复prepared → 停服 → 完整备份 → backup_complete → 切换配置/所选文件/入口 → 按模式启动与就绪 → 恢复服务意图 → complete原子提交。

备份包含原记录精确字节、程序、unit、服务及CLI配置、完整数据目录与owner/mode。升级与reconfigure都须在取得双锁并check_managed后显式执行data-check，提供具体双语挂载诊断，事务入口仍保留check_data_mount兜底；普通幂等重跑不因该变更预检而被阻断。数据目录挂载根保护、残锁保守处理、阶段重入、禁止半备份恢复和禁止新旧数据合并沿用原合同。失败需停止候选服务并恢复原组合/程序/入口/配置/数据及服务意图。历史备份与失败数据副本保留及purge提示不变。

CLI-only重开Web必须在停服前检查目标TCP地址/端口与已有启用TCP/组合规则的重叠，按原生地址Unmap后同族且相等或一方通配的规则判定。仅依赖重启后healthz不够：HTTP先占端口时，规则恢复可能只标listen_failed而进程仍健康。活跃实例优先root Unix只读规则查询；原本已停止时安装器可对固定可信业务DB用SQLite URI mode=ro和query_only作固定SELECT，不开Store、不迁移、不恢复热日志、不写业务或改变owner/mode；读失败保守拒绝。共享数据库当前journal_mode=DELETE，不能用immutable绕过日志和恢复。服务停止确认后、备份完成与任何替换前再对固定DB复验，防安装锁未冻结业务造成API预检后的并发新增规则冲突；stopped阶段拒绝时只恢复原服务/记录，不拿旧数据覆盖新提交。CLI客户端仍不能离线读写库。

### 3.4 按模式就绪

HTTP检查沿用所选地址healthz精确响应；本机检查由root直接Unix HTTP `GET /v1/rules`校验状态与响应结构，不执行CLI首次选语言，不创建偏好。socket文件存在或进程active不等于管理入口已就绪。双入口都必须验证；检查前后复核服务运行。仅Web还应无本机控制入口，仅CLI应无Web监听。

## 4. 校验与错误矩阵

| 条件 | 结果 |
| --- | --- |
| 双组件关闭或非法布尔 | 服务启动/预检拒绝；交互安装双否重选，不提交安装。 |
| Web关闭、凭据缺失或HTTP字段无效 | 不因Web字段拒绝CLI；仍校验通用设置。 |
| Web开启、缺凭据或地址无效 | 配置拒绝，不启动不完整Web。 |
| Web关闭且本机控制平台不支持 | 明确拒绝没有实际管理入口，不能创建容器控制socket。 |
| schema3缺features、双false、布尔类型错误或env冲突 | 拒绝正常安装/切换，保留可信恢复信息。 |
| 候选旧程序不支持组件预检 | 停服前拒绝，提示需支持该功能的版本。 |
| 重跑选当前组合 | 幂等，不隐式升级或反复重启。 |
| PATH未知文件/替换与本轮CLI变化冲突 | 停服前拒绝，不接管/删除未知命令。 |
| 目标Web端口被外部进程或既有规则占用 | 启动/就绪失败回退原组合和数据，不能删除原转发规则解决。 |
| SIGTERM/强制中断、回退中断 | 持久阶段恢复可重入；备份不完整不得覆盖业务数据。 |
| 普通卸载/purge | 按组件精确清理托管程序和入口；正式配置/数据保留或删除边界沿用原合同。 |

## 5. 正常、基础与异常案例

- 正常：仅CLI新装只选择语言/组件，不输入Web信息；root的参数与交互CRUD及真实TCP/UDP转发成功。重跑补开Web时输入所需配置，规则ID/统计/归属不变。
- 基础：默认双入口保持旧体验，升级保留选择。关CLI移除托管文件与socket但保留语言，之后同版本补装恢复偏好；Docker仍只有Web。
- 异常：仅CLI已有转发占用原HTTP端口，开Web冲突需回退原CLI服务及规则；不自动换规则端口、删规则或静默用别的HTTP端口。

## 6. 必须验证

- 配置缺省/三组合/双关/非法布尔、禁用Web忽略其字段、容器有效入口拒绝、--check-config无DB/监听/偏好副作用。
- 原生实际三模式、真实root/nonroot授权、参数/PTY双语CRUD、三协议转发和重启恢复；双入口一致与Web-only无CLI/socket。
- 双语安装默认双开与全关重选；三模式安装/重跑/升级/重装/卸载/purge，六方向切换与Web配置/CLI偏好复用，同版本补装不升级。
- 旧schema1/2和旧未完成transaction恢复，schema3/env不一致拒绝；未知PATH文件、状态锁后改变、缺库保护和跨目录归属。
- 停服、备份中/完成、env、程序增删、PATH、健康、状态提交、回滚阶段失败/信号/SIGKILL重入；原文件字节/owner/mode、数据及active/enabled意图必须有断言。
- 现有vet/race/Docker标签/integration、双架构构建/发行、Docker smoke与完整安装回归。区分模拟systemd、命名空间真实进程和真实宿主系统；不得以交叉编译冒充arm64实机。

## 7. 错误与正确写法

错误：只跳过安装器凭据输入，服务仍绑定0.0.0.0:8080；仅Web不复制porter但仍按双程序检查完整性；凭socket存在报就绪；用客户端离线写库解决没有Web；重跑隐式下载新版本；新增事务字段后拒绝旧升级恢复。

正确：在所属配置、启动和安装状态层明确三组合，按实际组件校验/部署/就绪；重跑同版本切换，持久备份与可重入恢复保护全部托管资源和业务数据，旧格式恢复兼容。
