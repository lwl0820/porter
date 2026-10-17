# 代码研究与实现检查重点

## 已读取入口

- internal/model/rule.go：Rule 同时包含配置与输出运行态，Input 为独立可写契约；统计数据不能加入 Input。
- internal/store/store.go：单连接 SQLite、DELETE journal、FULL synchronous，当前 migrations 长度为 2；Save 更新配置字段，新增统计表避免名称编辑覆盖统计。
- internal/rules/service.go：所有写操作由 Service.mu 串行控制；Save 先准备监听、持久化、再切换；Retry 和启动校验手工重建 Input，新增开关不能漏掉。
- internal/rules/runtime_group.go：一条组合规则对应多个 Runtime，physicalRule 保留逻辑 ID；统计应按逻辑 ID 共享且独立于 budget 引用回收。
- internal/forward/tcp.go：io.Copy 双向传输并保留 CloseWrite 半关闭；仅使用 io.Copy 最终返回的总字节无法提供长连接即时统计。
- internal/forward/udp.go：发送上游与回复客户端是不同路径，计数使用实际 Write/WriteToUDP 返回值，队列丢弃不能计数。
- internal/httpapi/server.go：标准库路由、protect 鉴权及写入 CSRF/Origin、match revision 可直接复用重置操作。
- cmd/porter/main.go：正常结束显式 Close，加上 defer 还会再次 Close；统计循环和最终落盘必须幂等。
- web/src/api.ts：unknown 响应统一校验，输入仅 RuleInput，重置响应需要同样的解码与 revision。
- web/src/App.vue：轮询每 5 秒、隐藏暂停，桌面与窄屏分别渲染；注意提交前旧读取覆盖重置结果。
- web/src/components/RuleActions.vue：现有行按钮支持 busy 和可访问名称；重置可在统计区复用相同交互规范。

## 新增契约审查

- 累计量 JSON 使用十进制字符串，前端边界必须拒绝负值、非整数或无效字符串，格式化集中处理。
- 速度按后台采样生成，与查询频率无关；重置同时清理采样基线。
- 数据路径不能等待服务锁或数据库，防止 Service 持锁 Stop 等待形成环路。
- 关闭统计后仍可重置历史累计值，重置不得更改开关或断开连接。
- 旧周期快照不能覆盖重置，删除后不能再次写回；持久化失败不丢增量和旧 revision。
- 正常退出与强制退出持久化保证不同，完整说明在设计及 PRD 中，不宣传断电零丢失。

## 研究边界

本研究仅根据当前仓库源码与契约；未运行应用测试或浏览器验证，未开始产品实现。
