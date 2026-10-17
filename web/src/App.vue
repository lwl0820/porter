<script setup lang="ts">
import { initializeLanguage, language, t, type MessageKey } from "./i18n";
import LanguageSwitcher from "./components/LanguageSwitcher.vue";
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import {
  adminApi, userApi, ApiError, errorMessage, ruleErrorMessage,
} from "./api";
import {
  address,
  matchesProtocol,
  protocolDetails,
  type Protocol,
  ruleInput,
  statusLabels,
  type Rule,
  type RuleInput,
  type RuleStatus,
  type Session, type Role, type Account,
} from "./types";
import Icon from "./components/Icon.vue";
import Modal from "./components/Modal.vue";
import RuleActions from "./components/RuleActions.vue";
import RuleTraffic from "./components/RuleTraffic.vue";
import RuleEditor from "./components/RuleEditor.vue";

import AdminResources from "./components/AdminResources.vue";
import AccountOverview from "./components/AccountOverview.vue";
const props = withDefaults(defineProps<{ role?: Role }>(), { role: "admin" });
const isAdmin = props.role === "admin";
const api = isAdmin ? adminApi : userApi;
const { setSession, onUnauthorized } = api;
const validPages = isAdmin ? ["rules", "plans", "users", "settings"] : ["rules", "account"];
const initialPage = window.location.pathname.split("/")[2] || "rules";
const page = ref(validPages.includes(initialPage) ? initialPage : "rules");
const resourceBusy = ref(false);
const navigationPages: { value: string; label: MessageKey }[] = [
  { value: "rules", label: "转发规则" },
  ...(isAdmin ? [
    { value: "plans", label: "套餐管理" as const },
    { value: "users", label: "用户管理" as const },
    { value: "settings", label: "系统配置" as const },
  ] : [{ value: "account", label: "账户概览" as const }]),
];
const navigationOpen = ref(false);
const navigationToggle = ref<HTMLButtonElement>();
const navigationId = `${props.role}-navigation`;
const users = ref<Account[]>([]);
const account = ref<Account>();
const ownerId = ref("");
const editorOwner = ref("");
const ownerOptions = computed(() => session.value ? [
  { id: session.value.user_id, username: session.value.username, role: session.value.role },
  ...users.value.map(item => item.user),
] : []);
const editorAccount = computed(() => isAdmin ? users.value.find(item => item.user.id === editorOwner.value) : account.value);
initializeLanguage();

const session = ref<Session>();
const booting = ref(true);
const bootError = ref<unknown>("");
const username = ref("");
const password = ref("");
const loginError = ref<unknown>("");
const loggingIn = ref(false);
const loginInput = ref<HTMLInputElement>();
const passwordInput = ref<HTMLInputElement>();
const rules = ref<Rule[]>([]);
const loaded = ref(false);
const loading = ref(false);
const listError = ref<unknown>("");
const actionError = ref<unknown>("");
const announcement = ref<MessageKey | "">("");
const search = ref("");
const searchInput = ref<HTMLInputElement>();
const protocol = ref<Protocol | "">("");
const status = ref("");
const editorOpen = ref(false);
const editingRule = ref<Rule>();
const editorBusy = ref(false);
const deletingRule = ref<Rule>();
const resettingRule = ref<Rule>();
const resetError = ref<unknown>("");
const deleteError = ref<unknown>("");
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
      matchesProtocol(rule.protocol, protocol.value) &&
      (!status.value || rule.status === status.value) &&
      (!query ||
        `${rule.name} ${rule.listen_host} ${rule.listen_port} ${rule.target_host} ${rule.target_port}`
          .toLocaleLowerCase()
          .includes(query)),
  );
});
const hasFilters = computed(() =>
  Boolean(search.value || protocol.value || status.value || ownerId.value),
);
const refreshingText = computed(
  () =>
    lastRefresh.value?.toLocaleTimeString(language.value, { hour12: false }) ??
    t("尚未同步"),
);
const mutationBusy = computed(() => Boolean(busy.value) || editorBusy.value || resourceBusy.value);

