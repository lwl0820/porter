# 双语交付验证

## 结果

中英文切换、偏好记忆、全部页面文案与已显示提示的动态翻译已实现。独立 trellis-check 复核发现并修复登录原生 required 气泡不跟随页面语言的问题；页面内提示保留必填语义及输入焦点。未发现未修复代码问题。

## 自动化检查

- 实现与复核代理执行 `npm --prefix web run lint`、`npm --prefix web run type-check`：通过。
- 最终 `npm --prefix web run test`：3 个测试文件、42 项通过，包含原有 29 项及新增 13 项双语回归。
- 最终 `npm --prefix web run build`：通过，产物脚本 `index-BPnBPhZ6.js`，样式 `index-Dy6SUCep.css`。
- 主会话在最终前端构建后执行 `CGO_ENABLED=0 go build -o .tmp/bilingual/porter ./cmd/porter`：通过；Go 临时与缓存目录均位于仓库 `.tmp/`。
- `git diff --check`：通过。

## 实际浏览器

使用独立 `.tmp/bilingual/preview.db` 和仅监听本机的 `127.0.0.1:18087` 验证实例，未使用用户实际数据。检查完成后已停止服务并关闭临时浏览器标签，恢复视口设置。

- 默认中文、切换 English，标题及描述元信息同步。
- 真实 credentials_invalid 登录失败显示英文，切回中文时已有错误同步更新。
- 最终构建复查空用户名和空密码提示随语言切换，焦点分别落在对应输入。
- English 登录后创建停用且开启统计的临时规则；规则中文名称保持原样。
- 规则编辑器空端口与目标校验英文正确，弹窗内切换中文保留名称及错误，再切英文正确更新。
- 桌面（默认 1265px）与窄屏（390×844）检查中文和 English；表单内容可滚动、保存操作与语言入口可达，统计卡片与长英文不溢出。
- 流量重置和删除确认中英文切换正确，取消返回且焦点恢复。成功重置、删除和异步防重等由行为测试覆盖。
- 刷新页面后保持 English；DOM 元信息为 lang=en、英文标题与描述；桌面内容宽度等于视口宽度。

## 截图

- `.tmp/bilingual/english-desktop.png`
- `.tmp/bilingual/english-mobile.png`
- `.tmp/bilingual/chinese-desktop.png`

## 收尾状态

质量门禁完成，前端国际化契约已同步。用户已确认提交计划，并授权合并 main、删除开发分支及推送，功能提交为 d19d15f，质量门禁完成，正在执行归档与 Git 交付。
