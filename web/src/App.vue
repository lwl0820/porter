<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from "vue";
import { api, ApiError, errorMessage, onUnauthorized, setSession } from "./api";
import {
  address,
  ruleInput,
  statusLabels,
  type Rule,
  type RuleInput,
  type RuleStatus,
  type Session,
} from "./types";
import Icon from "./components/Icon.vue";
import Modal from "./components/Modal.vue";
import RuleActions from "./components/RuleActions.vue";
import RuleEditor from "./components/RuleEditor.vue";

const session = ref<Session>();
const booting = ref(true);
const bootError = ref("");
const username = ref("");
const password = ref("");
const loginError = ref("");
const loggingIn = ref(false);
const loginInput = ref<HTMLInputElement>();
const rules = ref<Rule[]>([]);
const loaded = ref(false);
const loading = ref(false);
const listError = ref("");
const actionError = ref("");
const announcement = ref("");
const search = ref("");
const searchInput = ref<HTMLInputElement>();
const protocol = ref("");
const status = ref("");
const editorOpen = ref(false);
const editingRule = ref<Rule>();
const editorBusy = ref(false);
const deletingRule = ref<Rule>();
const deleteError = ref("");
const busy = ref("");
const lastRefresh = ref<Date>();
let requestController: AbortController | undefined;
let requestVersion = 0;
let sessionVersion = 0;
let timer: ReturnType<typeof setInterval> | undefined;
let toastTimer: ReturnType<typeof setTimeout> | undefined;

const counts = computed(() => ({
  all: rules.value.length,
  running: rules.value.filter((rule) => rule.status === "running").length,
  disabled: rules.value.filter((rule) => rule.status === "disabled").length,
  error: rules.value.filter((rule) => rule.status === "error").length,
}));
const filteredRules = computed(() => {
  const query = search.value.trim().toLocaleLowerCase();
  return rules.value.filter(
    (rule) =>
      (!protocol.value || rule.protocol === protocol.value) &&
      (!status.value || rule.status === status.value) &&
      (!query ||
        `${rule.name} ${rule.listen_host} ${rule.listen_port} ${rule.target_host} ${rule.target_port}`
          .toLocaleLowerCase()
          .includes(query)),
  );
});
const hasFilters = computed(() =>
  Boolean(search.value || protocol.value || status.value),
);
const refreshingText = computed(
  () =>
    lastRefresh.value?.toLocaleTimeString("zh-CN", { hour12: false }) ??
    "尚未同步",
);
const mutationBusy = computed(() => Boolean(busy.value) || editorBusy.value);

