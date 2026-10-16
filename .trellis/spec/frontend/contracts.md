# 前端交互与接口契约

## 1. 范围与触发条件

适用于 `web/` 中 Vue 3、TypeScript、Vite 的管理界面、请求封装和前端嵌入。产品界面中文，不展示开发需求、验收清单或任务过程文字。

## 2. 签名与模块职责

- `src/api.ts`：唯一 fetch 与运行时响应校验边界，导出 `api`、`ApiError`、`setSession`。
- `src/types.ts`：Rule、RuleInput、Session、FieldErrors 契约。
- `src/App.vue`：登录、列表、搜索筛选、轮询与写操作协调。
- `src/components/RuleEditor.vue`：规则表单、字段校验、转发变更提示。
- `src/components/Modal.vue`：原生 dialog、初始焦点与关闭后焦点恢复。
- `web/embed.go`：`Handler() http.Handler`，内嵌 `dist` 并提供静态文件服务。

## 3. 请求与状态契约

所有请求同源，Cookie 不进入 localStorage；登录或会话恢复后保存内存 CSRF token，修改请求通过 `X-CSRF-Token` 发送。PUT、DELETE、重试使用带引号的 `If-Match`。GET 列表解析 `{rules: Rule[]}`，新增和修改解析单条 Rule。

新增和修改只序列化 RuleInput，不能展开完整 Rule 回传元数据。所有 unknown 响应在 api.ts 中校验，组件不重复类型断言。412 提示关闭编辑器并刷新，不静默重试覆盖。

列表每 5 秒同步，隐藏页面暂停；刷新失败保留上次数据并提示。提交期间禁止重复写操作，保存成功关闭编辑器后仍需复位全局 busy，不能依赖子组件卸载后的事件。没有全局状态框架，简单状态放组件内。

## 4. 校验与错误矩阵

| 情形 | 界面行为 |
| --- | --- |
| 401 | 退出当前管理态并显示登录。 |
| 字段错误 | 显示在字段旁，取消禁用后聚焦第一个错误。 |
| 409 监听冲突 | 保留表单，显示服务端原因。 |
| 412 版本过期 | 不覆盖最新规则，要求刷新。 |
| 请求失败 | 保留已有列表，允许重试。 |
| 空列表、筛选无结果 | 分别提供新增入口或清除筛选。 |
| 修改转发参数、停用或删除 | 明确说明会断开该规则现有会话。 |

## 5. 正常、基础与异常案例

- 正常：桌面表格与窄屏卡片可完成相同操作；TCP 与 UDP 标签清晰。
- 基础：名称、协议、监听 IP 与端口、目标主机与端口、启用状态组成完整输入。
- 异常：网络断开后仍可看到之前规则；会话过期不会一直留在不可操作页面。
- 无障碍：状态包含文字，按钮和表单有可访问名称，原生 dialog 管理焦点，删除默认焦点落在保留操作。

## 6. 必需验证

执行 `npm --prefix web run lint`、`type-check`、`test`、`build`；Vite build 不代替 TypeScript 检查。交互测试位于 `src/App.test.ts`，应以用户行为验证登录、CRUD、提交防重、字段错误、revision、CSRF、隐藏暂停轮询与过期退出。

必须实际浏览器检查窄屏。CSS 隐藏文字时，退出和刷新图标按钮必须保留显式 aria-label，不能只靠 SVG 或 title。对最终构建产物再执行 Go 构建，避免并发 npm build 清空 dist 导致 embed 失败。

## 7. 错误与正确写法

错误：`JSON.stringify({...rule, ...form})`，会把 id、status、revision 发给严格后端。

正确：只构造 RuleInput；revision 在 If-Match 头传递，运行态由后端返回。

错误：窄屏用 `display:none` 隐藏按钮中的 span，却没有 aria-label，导致按钮在辅助功能树中没有名称。

正确：退出按钮使用 `aria-label="退出登录"`，刷新使用 `aria-label="刷新规则"`，并保留相应回归断言。

源码依赖与锁文件同时更新；临时文件、npm 缓存、构建测试输出使用仓库 `.tmp/` 或被忽略的项目子目录，不访问系统临时目录。