function closeNavigation(): void {
  if (!navigationOpen.value) return;
  navigationOpen.value = false;
  void nextTick(() => navigationToggle.value?.focus());
}
function navigate(value: string): void {
  if (mutationBusy.value) return;
  page.value = value;
  closeNavigation();
}
function notify(message: MessageKey): void {
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
  navigationOpen.value = false;
  rules.value = [];
  users.value = []; account.value = undefined; ownerId.value = ""; page.value = "rules";
  password.value = ""; busy.value = ""; resourceBusy.value = false; announcement.value = ""; deleteError.value = ""; resetError.value = "";
  loaded.value = false;
  editorOpen.value = false;
  editorBusy.value = false;
  deletingRule.value = undefined;
  resettingRule.value = undefined;
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
    const signal = requestController.signal;
    const [result, resources] = await Promise.all([
      api.rules(signal, isAdmin ? ownerId.value : ""),
      isAdmin ? api.users(signal) : api.account(signal),
    ]);
    if (version !== requestVersion || !session.value) return;
    rules.value = result;
    if (Array.isArray(resources)) users.value = resources;
    else account.value = resources;
    loaded.value = true;
    listError.value = "";
    lastRefresh.value = new Date();
  } catch (error) {
    if (version !== requestVersion || !session.value) return;
    listError.value = error;
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
      bootError.value = error;
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
  if (!username.value.trim() || !password.value) {
    const missingUsername = !username.value.trim();
    loginError.value = missingUsername ? (isAdmin ? "请输入管理员用户名" : "请输入用户名") : "请输入密码";
    await nextTick();
    (missingUsername ? loginInput.value : passwordInput.value)?.focus();
    return;
  }
  loggingIn.value = true;
  try {
    const result = await api.login(username.value.trim(), password.value);
    sessionVersion++;
    session.value = result;
    setSession(result);
    password.value = "";
    await loadRules();
  } catch (error) {
    loginError.value = error;
  } finally {
    loggingIn.value = false;
  }
}
async function logout(): Promise<void> {
  if (mutationBusy.value) return;
  busy.value = "logout";
  const version = sessionVersion;
  stopListRequest();
  try {
    await api.logout();
    expireSession();
    loginError.value = "";
    username.value = "";
  } catch (error) {
    if (version === sessionVersion && session.value) actionError.value = error;
  } finally {
    if (version === sessionVersion) busy.value = "";
  }
}
function openEditor(rule?: Rule): void {
  editingRule.value = rule ? { ...rule } : undefined;
  editorOwner.value = rule?.owner_id ?? (ownerId.value || session.value?.user_id || "");
  editorOpen.value = true;
}
function mergeRule(rule: Rule): void {
  const index = rules.value.findIndex((item) => item.id === rule.id);
  if (index >= 0) rules.value[index] = rule;
  else if (!isAdmin || !ownerId.value || ownerId.value === rule.owner_id) rules.value.push(rule);
  loaded.value = true;
  lastRefresh.value = new Date();
}
async function save(input: RuleInput): Promise<void> {
  stopListRequest();
  const version = sessionVersion;
  try {
    const result = editingRule.value
      ? await api.update(editingRule.value, input)
      : await api.create(input, editorOwner.value);
    if (version !== sessionVersion) return;
    mergeRule(result);
    notify(editingRule.value ? "规则已更新" : "规则已创建");
  } catch (error) {
    if (version !== sessionVersion) return;
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
    if (version === sessionVersion && session.value) actionError.value = error;
  } finally {
    if (version === sessionVersion) busy.value = "";
  }
}
function confirmReset(rule: Rule): void {
  resettingRule.value = { ...rule };
  resetError.value = "";
}
async function resetTraffic(): Promise<void> {
  const rule = resettingRule.value;
  if (!rule || mutationBusy.value) return;
  busy.value = rule.id;
  resetError.value = "";
  stopListRequest();
  const version = sessionVersion;
  try {
    const result = await api.resetTraffic(rule);
    if (version !== sessionVersion) return;
    mergeRule(result);
    resettingRule.value = undefined;
    notify("流量已重置");
  } catch (error) {
    if (version === sessionVersion && session.value) resetError.value = error;
  } finally {
    if (version === sessionVersion) busy.value = "";
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
    if (version === sessionVersion && session.value) deleteError.value = error;
  } finally {
    if (version === sessionVersion) busy.value = "";
  }
}
function clearFilters(): void {
  ownerId.value = "";
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
    deletingRule.value || resettingRule.value
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

function resourcesChanged(): void { stopListRequest(); void loadRules(); }
watch(page, value => { if (session.value) window.history.replaceState(null, "", `/${props.role}/${value}`); });
watch(session, value => { window.history.replaceState(null, "", `/${props.role}/${value ? page.value : "login"}`); });
watch(ownerId, () => { stopListRequest(); rules.value = []; loaded.value = false; void loadRules(); });

onMounted(() => {
  onUnauthorized(expireSession);
  void initialize();
  timer = setInterval(() => {
    if (
      document.visibilityState === "visible" &&
      !editorOpen.value &&
      !deletingRule.value && !resettingRule.value
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
    <LanguageSwitcher class="standalone-language" />
    <div class="brand-mark"><Icon name="arrows" :size="28" /></div>
    <template v-if="booting">
      <span class="spinner" />
      <p>{{ t("正在连接 Porter…") }}</p>
    </template>
    <template v-else>
      <p role="alert">{{ errorMessage(bootError) }}</p>
      <button class="button primary" @click="initialize">{{ t("重新连接") }}</button>
    </template>
  </div>

  <main v-else-if="!session" class="login-page">
    <section class="login-story" :aria-label="t('Porter 介绍')">
      <a class="brand" :href="isAdmin ? '/admin/' : '/user/'" :aria-label="t('Porter 首页')"
        ><span class="brand-mark"><Icon name="arrows" :size="25" /></span
        ><span>Porter</span></a
      >
      <div class="story-content">
        <h1>{{ t(isAdmin ? '连接更广阔的世界' : '用户后台') }}</h1>
        <p>{{ t(isAdmin ? '端口转发管理后台' : '登录您的账号以管理端口转发服务') }}</p>
        <div class="network-art" aria-hidden="true">
          <svg class="network-lines" viewBox="0 0 600 350" fill="none"><path d="M-20 310C90 180 150 280 260 190S430 30 610 125M-20 330C90 200 150 300 260 210S430 50 610 145M-20 290C90 160 150 260 260 170S430 10 610 105" stroke="#9fcaff" stroke-width="2" /><path d="M80 140 330 280 540 160M20 210 250 340M220 40 500 205" stroke="#d3e6ff" /><circle cx="125" cy="220" r="6" fill="#438aff" /><circle cx="410" cy="105" r="5" fill="#438aff" /><circle cx="475" cy="195" r="5" fill="#438aff" /></svg>
          <div class="network-cube cube-one"><i /><b /><span /></div><div class="network-cube cube-two"><i /><b /><span /></div><div class="network-cube cube-three"><i /><b /><span /></div>
        </div>
      </div>
      <span class="story-footer">{{ t("简单连接 · 轻松掌控") }}</span>
    </section>
    <section class="login-form-area">
      <LanguageSwitcher class="standalone-language" />
      <div class="login-form-wrap">
        <span class="login-mobile-brand"
          >Porter<span class="brand-period">.</span></span
        >
        <h2>{{ t(isAdmin ? "管理员登录" : "用户登录") }}</h2>
        <p class="login-description">{{ t("登录以管理你的端口转发规则") }}</p>
        <form novalidate @submit.prevent="login">
          <div v-if="loginError" role="alert" class="notice error">
            <Icon name="alert" :size="18" /><span>{{ errorMessage(loginError) }}</span>
          </div>
          <div class="field">
            <label for="username">{{ t("用户名") }}</label
            ><input
              id="username"
              ref="loginInput"
              v-model="username"
              autocomplete="username"
              :placeholder="t(isAdmin ? '请输入管理员用户名' : '请输入用户名')"
              required
              :disabled="loggingIn"
            />
          </div>
          <div class="field">
            <label for="password">{{ t("密码") }}</label
            ><input
              id="password"
              ref="passwordInput"
              v-model="password"
              type="password"
              autocomplete="current-password"
              :placeholder="t('请输入密码')"
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
              loggingIn ? t("正在登录…") : t("登录面板")
            }}<Icon v-if="!loggingIn" name="arrow" :size="18" />
          </button>
        </form>
        <div class="login-security">
          <Icon name="shield" :size="16" /><span>{{ t(isAdmin ? "仅限授权管理员访问" : "用户后台") }}</span>
        </div>
      </div>
      <span class="login-footer">{{ t("Porter · 端口转发管理") }}</span>
    </section>
  </main>

  <div v-else class="app-shell">
    <a class="skip-link" href="#main-content">{{ t("跳转到主内容") }}</a>
    <header class="topbar" @keydown.esc="closeNavigation">
      <div class="topbar-inner">
        <a class="brand" :href="isAdmin ? '/admin/' : '/user/'" :aria-label="t('Porter 首页')"
          ><span class="brand-mark"><Icon name="arrows" :size="24" /></span
          ><span>Porter</span></a
        >
        <span class="header-divider" aria-hidden="true" />
        <button
          ref="navigationToggle"
          type="button"
          class="navigation-toggle"
          :aria-label="t(navigationOpen ? '关闭导航' : '打开导航')"
          :aria-expanded="navigationOpen"
          :aria-controls="navigationId"
          @click="navigationOpen = !navigationOpen"
        >
          <Icon :name="navigationOpen ? 'close' : 'menu'" :size="21" />
        </button>
        <nav
          :id="navigationId"
          class="topbar-navigation"
          :class="{ open: navigationOpen }"
          :aria-label="t(isAdmin ? '管理后台' : '用户后台')"
        >
          <button
            v-for="item in navigationPages"
            :key="item.value"
            type="button"
            class="navigation-link"
            :aria-current="page === item.value ? 'page' : undefined"
            :disabled="mutationBusy"
            @click="navigate(item.value)"
          >
            {{ t(item.label) }}
          </button>
        </nav>
        <div class="account">
          <LanguageSwitcher />
          <span class="account-avatar"><Icon name="user" :size="17" /></span
          ><span class="account-name">{{ session.username }}</span
          ><button
            type="button"
            class="logout-button"
            :aria-label="t('退出登录')"
            :disabled="mutationBusy"
            @click="logout"
          >
            <Icon name="logout" :size="17" /><span>{{ t("退出登录") }}</span>
          </button>
        </div>
      </div>
    </header>
    <main id="main-content" class="dashboard" tabindex="-1">
      <AdminResources v-if="isAdmin && page !== 'rules'" :key="page" :page="page" :api="api" @changed="resourcesChanged" @busy="resourceBusy = $event" />
      <AccountOverview v-if="!isAdmin && account" :account="account" :compact="page === 'rules'" />
      <template v-if="page === 'rules'">
      <div class="page-heading">
        <div>
<h1>{{ t("端口转发") }}</h1>
          <p>{{ t("管理端口转发规则，支持 TCP / UDP 协议转发。") }}</p>
        </div>
        <button
          type="button"
          class="button primary"
          :disabled="mutationBusy"
          @click="openEditor()"
        >
          <Icon name="plus" :size="19" />{{ t("新增规则") }}
        </button>
      </div>
      <section class="summary-grid" :aria-label="t('规则概览')">
        <button
          class="summary-card"
          :class="{ active: status === '' }"
          :aria-pressed="status === ''"
          @click="status = ''"
        >
          <span class="summary-icon blue"
            ><Icon name="layers" :size="22" /></span
          ><span class="summary-body"
            ><span>{{ t("全部规则") }}</span
            ><strong
              >{{ loaded ? counts.all : "—" }}<small>{{ t("条", { count: counts.all }) }}</small></strong
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
            ><span>{{ t("运行中") }}</span
            ><strong
              >{{ loaded ? counts.running : "—" }}<small>{{ t("条", { count: counts.running }) }}</small></strong
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
            ><span>{{ t("已停用") }}</span
            ><strong
              >{{ loaded ? counts.disabled : "—" }}<small>{{ t("条", { count: counts.disabled }) }}</small></strong
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
            ><span>{{ t("需要关注") }}</span
            ><strong
              >{{ loaded ? counts.error : "—" }}<small>{{ t("条", { count: counts.error }) }}</small></strong
            ></span
          ><span class="summary-accent amber" />
        </button>
      </section>
      <div v-if="actionError" role="alert" class="notice error dismissible">
        <Icon name="alert" /><span>{{ errorMessage(actionError) }}</span
        ><button
          class="icon-button"
          :aria-label="t('关闭错误提示')"
          @click="actionError = ''"
        >
          <Icon name="close" :size="17" />
        </button>
      </div>
      <section
        class="rules-panel"
        :aria-label="t('转发规则列表')"
        :aria-busy="loading"
      >
        <div class="panel-heading">
          <h2>
            {{ t("转发规则") }}<span class="count-badge">{{
              loaded ? counts.all : "—"
            }}</span>
          </h2>
          <span class="sync-indicator"
            ><span class="live-dot" />{{ t("每 5 秒同步") }}</span
          >
        </div>
        <div class="toolbar">
          <div class="search-box">
            <Icon name="search" :size="18" /><input
              ref="searchInput"
              v-model="search"
              :aria-label="t('搜索规则')"
              :placeholder="t('搜索规则名称、IP 或域名…')"
              type="search"
            /><kbd>/</kbd>
          </div>
          <div class="filters">
            <select v-if="isAdmin" v-model="ownerId" :aria-label="t('按用户筛选')" :disabled="mutationBusy">
              <option value="">{{ t('全部用户') }}</option>
              <option v-for="owner in ownerOptions" :key="owner.id" :value="owner.id">{{ owner.username }}{{ owner.role === 'admin' ? ` (${t('管理员')})` : '' }}</option>
            </select>
            <select v-model="protocol" :aria-label="t('按协议筛选')">
              <option value="">{{ t("全部协议") }}</option>
              <option value="tcp">TCP</option>
              <option value="udp">UDP</option>
              <option value="tcp+udp">TCP+UDP</option>
</select
            ><select v-model="status" :aria-label="t('按状态筛选')">
              <option value="">{{ t("全部状态") }}</option>
              <option value="running">{{ t("运行中") }}</option>
              <option value="disabled">{{ t("已停用") }}</option>
              <option value="error">{{ t("错误") }}</option><option value="quota_exceeded">{{ t("额度已用尽") }}</option>
</select
            ><button
              class="button refresh-button"
              :aria-label="t('刷新规则')"
              :disabled="loading || mutationBusy"
              @click="loadRules"
            >
              <Icon
                name="refresh"
                :size="17"
                :class="{ spinning: loading }"
              /><span>{{ t("刷新") }}</span>
            </button>
          </div>
        </div>
        <div v-if="listError" role="alert" class="notice error list-notice">
          <Icon name="alert" /><span
            >{{ errorMessage(listError)
            }}{{ loaded ? t(" 当前保留上次同步的数据。") : "" }}</span
          ><button
            class="text-button"
            :disabled="loading || mutationBusy"
            @click="loadRules"
          >
            {{ t("重试") }}
          </button>
        </div>
        <div v-if="!loaded" class="empty-state">
          <template v-if="loading">
            <span class="spinner" />
            <h3>{{ t("正在加载规则") }}</h3>
            <p>{{ t("稍等片刻，正在同步你的转发配置。") }}</p>
</template
          ><template v-else>
            <span class="empty-icon"><Icon name="alert" :size="29" /></span>
            <h3>{{ t("暂时无法获取规则") }}</h3>
            <p>{{ t("请检查网络连接，然后重新尝试。") }}</p>
            <button class="button secondary" @click="loadRules">
              {{ t("重新加载") }}
            </button>
          </template>
        </div>
        <template v-else-if="filteredRules.length">
          <div class="desktop-table">
            <table class="rules-table" :class="isAdmin ? 'admin-rules-table' : 'user-rules-table'">
              <colgroup>
                <col class="column-name">
                <col v-if="isAdmin" class="column-owner">
                <col class="column-protocol">
                <col class="column-listen">
                <col class="column-target">
                <col class="column-status">
                <col class="column-traffic">
                <col class="column-speed">
                <col class="column-operations">
              </colgroup>
              <thead>
                <tr>
                  <th scope="col" class="name-heading">{{ t("规则名称") }}</th>
                  <th v-if="isAdmin" scope="col">{{ t("所属用户") }}</th>
                  <th scope="col">{{ t("协议") }}</th>
                  <th scope="col">{{ t("监听地址") }}</th>
                  <th scope="col">{{ t("转发目标") }}</th>
                  <th scope="col">{{ t("运行状态") }}</th>
                  <th scope="col">{{ t("累计流量") }}</th>
                  <th scope="col">{{ t("当前速度") }}</th>
                  <th scope="col" class="operation-heading">{{ t("操作") }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="rule in filteredRules" :key="rule.id">
                  <td class="rule-name-cell">
                    <span
                      class="rule-avatar"
                      :class="protocolDetails[rule.protocol].className"
                      ><Icon
                        :name="protocolDetails[rule.protocol].icon"
                        :size="18" /></span
                    ><strong :title="rule.name">{{ rule.name }}</strong>
                  </td>
                  <td v-if="isAdmin" class="owner-cell"><span :title="rule.owner?.username">{{ rule.owner?.username }}</span></td>
                  <td>
                    <span
                      class="protocol-badge"
                      :class="protocolDetails[rule.protocol].className"
                      >{{ protocolDetails[rule.protocol].label }}</span
                    >
                  </td>
                  <td class="address-cell">
                    <code
                      :title="address(rule.listen_host, rule.listen_port)"
                      >{{ address(rule.listen_host, rule.listen_port) }}</code
                    >
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
                          ? t('本机监听正常，不代表目标服务始终可达')
                          : undefined
                      "
                      ><i />{{ t(statusLabels[rule.status]) }}</span
                    ><span
                      v-if="rule.last_error && rule.status === 'error'"
                      class="rule-error"
                      :title="ruleErrorMessage(rule)"
                      >{{ ruleErrorMessage(rule) }}</span
                    >
                  </td>
                  <td class="traffic-cell"><RuleTraffic :rule="rule" :busy="mutationBusy" mode="totals" @reset="confirmReset" /></td>
                  <td class="speed-cell"><RuleTraffic :rule="rule" :busy="mutationBusy" mode="speed" /></td>
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
                  <span
                    class="rule-avatar"
                    :class="protocolDetails[rule.protocol].className"
                    ><Icon
                      :name="protocolDetails[rule.protocol].icon"
                      :size="17" /></span
                  >{{ rule.name }}
                </h3>
                <span
                  class="protocol-badge"
                  :class="protocolDetails[rule.protocol].className"
                  >{{ protocolDetails[rule.protocol].label }}</span
                >
              </div>
              <p v-if="isAdmin" class="owner-label">{{ t("所属用户") }}: {{ rule.owner?.username }}</p>
              <div class="mobile-route">
                <div>
                  <span>{{ t("监听地址") }}</span
                  ><code>{{
                    address(rule.listen_host, rule.listen_port)
                  }}</code>
                </div>
                <Icon name="arrow" :size="18" />
                <div>
                  <span>{{ t("转发目标") }}</span
                  ><code>{{
                    address(rule.target_host, rule.target_port)
                  }}</code>
                </div>
              </div>
              <p
                v-if="rule.last_error && rule.status === 'error'"
                class="mobile-rule-error"
              >
                {{ ruleErrorMessage(rule) }}
              </p>
              <RuleTraffic :rule="rule" :busy="mutationBusy" @reset="confirmReset" />
              <div class="rule-card-footer">
                <span class="status-badge" :class="rule.status"
                  ><i />{{ t(statusLabels[rule.status]) }}</span
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
          <h3>{{ hasFilters ? t("没有找到匹配的规则") : t("从第一条规则开始") }}</h3>
          <p>
            {{
              hasFilters
                ? t("试试其他关键词，或调整筛选条件。")
                : t("添加本机监听端口和目标地址，即可开始转发。")
            }}
          </p>
          <button
            v-if="hasFilters"
            class="button secondary"
            @click="clearFilters"
          >
            {{ t("清除筛选") }}
</button
          ><button
            v-else
            class="button primary"
            :disabled="mutationBusy"
            @click="openEditor()"
          >
            <Icon name="plus" :size="18" />{{ t("新增规则") }}
          </button>
        </div>
        <footer class="panel-footer">
          <span>{{
            hasFilters
              ? t("显示 {shown} 条，共 {total} 条规则", { shown: filteredRules.length, total: counts.all })
              : t("共 {total} 条规则", { total: loaded ? counts.all : "—" })
          }}</span
          ><span>{{ t("最后同步 {time}", { time: refreshingText }) }}</span>
        </footer>
      </section>
      <div class="dashboard-note">
        <Icon name="shield" :size="16" />
        <p>{{ t("“运行中”表示本机监听正常。停用规则会立即断开现有会话。") }}</p>
      </div>
      </template>
    </main>
    <footer class="app-footer">
      <span>Porter<span class="brand-period">.</span></span
      ><span>{{ t("让连接井然有序") }}</span><span>TCP / UDP</span>
    </footer>
    <Transition name="toast">
      <div v-if="announcement" role="status" class="toast-notice">
        <span class="toast-icon"><Icon name="check" :size="17" /></span
        >{{ t(announcement) }}
      </div>
    </Transition>
    <RuleEditor
      v-if="editorOpen"
      :rule="editingRule"
      :save="save"
      :owner-options="isAdmin ? ownerOptions : undefined"
      :owner-id="editorOwner"
      :port-start="editorAccount?.user.port_start"
      :port-end="editorAccount?.user.port_end"
      @owner="editorOwner = $event"
      @close="editorOpen = false"
      @busy="editorBusy = $event"
    />
    <Modal v-if="resettingRule" :label="t('重置流量')" :busy="Boolean(busy)" @close="resettingRule = undefined">
      <div class="delete-dialog">
        <h2>{{ t("重置流量？") }}</h2>
        <p>{{ t("确定清空 {name} 的上下行累计流量吗？", { name: resettingRule.name }) }}</p>
        <p class="delete-warning">{{ t("此操作无法撤销，不会影响转发连接或统计开关。") }}</p>
        <div v-if="resetError" role="alert" class="notice error">{{ errorMessage(resetError) }}</div>
        <div class="delete-actions">
          <button class="button secondary" data-autofocus :disabled="Boolean(busy)" @click="resettingRule = undefined">{{ t("取消") }}</button>
          <button class="button primary" :disabled="Boolean(busy)" @click="resetTraffic">{{ busy ? t("正在重置…") : t("确认重置") }}</button>
        </div>
      </div>
    </Modal>
    <Modal
      v-if="deletingRule"
      :label="t('删除规则')"
      :busy="Boolean(busy)"
      @close="deletingRule = undefined"
    >
      <div class="delete-dialog">
        <span class="delete-icon"><Icon name="trash" :size="26" /></span>
        <h2>{{ t("删除这条规则？") }}</h2>
        <p>
          {{ t("确定删除 {name} 吗？", { name: deletingRule.name }) }}
        </p>
        <code
          >{{ address(deletingRule.listen_host, deletingRule.listen_port) }} →
          {{
            address(deletingRule.target_host, deletingRule.target_port)
          }}</code
        >
        <p class="delete-warning">
          {{ t("转发将立即停止，现有会话会被断开。此操作无法撤销。") }}
        </p>
        <div v-if="deleteError" role="alert" class="notice error">
          {{ errorMessage(deleteError) }}
        </div>
        <div class="delete-actions">
          <button
            class="button secondary"
            data-autofocus
            :disabled="Boolean(busy)"
            @click="deletingRule = undefined"
          >
            {{ t("保留规则") }}
</button
          ><button
            class="button danger"
            :disabled="Boolean(busy)"
            @click="remove"
          >
            <span v-if="busy" class="spinner small" />{{
              busy ? t("正在删除…") : t("确认删除")
            }}
          </button>
        </div>
      </div>
    </Modal>
  </div>
</template>