function notify(message: string): void {
  announcement.value = message;
  if (toastTimer) clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    announcement.value = "";
  }, 4000);
}
function stopListRequest(): void {
  requestVersion++;
  requestController?.abort();
  loading.value = false;
}
function expireSession(): void {
  sessionVersion++;
  stopListRequest();
  setSession();
  session.value = undefined;
  rules.value = [];
  loaded.value = false;
  editorOpen.value = false;
  editorBusy.value = false;
  deletingRule.value = undefined;
  actionError.value = "";
  listError.value = "";
  loginError.value = "登录已过期，请重新登录。";
  void nextTick(() => loginInput.value?.focus());
}
async function loadRules(): Promise<void> {
  if (!session.value || mutationBusy.value || loading.value) return;
  const version = ++requestVersion;
  requestController = new AbortController();
  loading.value = true;
  try {
    const result = await api.rules(requestController.signal);
    if (version !== requestVersion || !session.value) return;
    rules.value = result;
    loaded.value = true;
    listError.value = "";
    lastRefresh.value = new Date();
  } catch (error) {
    if (version !== requestVersion || !session.value) return;
    listError.value = errorMessage(error);
  } finally {
    if (version === requestVersion) loading.value = false;
  }
}
async function initialize(): Promise<void> {
  booting.value = true;
  bootError.value = "";
  try {
    const result = await api.session();
    session.value = result;
    setSession(result);
    loginError.value = "";
    await loadRules();
  } catch (error) {
    if (!(error instanceof ApiError && error.status === 401))
      bootError.value = errorMessage(error);
    loginError.value = "";
  } finally {
    booting.value = false;
    await nextTick();
    if (!session.value && !bootError.value) loginInput.value?.focus();
  }
}
async function login(): Promise<void> {
  if (loggingIn.value) return;
  loginError.value = "";
  loggingIn.value = true;
  try {
    const result = await api.login(username.value.trim(), password.value);
    sessionVersion++;
    session.value = result;
    setSession(result);
    password.value = "";
    await loadRules();
  } catch (error) {
    loginError.value = errorMessage(error);
  } finally {
    loggingIn.value = false;
  }
}
async function logout(): Promise<void> {
  if (mutationBusy.value) return;
  busy.value = "logout";
  stopListRequest();
  try {
    await api.logout();
    expireSession();
    loginError.value = "";
    username.value = "";
  } catch (error) {
    if (session.value) actionError.value = errorMessage(error);
  } finally {
    busy.value = "";
  }
}
function openEditor(rule?: Rule): void {
  editingRule.value = rule ? { ...rule } : undefined;
  editorOpen.value = true;
}
function mergeRule(rule: Rule): void {
  const index = rules.value.findIndex((item) => item.id === rule.id);
  if (index >= 0) rules.value[index] = rule;
  else rules.value.push(rule);
  loaded.value = true;
  lastRefresh.value = new Date();
}
async function save(input: RuleInput): Promise<void> {
  stopListRequest();
  const version = sessionVersion;
  try {
    const result = editingRule.value
      ? await api.update(editingRule.value, input)
      : await api.create(input);
    if (version !== sessionVersion) return;
    mergeRule(result);
    notify(editingRule.value ? "规则已更新" : "规则已创建");
  } catch (error) {
    if (error instanceof ApiError && error.status === 412)
      listError.value = "规则已在其他页面变更，请刷新后重新编辑。";
    throw error;
  }
}
async function mutate(
  rule: Rule,
  operation: "toggle" | "retry",
): Promise<void> {
  if (mutationBusy.value) return;
  busy.value = rule.id;
  actionError.value = "";
  stopListRequest();
  const version = sessionVersion;
  try {
    const result =
      operation === "retry"
        ? await api.retry(rule)
        : await api.update(rule, {
            ...ruleInput(rule),
            enabled: !rule.enabled,
          });
    if (version !== sessionVersion) return;
    mergeRule(result);
    notify(
      operation === "retry"
        ? "已重新尝试启动规则"
        : result.enabled
          ? "规则已启用"
          : "规则已停用，现有会话已断开",
    );
  } catch (error) {
    if (session.value) actionError.value = errorMessage(error);
  } finally {
    busy.value = "";
  }
}
function confirmDelete(rule: Rule): void {
  deletingRule.value = { ...rule };
  deleteError.value = "";
}
async function remove(): Promise<void> {
  const rule = deletingRule.value;
  if (!rule || mutationBusy.value) return;
  busy.value = rule.id;
  deleteError.value = "";
  stopListRequest();
  const version = sessionVersion;
  try {
    await api.remove(rule);
    if (version !== sessionVersion) return;
    rules.value = rules.value.filter((item) => item.id !== rule.id);
    deletingRule.value = undefined;
    notify("规则已删除");
  } catch (error) {
    deleteError.value = errorMessage(error);
  } finally {
    busy.value = "";
  }
}
function clearFilters(): void {
  search.value = "";
  protocol.value = "";
  status.value = "";
}
function filterStatus(value: RuleStatus | ""): void {
  status.value = status.value === value ? "" : value;
}
function searchShortcut(event: KeyboardEvent): void {
  if (
    event.key !== "/" ||
    !session.value ||
    editorOpen.value ||
    deletingRule.value
  )
    return;
  const target = event.target;
  if (
    target instanceof HTMLElement &&
    (["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName) ||
      target.isContentEditable)
  )
    return;
  event.preventDefault();
  searchInput.value?.focus();
}
function visibilityChanged(): void {
  if (document.visibilityState === "visible") void loadRules();
}

onMounted(() => {
  onUnauthorized(expireSession);
  void initialize();
  timer = setInterval(() => {
    if (
      document.visibilityState === "visible" &&
      !editorOpen.value &&
      !deletingRule.value
    )
      void loadRules();
  }, 5000);
  document.addEventListener("visibilitychange", visibilityChanged);
  document.addEventListener("keydown", searchShortcut);
});
onUnmounted(() => {
  onUnauthorized();
  stopListRequest();
  clearInterval(timer);
  clearTimeout(toastTimer);
  document.removeEventListener("visibilitychange", visibilityChanged);
  document.removeEventListener("keydown", searchShortcut);
});
</script>

<template>
  <div v-if="booting || bootError" class="startup-screen">
    <div class="brand-mark"><Icon name="arrows" :size="28" /></div>
    <template v-if="booting"
      >
<span class="spinner" />
      <p>正在连接 Porter…</p>
</template
    >
    <template v-else
      >
<p role="alert">{{ bootError }}</p>
      <button class="button primary" @click="initialize">
        重新连接
      </button>
</template
    >
  </div>

  <main v-else-if="!session" class="login-page">
    <section class="login-story" aria-label="Porter 介绍">
      <a class="brand" href="/" aria-label="Porter 首页"
        ><span class="brand-mark"><Icon name="arrows" :size="25" /></span
        ><span>Porter<span class="brand-period">.</span></span></a
      >
      <div class="story-content">
        <span class="eyebrow">让连接井然有序</span>
        <h1>每一个端口，<br />都有清晰的去向。</h1>
        <p>在一个面板中管理 TCP 与 UDP 转发，<br />让你的服务轻松连接。</p>
        <div class="network-art" aria-hidden="true">
          <div class="art-endpoint">
            <Icon name="network" :size="28" /><span>本机端口</span>
          </div>
          <div class="art-link">
            <i /><i /><i /><Icon name="arrow" :size="24" />
          </div>
          <div class="art-center"><Icon name="arrows" :size="34" /></div>
          <div class="art-link">
            <i /><i /><i /><Icon name="arrow" :size="24" />
          </div>
          <div class="art-endpoint">
            <Icon name="layers" :size="28" /><span>目标服务</span>
          </div>
        </div>
      </div>
      <span class="story-footer">简单连接 · 轻松掌控</span>
    </section>
    <section class="login-form-area">
      <div class="login-form-wrap">
        <span class="login-mobile-brand"
          >Porter<span class="brand-period">.</span></span
        >
        <div class="login-lock"><Icon name="lock" :size="25" /></div>
        <h2>欢迎回来</h2>
        <p class="login-description">登录以管理你的端口转发规则</p>
        <form @submit.prevent="login">
          <div v-if="loginError" role="alert" class="notice error">
            <Icon name="alert" :size="18" /><span>{{ loginError }}</span>
          </div>
          <div class="field">
            <label for="username">用户名</label
            ><input
              id="username"
              ref="loginInput"
              v-model="username"
              autocomplete="username"
              placeholder="请输入管理员用户名"
              required
              :disabled="loggingIn"
            />
          </div>
          <div class="field">
            <label for="password">密码</label
            ><input
              id="password"
              v-model="password"
              type="password"
              autocomplete="current-password"
              placeholder="请输入密码"
              required
              :disabled="loggingIn"
            />
          </div>
          <button
            class="button primary login-submit"
            type="submit"
            :disabled="loggingIn"
          >
            <span v-if="loggingIn" class="spinner small" />{{
              loggingIn ? "正在登录…" : "登录面板"
            }}<Icon v-if="!loggingIn" name="arrow" :size="18" />
          </button>
        </form>
        <div class="login-security">
          <Icon name="shield" :size="16" /><span>仅限授权管理员访问</span>
        </div>
      </div>
      <span class="login-footer">Porter · 端口转发管理</span>
    </section>
  </main>

  <div v-else class="app-shell">
    <a class="skip-link" href="#main-content">跳转到主内容</a>
    <header class="topbar">
      <div class="topbar-inner">
        <a class="brand" href="/" aria-label="Porter 首页"
          ><span class="brand-mark"><Icon name="arrows" :size="24" /></span
          ><span>Porter<span class="brand-period">.</span></span></a
        ><span class="header-divider" /><span class="topbar-section"
          ><Icon name="network" :size="17" />端口转发</span
        >
        <div class="account">
          <span class="account-avatar"><Icon name="user" :size="17" /></span
          ><span class="account-name">{{ session.username }}</span
          ><button
            type="button"
            class="logout-button"
            aria-label="退出登录"
            :disabled="mutationBusy"
            @click="logout"
          >
            <Icon name="logout" :size="17" /><span>退出登录</span>
          </button>
        </div>
      </div>
    </header>
    <main id="main-content" class="dashboard" tabindex="-1">
      <div class="page-heading">
        <div>
          <div class="eyebrow">网络管理</div>
          <h1>端口转发</h1>
          <p>为你的服务建立连接，统一管理每一条转发规则。</p>
        </div>
        <button
          type="button"
          class="button primary"
          :disabled="mutationBusy"
          @click="openEditor()"
        >
          <Icon name="plus" :size="19" />新增规则
        </button>
      </div>
      <section class="summary-grid" aria-label="规则概览">
        <button
          class="summary-card"
          :class="{ active: status === '' }"
          :aria-pressed="status === ''"
          @click="status = ''"
        >
          <span class="summary-icon blue"
            ><Icon name="layers" :size="22" /></span
          ><span class="summary-body"
            ><span>全部规则</span
            ><strong
              >{{ loaded ? counts.all : "—" }}<small>条</small></strong
            ></span
          ><span class="summary-accent blue" />
        </button>
        <button
          class="summary-card"
          :class="{ active: status === 'running' }"
          :aria-pressed="status === 'running'"
          @click="filterStatus('running')"
        >
          <span class="summary-icon teal"
            ><Icon name="activity" :size="22" /></span
          ><span class="summary-body"
            ><span>运行中</span
            ><strong
              >{{ loaded ? counts.running : "—" }}<small>条</small></strong
            ></span
          ><span class="summary-accent teal" />
        </button>
        <button
          class="summary-card"
          :class="{ active: status === 'disabled' }"
          :aria-pressed="status === 'disabled'"
          @click="filterStatus('disabled')"
        >
          <span class="summary-icon gray"><Icon name="pause" :size="22" /></span
          ><span class="summary-body"
            ><span>已停用</span
            ><strong
              >{{ loaded ? counts.disabled : "—" }}<small>条</small></strong
            ></span
          ><span class="summary-accent gray" />
        </button>
        <button
          class="summary-card"
          :class="{ active: status === 'error' }"
          :aria-pressed="status === 'error'"
          @click="filterStatus('error')"
        >
          <span class="summary-icon amber"
            ><Icon name="alert" :size="22" /></span
          ><span class="summary-body"
            ><span>需要关注</span
            ><strong
              >{{ loaded ? counts.error : "—" }}<small>条</small></strong
            ></span
          ><span class="summary-accent amber" />
        </button>
      </section>
      <div v-if="actionError" role="alert" class="notice error dismissible">
        <Icon name="alert" /><span>{{ actionError }}</span
        ><button
          class="icon-button"
          aria-label="关闭错误提示"
          @click="actionError = ''"
        >
          <Icon name="close" :size="17" />
        </button>
      </div>
      <section
        class="rules-panel"
        aria-label="转发规则列表"
        :aria-busy="loading"
      >
        <div class="panel-heading">
          <h2>
            转发规则<span class="count-badge">{{
              loaded ? counts.all : "—"
            }}</span>
          </h2>
          <span class="sync-indicator"
            ><span class="live-dot" />每 5 秒同步</span
          >
        </div>
        <div class="toolbar">
          <div class="search-box">
            <Icon name="search" :size="18" /><input
              ref="searchInput"
              v-model="search"
              aria-label="搜索规则"
              placeholder="搜索规则名称、IP 或域名…"
              type="search"
            /><kbd>/</kbd>
          </div>
          <div class="filters">
            <select v-model="protocol" aria-label="按协议筛选">
              <option value="">全部协议</option>
              <option value="tcp">TCP</option>
              <option value="udp">UDP</option>
</select
            ><select v-model="status" aria-label="按状态筛选">
              <option value="">全部状态</option>
              <option value="running">运行中</option>
              <option value="disabled">已停用</option>
              <option value="error">错误</option>
</select
            ><button
              class="button refresh-button"
              aria-label="刷新规则"
              :disabled="loading || mutationBusy"
              @click="loadRules"
            >
              <Icon
                name="refresh"
                :size="17"
                :class="{ spinning: loading }"
              /><span>刷新</span>
            </button>
          </div>
        </div>
        <div v-if="listError" role="alert" class="notice error list-notice">
          <Icon name="alert" /><span
            >{{ listError
            }}{{ loaded ? " 当前保留上次同步的数据。" : "" }}</span
          ><button
            class="text-button"
            :disabled="loading || mutationBusy"
            @click="loadRules"
          >
            重试
          </button>
        </div>
        <div v-if="!loaded" class="empty-state">
          <template v-if="loading"
            >
<span class="spinner" />
            <h3>正在加载规则</h3>
            <p>稍等片刻，正在同步你的转发配置。</p>
</template
          ><template v-else
            >
<span class="empty-icon"><Icon name="alert" :size="29" /></span>
            <h3>暂时无法获取规则</h3>
            <p>请检查网络连接，然后重新尝试。</p>
            <button class="button secondary" @click="loadRules">
              重新加载
            </button>
</template
          >
        </div>
        <template v-else-if="filteredRules.length">
          <div class="desktop-table">
            <table>
              <thead>
                <tr>
                  <th scope="col">规则名称</th>
                  <th scope="col">协议</th>
                  <th scope="col">监听地址</th>
                  <th scope="col" class="direction-cell" />
                  <th scope="col">转发目标</th>
                  <th scope="col">运行状态</th>
                  <th scope="col" class="operation-heading">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="rule in filteredRules" :key="rule.id">
                  <td class="rule-name-cell">
                    <span class="rule-avatar" :class="rule.protocol"
                      ><Icon
                        :name="rule.protocol === 'tcp' ? 'arrows' : 'network'"
                        :size="18" /></span
                    ><strong :title="rule.name">{{ rule.name }}</strong>
                  </td>
                  <td>
                    <span class="protocol-badge" :class="rule.protocol">{{
                      rule.protocol.toUpperCase()
                    }}</span>
                  </td>
                  <td class="address-cell">
                    <code
                      :title="address(rule.listen_host, rule.listen_port)"
                      >{{ address(rule.listen_host, rule.listen_port) }}</code
                    >
                  </td>
                  <td class="direction-cell">
                    <Icon name="arrow" :size="16" />
                  </td>
                  <td class="address-cell">
                    <code
                      :title="address(rule.target_host, rule.target_port)"
                      >{{ address(rule.target_host, rule.target_port) }}</code
                    >
                  </td>
                  <td class="status-cell">
                    <span
                      class="status-badge"
                      :class="rule.status"
                      :title="
                        rule.status === 'running'
                          ? '本机监听正常，不代表目标服务始终可达'
                          : undefined
                      "
                      ><i />{{ statusLabels[rule.status] }}</span
                    ><span
                      v-if="rule.last_error && rule.status === 'error'"
                      class="rule-error"
                      :title="rule.last_error"
                      >{{ rule.last_error }}</span
                    >
                  </td>
                  <td>
                    <RuleActions
                      :rule="rule"
                      :busy="mutationBusy"
                      @edit="openEditor"
                      @remove="confirmDelete"
                      @toggle="mutate($event, 'toggle')"
                      @retry="mutate($event, 'retry')"
                    />
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="mobile-cards">
            <article
              v-for="rule in filteredRules"
              :key="rule.id"
              class="rule-card"
            >
              <div class="rule-card-title">
                <h3>
                  <span class="rule-avatar" :class="rule.protocol"
                    ><Icon name="arrows" :size="17" /></span
                  >{{ rule.name }}
                </h3>
                <span class="protocol-badge" :class="rule.protocol">{{
                  rule.protocol.toUpperCase()
                }}</span>
              </div>
              <div class="mobile-route">
                <div>
                  <span>监听地址</span
                  ><code>{{
                    address(rule.listen_host, rule.listen_port)
                  }}</code>
                </div>
                <Icon name="arrow" :size="18" />
                <div>
                  <span>转发目标</span
                  ><code>{{
                    address(rule.target_host, rule.target_port)
                  }}</code>
                </div>
              </div>
              <p
                v-if="rule.last_error && rule.status === 'error'"
                class="mobile-rule-error"
              >
                {{ rule.last_error }}
              </p>
              <div class="rule-card-footer">
                <span class="status-badge" :class="rule.status"
                  ><i />{{ statusLabels[rule.status] }}</span
                ><RuleActions
                  :rule="rule"
                  :busy="mutationBusy"
                  @edit="openEditor"
                  @remove="confirmDelete"
                  @toggle="mutate($event, 'toggle')"
                  @retry="mutate($event, 'retry')"
                />
              </div>
            </article>
          </div>
        </template>
        <div v-else class="empty-state">
          <span class="empty-icon"
            ><Icon :name="hasFilters ? 'search' : 'arrows'" :size="32"
          /></span>
          <h3>{{ hasFilters ? "没有找到匹配的规则" : "从第一条规则开始" }}</h3>
          <p>
            {{
              hasFilters
                ? "试试其他关键词，或调整筛选条件。"
                : "添加本机监听端口和目标地址，即可开始转发。"
            }}
          </p>
          <button
            v-if="hasFilters"
            class="button secondary"
            @click="clearFilters"
          >
            清除筛选
</button
          ><button
            v-else
            class="button primary"
            :disabled="mutationBusy"
            @click="openEditor()"
          >
            <Icon name="plus" :size="18" />新增规则
          </button>
        </div>
        <footer class="panel-footer">
          <span>{{
            hasFilters
              ? `显示 ${filteredRules.length} 条，共 ${counts.all} 条规则`
              : `共 ${loaded ? counts.all : "—"} 条规则`
          }}</span
          ><span>最后同步 {{ refreshingText }}</span>
        </footer>
      </section>
      <div class="dashboard-note">
        <Icon name="shield" :size="16" />
        <p>“运行中”表示本机监听正常。停用规则会立即断开现有会话。</p>
      </div>
    </main>
    <footer class="app-footer">
      <span>Porter<span class="brand-period">.</span></span
      ><span>让连接井然有序</span><span>TCP / UDP</span>
    </footer>
    <Transition name="toast"
      >
<div v-if="announcement" role="status" class="toast-notice">
        <span class="toast-icon"><Icon name="check" :size="17" /></span
        >{{ announcement }}
      </div>
</Transition
    >
    <RuleEditor
      v-if="editorOpen"
      :rule="editingRule"
      :save="save"
      @close="editorOpen = false"
      @busy="editorBusy = $event"
    />
    <Modal
      v-if="deletingRule"
      label="删除规则"
      :busy="Boolean(busy)"
      @close="deletingRule = undefined"
      >
<div class="delete-dialog">
        <span class="delete-icon"><Icon name="trash" :size="26" /></span>
        <h2>删除这条规则？</h2>
        <p>
          确定删除 <strong>{{ deletingRule.name }}</strong> 吗？
        </p>
        <code
          >{{ address(deletingRule.listen_host, deletingRule.listen_port) }} →
          {{
            address(deletingRule.target_host, deletingRule.target_port)
          }}</code
        >
        <p class="delete-warning">
          转发将立即停止，现有会话会被断开。此操作无法撤销。
        </p>
        <div v-if="deleteError" role="alert" class="notice error">
          {{ deleteError }}
        </div>
        <div class="delete-actions">
          <button
            class="button secondary"
            data-autofocus
            :disabled="Boolean(busy)"
            @click="deletingRule = undefined"
          >
            保留规则
</button
          ><button
            class="button danger"
            :disabled="Boolean(busy)"
            @click="remove"
          >
            <span v-if="busy" class="spinner small" />{{
              busy ? "正在删除…" : "确认删除"
            }}
          </button>
        </div>
      </div>
</Modal
    >
  </div>
</template>
