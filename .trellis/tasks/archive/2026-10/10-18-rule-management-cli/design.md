# 技术设计

## 边界与数据流

CLI 语言选择或显式语言解析 → 交互菜单收集字段或直接解析命令参数 → 共用操作层 → Unix 域套接字客户端 → 独立本机控制服务 → 现有 rules.Service → SQLite 与转发引擎。

复用 `cmd/porter` 作为入口，按首个子命令分流；CLI 在加载服务凭据、数据库和日志初始化之前执行。服务继续保留现有 Web 登录、CSRF、Origin 和管理员权限校验。本机控制服务采用单独 handler，不将其挂载到 Web HTTP mux，不修改 Web 鉴权路径。

建议新增 `internal/cli` 负责参数、双语与展示，`internal/localapi` 负责本机协议、客户端和监听生命周期；既有 `internal/model` 继续拥有规则业务模型，不复制规则校验和运行态协调逻辑。

交互菜单和参数入口共用 CLI 操作层、类型化输入、部分更新合并、revision 策略、错误翻译和展示函数；不得各自复制一套业务校验或直接操作数据库。

## 本机通道与授权

- 原生服务默认在数据库目录旁创建 `porter-control.sock`，原生安装默认 `/var/lib/porter/porter-control.sock`。
- 服务支持 `PORTER_CONTROL_SOCKET` 覆盖路径；CLI 使用 `--socket`，其次同名环境变量，最后原生安装默认路径。手动运行自定义数据库时，文档要求 CLI 指定相同控制路径。
- 套接字权限为 0600，父目录及全部祖先必须归属 root 或服务 UID，并拒绝 group/other 写权限和符号链接；其他 owner 即使当前只读也可自行改权，因此拒绝。控制入口在 Linux 读取实际对端凭据，服务端仅允许 UID 0；不能只依赖客户端的 root 检查、请求头或请求中的角色。
- 服务实际由 porter 非登录用户运行，root 客户端仍能访问；不能把服务用户身份直接当作调用方身份。业务 actor 固定为服务中权威的管理员身份。
- 本机通信可采用标准库 HTTP over Unix socket，复用业务模型与错误包络，不引入第三方 CLI 框架。
- 现有依赖含 `golang.org/x/sys`，本地缓存存在 Linux 对端凭据读取实现；实现时依实际使用将直接依赖关系整理清楚。
- 创建前拒绝普通文件、符号链接及活动监听冲突。仅在证实为本实例路径下的失效套接字后清理；退出仅清理自身创建的节点，不能误删其他实例替换的套接字。
- 请求体大小、头部、读写和客户端超时均有限制；服务关闭时先停止接收本机操作并等待请求，再执行规则服务最终保存和数据库关闭。

## 命令合同

| 命令 | 参数及行为 |
| --- | --- |
| `rules` | 不提供动作子命令时进入交互菜单；支持 --lang、--socket、--help，不接受 --json。 |
| `rules add` | 必填 `--name --protocol --listen-host --listen-port --target-host --target-port`；可选 `--enabled --traffic-enabled --owner-id`。 |
| `rules list` | 可选 `--owner-id`，返回全部或对应归属规则。 |
| `rules get ID` | 返回单条详情，不存在返回 not_found。 |
| `rules update ID` | 仅合并显式配置参数；允许修改 enabled/traffic-enabled，不接受 owner-id。 |
| `rules delete ID` | 删除单条规则，不增加确认提示；删除不可逆，帮助明确说明。 |

通用参数为 `--lang zh-CN|en`、`--json`、`--socket PATH`、`--help`。帮助展示准确参数位置，保证与实际解析器一致，不暗示任意位置均可识别。布尔参数使用明确的 `--enabled=true/false`、`--traffic-enabled=true/false`，区分省略与 false。update 不含任何修改字段时返回用法错误，不触发无意义 revision 更新。

## 两种操作方式

- `porter rules` 在完成语言流程后进入菜单。菜单提供创建、列表、详情、修改、删除及退出，不加入用户或套餐入口。显式 --lang 只跳过语言选择，不跳过管理菜单。
- 创建向导逐项收集必要字段和可选归属 ID、启用及统计开关，提交前展示配置并确认；创建默认值沿用参数方式及服务合同。
- 列表显示稳定 ID 和归属；详情、修改、删除可从当前列表选择规则。序号仅用于当前视图，必须转换为对应 ID，不能把序号作为持久标识。
- 修改加载所选规则快照并显示当前值，保留未修改字段，空输入保留原值；不允许转移归属。提交使用该快照的 revision，发生冲突提示重新加载，不自动提交覆盖。
- 删除展示规则详情并要求显式确认，默认取消；使用展示时的 revision 删除，避免确认后误删已被修改的规则。
- 所有菜单读写及确认使用控制终端，不读取 stdin 中的管道内容；复用相同的表格和详情渲染函数，将交互展示写到终端。取消/返回只丢弃当前未提交输入，已成功的操作仍然生效；输入结束或中断则结束会话。
- 业务字段错误可保留待编辑内容并重新输入，提交失败时展示错误并允许返回菜单；取消不能导致补发请求。变更请求超时则提示先查看结果，不自动重发。
- `porter rules ACTION ...` 始终为参数模式。缺少字段或 ID 直接报错，不自动补问；完整参数和 --lang 允许在无终端环境运行，参数删除没有二次确认。
- --json 仅用于单次参数操作；`porter rules --json` 在语言确定后返回用法错误，不启动菜单。交互管理即使指定了 --lang，也必须有可用控制终端。

