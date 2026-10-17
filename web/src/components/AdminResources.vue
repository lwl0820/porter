<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref } from "vue";
import { ApiError, errorMessage, type ApiClient } from "../api";
import { knownMessage, t, type MessageKey } from "../i18n";
import { accountingLabels, resetLabels, type Account, type FieldErrors, type Plan, type PlanInput, type Settings } from "../types";
import { formatBytes, gibToBytes } from "../traffic";
import AccountOverview from "./AccountOverview.vue";
const props = defineProps<{ page: string; api: ApiClient }>();
const emit = defineEmits<{ changed: []; busy: [value: boolean] }>();
const plans = ref<Plan[]>([]);
const users = ref<Account[]>([]);
const settings = ref<Settings>();
const error = ref<unknown>();
const success = ref<MessageKey>();
const fields = ref<FieldErrors>({});
const loading = ref(true);
const busy = ref(false);
const formElement = ref<HTMLFormElement>();
const controller = new AbortController();
let version = 0;
let disposed = false;
let timer: ReturnType<typeof setInterval> | undefined;
const plan = reactive<PlanInput>({ name: "", port_count: 10, traffic_limit_bytes: "", accounting_mode: "both", reset_mode: "calendar_month", reset_day: null });
const gib = ref("100");
const resetDay = ref(1);
const user = reactive({ username: "", password: "", plan_id: "" });
const selectedPlan = computed(() => plans.value.find(item => item.id === user.plan_id));
const pool = reactive({ port_start: 10000, port_end: 20000 });
const pageLabels: Record<string, MessageKey> = { plans: "套餐管理", users: "用户管理", settings: "系统配置" };
async function load(force = false): Promise<void> {
 if (disposed || busy.value || document.visibilityState === "hidden") return;
 const token = ++version;
 try {
  if (props.page === "settings") {
   const result = await props.api.settings(controller.signal);
   if (token !== version || disposed) return;
   // 自动读取保留草稿；用户显式刷新时重新载入配置及版本。
   if (!settings.value || force) { settings.value = result; pool.port_start = result.port_start || 10000; pool.port_end = result.port_end || 20000; }
  } else {
   const [p, u] = await Promise.all([props.api.plans(controller.signal), props.page === "users" ? props.api.users(controller.signal) : Promise.resolve([])]);
   if (token !== version || disposed) return;
   plans.value = p; users.value = u;
   if (!user.plan_id && p[0]) user.plan_id = p[0].id;
  }
  error.value = undefined;
 } catch (failure) { if (token === version && !disposed) error.value = failure; }
 finally { if (token === version && !disposed) loading.value = false; }
}
async function save(): Promise<void> {
 if (busy.value) return;
 error.value = undefined; success.value = undefined; fields.value = {};
 if (props.page === "plans") {
  if (!plan.name.trim() || [...plan.name.trim()].length > 80) fields.value.name = "名称需要 1–80 个字符";
  if (!Number.isInteger(plan.port_count) || plan.port_count < 1 || plan.port_count > 65535) fields.value.port_count = "端口数量范围为 1–65535";
  if (plan.reset_mode === "fixed_day" && (!Number.isInteger(resetDay.value) || resetDay.value < 1 || resetDay.value > 31)) fields.value.reset_day = "固定日范围为 1–31";
 } else if (props.page === "users") {
  if (!user.username.trim() || [...user.username.trim()].length > 80) fields.value.username = "用户名需要 1–80 个字符";
  const length = new TextEncoder().encode(user.password).length;
  if (length < 8 || length > 1024) fields.value.password = "密码需要 8–1024 字节";
  if (!user.plan_id) fields.value.plan_id = "请选择套餐";
 } else {
  if (!Number.isInteger(pool.port_start) || pool.port_start < 1 || pool.port_start > 65535) fields.value.port_start = "端口范围为 1–65535";
  if (!Number.isInteger(pool.port_end) || pool.port_end < pool.port_start || pool.port_end > 65535) fields.value.port_end = "结束端口必须不小于起始端口，且不超过 65535";
 }
 if (props.page === "plans" && !gibToBytes(gib.value)) { fields.value.traffic_limit_bytes = "请输入可精确换算为整数字节的正数 GiB。"; }
 if (Object.keys(fields.value).length) { await focusError(); return; }
 busy.value = true; emit("busy", true); version++;
 try {
  if (props.page === "plans") {
   const result = await props.api.createPlan({ ...plan, traffic_limit_bytes: gibToBytes(gib.value)!, reset_day: plan.reset_mode === "fixed_day" ? resetDay.value : null });
   if (disposed) return;
   plans.value.push(result); plan.name = ""; success.value = "套餐已创建";
  } else if (props.page === "users") {
   const result = await props.api.createUser({ ...user });
   if (disposed) return;
   users.value.push(result); user.username = ""; user.password = ""; success.value = "用户已创建，端口已分配";
  } else if (settings.value) {
   const result = await props.api.saveSettings({ ...pool }, settings.value.revision);
   if (disposed) return;
   settings.value = result; success.value = "系统配置已保存";
  }

 } catch (failure) {
  if (disposed) return;
  error.value = failure;
  if (failure instanceof ApiError) fields.value = failure.fields;
 } finally { busy.value = false; emit("busy", false); }
 if (success.value) emit("changed");
 await focusError();
}
async function focusError(): Promise<void> { await nextTick(); formElement.value?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus(); }
function hint(key: string): string {
 const reason = fields.value[key];
 return reason ? (knownMessage(reason) ?? t("请检查此字段")) : "";
}
function visible(): void { if (document.visibilityState === "visible") void load(); }
onMounted(() => { void load(); timer = setInterval(() => { if (props.page !== "settings") void load(); }, 5000); document.addEventListener("visibilitychange", visible); });
onUnmounted(() => { disposed = true; version++; controller.abort(); clearInterval(timer); user.password = ""; emit("busy", false); document.removeEventListener("visibilitychange", visible); });
</script>
<template>
 <section class="resources-page">
  <header class="page-heading"><h1>{{ t(pageLabels[page] ?? '系统配置') }}</h1><button class="button secondary" :disabled="busy" @click="load(true)">{{ t('刷新') }}</button></header>
  <p v-if="error" role="alert" class="notice error">{{ errorMessage(error) }}</p>
  <p v-if="success" role="status" class="notice subtle">{{ t(success) }}</p>
  <p v-if="loading">{{ t('正在加载…') }}</p>
  <form v-else ref="formElement" class="resource-form" novalidate @submit.prevent="save">
   <fieldset :disabled="busy">
    <template v-if="page === 'plans'">
     <div class="field"><label for="plan-name">{{ t('套餐名称') }}</label><input id="plan-name" v-model="plan.name" required maxlength="80" :aria-invalid="Boolean(fields.name)"><p v-if="fields.name" class="field-error">{{ hint('name') }}</p></div>
     <div class="field"><label for="port-count">{{ t('端口数量') }}</label><input id="port-count" v-model.number="plan.port_count" type="number" min="1" max="65535" required :aria-invalid="Boolean(fields.port_count)"><p v-if="fields.port_count" class="field-error">{{ hint('port_count') }}</p></div>
     <div class="field"><label for="traffic-limit">{{ t('流量额度（GiB）') }}</label><input id="traffic-limit" v-model="gib" inputmode="decimal" required :aria-invalid="Boolean(fields.traffic_limit_bytes)"><p v-if="fields.traffic_limit_bytes" class="field-error">{{ hint('traffic_limit_bytes') }}</p></div>
     <div class="field"><label for="accounting-mode">{{ t('流量计费') }}</label><select id="accounting-mode" v-model="plan.accounting_mode"><option v-for="(label, key) in accountingLabels" :key="key" :value="key">{{ t(label) }}</option></select></div>
     <div class="field"><label for="reset-mode">{{ t('重置方式') }}</label><select id="reset-mode" v-model="plan.reset_mode"><option v-for="(label, key) in resetLabels" :key="key" :value="key">{{ t(label) }}</option></select></div>
     <div v-if="plan.reset_mode === 'fixed_day'" class="field"><label for="reset-day">{{ t('每月重置日') }}</label><input id="reset-day" v-model.number="resetDay" type="number" min="1" max="31" required :aria-invalid="Boolean(fields.reset_day)"><p v-if="fields.reset_day" class="field-error">{{ hint('reset_day') }}</p></div>
     <p class="field-hint">{{ t('统一 UTC 00:00 重置；当月无对应日期时在月末重置，后续恢复原定日期。') }}</p>
    </template>
    <template v-else-if="page === 'users'">
     <div class="field"><label for="new-username">{{ t('用户名') }}</label><input id="new-username" v-model="user.username" required autocomplete="off" :aria-invalid="Boolean(fields.username)"><p v-if="fields.username" class="field-error">{{ hint('username') }}</p></div>
     <div class="field"><label for="new-password">{{ t('密码') }}</label><input id="new-password" v-model="user.password" required type="password" autocomplete="new-password" :aria-invalid="Boolean(fields.password)"><p v-if="fields.password" class="field-error">{{ hint('password') }}</p></div>
     <div class="field"><label for="user-plan">{{ t('选择套餐') }}</label><select id="user-plan" v-model="user.plan_id" required :aria-invalid="Boolean(fields.plan_id)"><option value="">{{ t('选择套餐') }}</option><option v-for="item in plans" :key="item.id" :value="item.id">{{ item.name }}</option></select><p v-if="fields.plan_id" class="field-error">{{ hint('plan_id') }}</p></div>
     <p v-if="selectedPlan" class="notice subtle">{{ selectedPlan.port_count }} {{ t('个端口') }} · {{ formatBytes(selectedPlan.traffic_limit_bytes) }} · {{ t(accountingLabels[selectedPlan.accounting_mode]) }}</p>
    </template>
    <template v-else>
     <div class="field"><label for="pool-start">{{ t('分配起始端口') }}</label><input id="pool-start" v-model.number="pool.port_start" type="number" min="1" max="65535" required :aria-invalid="Boolean(fields.port_start)"><p v-if="fields.port_start" class="field-error">{{ hint('port_start') }}</p></div>
     <div class="field"><label for="pool-end">{{ t('分配结束端口') }}</label><input id="pool-end" v-model.number="pool.port_end" type="number" :min="pool.port_start" max="65535" required :aria-invalid="Boolean(fields.port_end)"><p v-if="fields.port_end" class="field-error">{{ hint('port_end') }}</p></div>
     <p class="field-hint">{{ t('范围变更只影响新用户，已有用户保留原端口段。') }}</p>
    </template>
    <button class="button primary" type="submit" :disabled="busy || (page === 'users' && !plans.length) || (page === 'settings' && !settings)">{{ busy ? t('正在保存…') : t(page === 'plans' ? '添加套餐' : page === 'users' ? '添加用户' : '保存配置') }}</button>
   </fieldset>
  </form>
  <div v-if="page === 'plans'" class="plan-list">
   <article v-for="item in plans" :key="item.id" class="plan-card"><h2>{{ item.name }}</h2><p :title="`${item.traffic_limit_bytes} B`">{{ item.port_count }} {{ t('个端口') }} · {{ formatBytes(item.traffic_limit_bytes) }}</p><p>{{ t(accountingLabels[item.accounting_mode]) }} · {{ t(resetLabels[item.reset_mode]) }}{{ item.reset_day ? ` · ${item.reset_day}` : '' }}</p></article>
  </div>
  <div v-if="page === 'users'" class="user-list"><AccountOverview v-for="item in users" :key="item.user.id" :account="item" /></div>
 </section>
</template>
