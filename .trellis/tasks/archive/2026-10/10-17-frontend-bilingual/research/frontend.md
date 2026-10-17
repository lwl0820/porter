# 现有前端与双语接入证据

- `web/package.json`：Vue 3.5.44，无现成国际化依赖；已有 lint、type-check、test、build。
- `web/src/App.vue`：管理启动、登录、规则列表、轮询、通知和删除/流量重置弹窗；消息目前保存为字符串，时间固定 zh-CN，桌面和窄屏分别显示规则卡片。
- `web/src/components/RuleEditor.vue`：客户端校验及服务端 fields 目前直接保存中文字符串；切换语言时必须保持表单和错误上下文。
- `web/src/types.ts`：集中 protocolDetails 与 statusLabels；协议标识、样式和图标保持不变，展示文案需要响应语言变更。
- `web/src/api.ts`：唯一 fetch/解码边界，ApiError 已包含 status、code、fields。需要保留 401 回登录、412 防覆盖以及 CSRF/If-Match 行为。
- `internal/httpapi/server.go`：credentials_invalid、unauthorized、csrf_invalid、origin_invalid、login_limited、validation_error、revision_required、revision_invalid、json_required、invalid_json、not_found、internal_error。
- `internal/rules/service.go`：shutting_down、not_found、revision_mismatch、rule_limit、database_error、disabled、admin_conflict、rule_conflict、listen_conflict、forward_loop；fields 有名称、协议、监听 IP、端口、目标主机及单播约束。
- `internal/rules/runtime_group.go:134`：监听消息为有限模板，含协议和检查地址、占用、权限的建议；`last_error_code=listen_failed`，另有未启动请重试提示。
- `web/src/App.test.ts`：已有真实 fetch 边界模拟和完整行为测试，默认中文需保持。测试模拟含 credentials / validation 等兼容 code，不能据此替换真实服务端 code 清单。
- `.trellis/spec/frontend/contracts.md`：要求四项前端验证、最后 Go 构建和实际窄屏浏览器检查。原“产品界面中文”约定在本任务批准后更新为中英文，不改变内部中文约束。
- 任务开始时已有多项未追踪 Trellis/代理配置；不纳入本任务提交，不删除或覆盖。