## 语言选择流程

- 对规则 CLI 及 CLI 帮助，先按参数语法识别显式语言，支持 `--lang en` 和 `--lang=en`；不能把其他参数值中碰巧出现的字符串当作语言标记。无效显式语言用双语说明合法值，退出码 2，不触发操作。
- 无显式语言时，在 `/dev/tty` 首先展示双语选择“语言 / Language：1 中文，2 English”，等待有效选择；空白或其他无效输入重新提示，不默认选择中文。
- 语言确定前不输出命令用法错误、业务提示，不检查调用权限、不连接控制服务；未知子命令、缺失业务字段及 --help 同样遵守此顺序。
- 每次进程进入 CLI 时未指定语言都询问，不缓存语言，也不从 LANG、浏览器或安装器状态自动推断。一次交互会话只选择一次，返回菜单不再询问。选择后才输出对应语言帮助、参数错误、权限错误或操作结果。
- 通过控制终端读写选择流程，独立于 stdin/stdout/stderr；stdin 被重定向而仍有控制终端时可以继续选择，stdout 的表格和 JSON 都不混入提示。
- 无法打开控制终端且无语言参数时，用双语说明必须显式传入 `--lang`，按用法错误退出；--json 模式保持标准错误为稳定 code 的可解析错误 JSON。显式语言在无终端场景照常执行。
- 输入结束或中断时关闭控制终端并退出，不执行规则操作；无子命令的服务启动和原有 --version 不进入 CLI 语言流程。

命令示例：

```bash
sudo /opt/porter/porter rules
sudo /opt/porter/porter rules --lang en
sudo /opt/porter/porter rules add --name 网站转发 --protocol tcp --listen-host 0.0.0.0 --listen-port 18081 --target-host example.com --target-port 80
sudo /opt/porter/porter rules list --lang en
sudo /opt/porter/porter rules get 规则ID --json --lang zh-CN
sudo /opt/porter/porter rules update 规则ID --enabled=false
sudo /opt/porter/porter rules delete 规则ID
```

前两条进入交互管理；其余通过参数直接管理。上述未指定 `--lang` 的示例均会先选择语言；无人值守调用需显式提供动作、完整参数及语言。

## 本机协议与版本

- 本机提供限定的规则 list/get/create/update/delete 操作，不提供用户、套餐、设置、重试或统计重置路由。
- 列表和详情复用管理员 `ListFor` 权限结果；如新增服务层详情方法，必须复用服务锁、授权和 `ruleFor`，不读取 Store 绕过运行态。
- 创建/修改调用 `SaveFor`，删除调用 `DeleteFor`，保留现有校验、审计、提交失败回滚和规则归属语义。
- 更新先读取最新详情，将显式字段合并为完整 Input，再带其 revision 执行；删除先获取当前 revision 再删除。提供可选 `--revision` 供脚本固定版本；指定时不能以新 revision 替代。
- 服务端仍要求有效 If-Match；读后发生并发更新则返回 revision_mismatch，不自动重试写操作。超时可能意味着服务端已完成写入，提示先查询，不自动重发创建。
- 复用 Rule/Input/AdminRuleCreateInput；响应错误采用现有 code/message/fields 结构。CLI 依据稳定 code 及已知字段约束集中生成双语信息，不把动态中文服务错误原样展示在英文业务提示中；未知错误使用所选语言的通用提示和稳定代码。
- JSON 字段采用既有 snake_case、计数字符串和状态值，稳定代码保留；可读输出翻译列名及状态。错误 JSON 同样可解析且输出到标准错误。

## Docker 与原生兼容

- Dockerfile 通过专用构建标签（建议 porter_docker）编译硬性禁用规则命令及本机控制监听的版本，环境变量不能解除禁用。原生 Release 保持常规构建。
- 原生构建在执行规则命令和创建本机入口前检查 Docker 标记，包括 .dockerenv 以及可用的容器环境线索；识别容器时停止本机入口，Web 服务照常运行。
- 对无法识别且人为隐藏标记的容器不作完整侦测承诺；镜像构建硬性关闭是受支持 Docker 部署的主要保证。
- 不新增数据库迁移，不改变原生安装 unit 或全局 PATH，避免破坏安装器对现有 unit 的严格识别及卸载逻辑。
- 服务启动创建 socket，退出清理；原生数据目录本就由服务用户可写。卸载先停服务即可清理，purge 沿用整目录删除。验证异常中断后失效 socket 的恢复。
- Docker 二进制的 `--version`、现有 Web 管理及转发功能保持兼容。

## 运行及回退

更换原生程序后需重启服务以启用控制通道。CLI 连接旧服务失败时提供检查运行状态、路径及升级的提示。只新增运行时 socket，不更改数据库格式；回退前停止服务并按现有备份方案恢复旧程序，移除已确认归属的残留 socket，不删除规则和统计数据。
