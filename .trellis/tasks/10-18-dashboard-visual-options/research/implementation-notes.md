# A 方案代码证据与实现提示

## 已确认的代码入口
- web/src/App.vue：固定role，两端页面与导航、登录、规则筛选/列表、AccountOverview、RuleEditor及确认弹窗均在此协调。普通用户当前在任意登录后页面都渲染完整AccountOverview，需拆分规则页摘要和账户页完整模式。
- web/src/components/AdminResources.vue：plans/users/settings分支，共用resource-form；users目前v-for AccountOverview卡片，应替换为独立表格。创建与busy/version/disposed等异步保护不可丢失。
- web/src/components/AccountOverview.vue：包含套餐、用量与UTC格式；不含页面级h1和进度图，需增加完整/紧凑模式或专用摘要。
- web/src/components/Modal.vue：原生dialog、showModal、关闭焦点恢复、busy拦截；RuleEditor现传drawer，桌面显示右侧全高抽屉，目标改居中。
- web/src/components/RuleTraffic.vue：累计与速度在同一组件；traffic.ts精确BigInt格式化，新增进度比例不得先把原始字节转Number。
- web/src/style.css：顶栏76px、内容max-width1352px且左右48px，资源表单max-width660px。用户列表需解除资源表单宽度限制；现有大量媒体规则和末尾覆盖需归并处理。
- web/src/accounts.test.ts、App.test.ts：已覆盖导航、busy、双端、字段错误、归属、套餐、配置、异步会话和组合协议；DOM变动不能删除业务断言。
- web/src/i18n.ts：集中词典、默认中文与偏好持久化。新增总用户数、弹窗标题等通过词条插值，不能直接拼中文。
- web/vite.config.ts：两端入口与/api代理到127.0.0.1:8080。最终Go服务嵌入构建后的web/dist；npm build与Go embed验证不可并发。

## 权威图像
A 管理端/a-admin.png、用户端/a-user.png 和表格修订/a-users-list-v2.png均位于本任务design-options。表格修订优先。蓝色字标、浅蓝背景、白色卡片、顶部横向导航、蓝色下划线、紧凑统计与表格、居中编辑为明显视觉特征。图中的错误日期/示例数值/协议下拉不是功能决策。

## 设计确认
用户已经选择A，且要求用户列表与内置浏览器复核。实现范围为双端全部页面，不仅是用户列表。无新增业务接口需求；新增用户入口可用Modal承载现有表单。B/C不实现。

## 检查重点
真实数据、账户nullable字段、超过安全整数的流量、额度超限进度、四种计费、三种周期、401/412、旧请求保护、桌面与窄屏中英文、dialog焦点与busy保护。截图与差异记录必须来自内置浏览器实际运行的实现。
