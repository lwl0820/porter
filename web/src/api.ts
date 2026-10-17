import { knownMessage, t, type MessageKey } from "./i18n";
import { isProtocol } from "./types";
import type { Account, Plan, PlanInput, Role, Settings, FieldErrors, Rule, RuleInput, Session } from "./types";

export class ApiError extends Error {
  constructor(
    message: string,
    public status = 0,
    public code = "",
    public fields: FieldErrors = {},
  ) {
    super(message);
    this.name = "ApiError";
  }
}

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function decodeSession(value: unknown, role: Role): Session {
  if (
    !record(value) ||
    typeof value.user_id !== "string" || value.role !== role ||
    typeof value.username !== "string" ||
    typeof value.csrf_token !== "string"
  ) {
    throw new ApiError("登录信息无效，请重新登录。", 0, "invalid_session");
  }
  return value as unknown as Session;
}

function byteCount(value: unknown): boolean {
  return typeof value === "string" && /^(0|[1-9][0-9]*)$/.test(value) && value.length <= 19 && BigInt(value) <= 9223372036854775807n;
}
function speed(value: unknown): boolean {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}

function decodeRule(value: unknown): Rule {
  if (
    !record(value) ||
    typeof value.owner_id !== "string" ||
    typeof value.id !== "string" ||
    typeof value.name !== "string" ||
    !isProtocol(value.protocol) ||
    typeof value.listen_host !== "string" ||
    typeof value.target_host !== "string" ||
    !Number.isInteger(value.listen_port) ||
    !Number.isInteger(value.target_port) ||
    typeof value.enabled !== "boolean" ||
    typeof value.traffic_enabled !== "boolean" ||
    !record(value.traffic) ||
    !byteCount(value.traffic.upload_bytes) ||
    !byteCount(value.traffic.download_bytes) ||
    !speed(value.traffic.upload_bytes_per_second) ||
    !speed(value.traffic.download_bytes_per_second) ||
    !Number.isInteger(value.revision) ||
    typeof value.created_at !== "string" ||
    typeof value.updated_at !== "string" ||
    !["running", "disabled", "error", "quota_exceeded"].includes(String(value.status)) ||
    (value.last_error !== undefined && typeof value.last_error !== "string") ||
    (value.last_error_code !== undefined &&
      typeof value.last_error_code !== "string")
  ) {
    throw new ApiError("服务器返回的规则格式无效，请刷新重试。", 0, "invalid_rule");
  }
  return value as unknown as Rule;
}

function invalid(): never { throw new ApiError("服务器返回的数据格式无效，请刷新重试。", 0, "invalid_response"); }
function date(value: unknown): boolean { return typeof value === "string" && value.endsWith("Z") && Number.isFinite(Date.parse(value)); }
function integer(value: unknown, min = 1, max = 65535): boolean { return typeof value === "number" && Number.isInteger(value) && value >= min && value <= max; }
function decodePlan(value: unknown): Plan {
 if (!record(value) || typeof value.id !== "string" || typeof value.name !== "string" || !integer(value.port_count) || !byteCount(value.traffic_limit_bytes) || value.traffic_limit_bytes === "0" || !["both", "max", "upload", "download"].includes(String(value.accounting_mode)) || !["calendar_month", "anniversary", "fixed_day"].includes(String(value.reset_mode)) || (value.reset_mode === "fixed_day" ? !integer(value.reset_day, 1, 31) : value.reset_day !== null) || !date(value.created_at)) invalid();
 return value as unknown as Plan;
}
function decodeAccount(value: unknown): Account {
 if (!record(value) || !record(value.user)) invalid();
 const u = value.user;
 if (typeof u.id !== "string" || typeof u.username !== "string" || !["admin", "user"].includes(String(u.role)) || !date(u.created_at) || (u.role === "user" ? typeof u.plan_id !== "string" || !integer(u.port_start) || !integer(u.port_end) || Number(u.port_start) > Number(u.port_end) : u.plan_id !== null || u.port_start !== 0 || u.port_end !== 0)) invalid();
 if (value.plan !== null) decodePlan(value.plan);
 if (value.usage !== null) {
  const usage = value.usage;
  if (!record(usage) || !date(usage.cycle_start) || !date(usage.cycle_end) || typeof usage.exceeded !== "boolean" || !["upload_bytes", "download_bytes", "used_bytes", "limit_bytes"].every(key => byteCount(usage[key]))) invalid();
 }
 if (u.role === "user" && (value.plan === null || value.usage === null)) invalid();
 return value as unknown as Account;
}
function decodeSettings(value: unknown): Settings {
 if (!record(value) || !integer(value.revision, 1, Number.MAX_SAFE_INTEGER) || !(value.port_start === 0 && value.port_end === 0) && (!integer(value.port_start) || !integer(value.port_end) || Number(value.port_start) > Number(value.port_end))) invalid();
 return value as unknown as Settings;
}
function decodeList<T>(body: unknown, key: string, decode: (value: unknown) => T): T[] {
 if (!record(body) || !Array.isArray(body[key])) invalid();
 return body[key].map(decode);
}

export function createClient(role: Role) {
let csrf = "";
let unauthorized: (() => void) | undefined;
let generation = 0;
function setSession(session?: Session): void { csrf = session?.csrf_token ?? ""; generation++; }
function onUnauthorized(handler?: () => void): void { unauthorized = handler; }
function decodeOwnedRule(value: unknown): Rule {
  const result = decodeRule(value);
  if (role === "admin" && (!record(result.owner) || result.owner.id !== result.owner_id || typeof result.owner.username !== "string" || !["admin", "user"].includes(result.owner.role))) invalid();
  return result;
}
async function request(
  path: string,
  options: RequestInit = {},
): Promise<unknown> {
  const requestGeneration = generation;
  const headers = new Headers(options.headers);
  if (options.body) headers.set("Content-Type", "application/json");
  if (options.method && options.method !== "GET" && csrf)
    headers.set("X-CSRF-Token", csrf);
  let response: Response;
  try {
    response = await fetch(`/api/${role}${path}`, {
      ...options,
      headers,
      credentials: "same-origin",
      cache: "no-store",
    });
  } catch (error) {
    if (error instanceof Error && error.name === "AbortError") throw error;
    throw new ApiError("无法连接服务器，请检查网络后重试。", 0, "network_error");
  }
  if (response.status === 204) return undefined;
  const body: unknown = await response.json().catch(() => undefined);
  if (!response.ok) {
    if (response.status === 401 && path !== "/auth/login" && generation === requestGeneration) unauthorized?.();
    const failure = record(body) && record(body.error) ? body.error : {};
    const fields: FieldErrors = {};
    if (record(failure.fields)) {
      for (const key of Object.keys(failure.fields)) {
        const message = failure.fields[key];
        if (typeof message === "string") fields[key] = message;
      }
    }
    const fallback =
      response.status === 412
        ? path === "/settings" ? "配置已被修改，请刷新后重试" : "规则已在其他页面更新，请关闭表单并刷新后重新编辑。"
        : "操作失败，请稍后重试。";
    throw new ApiError(
      response.status === 412
        ? fallback
        : typeof failure.message === "string"
          ? failure.message
          : fallback,
      response.status,
      typeof failure.code === "string" ? failure.code : "",
      fields,
    );
  }
  return body;
}

const revisionHeader = (revision: number) => ({ "If-Match": `"${revision}"` });
return {
  setSession, onUnauthorized,
  async session(): Promise<Session> {
    return decodeSession(await request("/auth/session"), role);
  },
  async login(username: string, password: string): Promise<Session> {
    return decodeSession(
      await request("/auth/login", {
        method: "POST",
        body: JSON.stringify({ username, password }),
      }), role,
    );
  },
  async logout(): Promise<void> {
    await request("/auth/logout", { method: "POST" });
  },
  async rules(signal?: AbortSignal, ownerId = ""): Promise<Rule[]> {
    const body = await request(`/rules${ownerId ? `?owner_id=${encodeURIComponent(ownerId)}` : ""}`, { signal });
    if (!record(body) || !Array.isArray(body.rules))
      throw new ApiError("服务器返回的规则列表无效，请刷新重试。", 0, "invalid_rules");
    return body.rules.map(decodeOwnedRule);
  },
  async create(input: RuleInput, ownerId?: string): Promise<Rule> {
    return decodeOwnedRule(
      await request("/rules", { method: "POST", body: JSON.stringify(role === "admin" ? { ...input, owner_id: ownerId } : input) }),
    );
  },
  async update(rule: Rule, input: RuleInput): Promise<Rule> {
    return decodeOwnedRule(
      await request(`/rules/${encodeURIComponent(rule.id)}`, {
        method: "PUT",
        headers: revisionHeader(rule.revision),
        body: JSON.stringify(input),
      }),
    );
  },
  async remove(rule: Rule): Promise<void> {
    await request(`/rules/${encodeURIComponent(rule.id)}`, {
      method: "DELETE",
      headers: revisionHeader(rule.revision),
    });
  },
  async resetTraffic(rule: Rule): Promise<Rule> {
    return decodeOwnedRule(await request(`/rules/${encodeURIComponent(rule.id)}/traffic/reset`, {
      method: "POST", headers: revisionHeader(rule.revision),
    }));
  },
  async retry(value: Rule): Promise<Rule> {
    return decodeOwnedRule(
      await request(`/rules/${encodeURIComponent(value.id)}/retry`, {
        method: "POST",
        headers: revisionHeader(value.revision),
      }),
    );
  },
  async plans(signal?: AbortSignal): Promise<Plan[]> { return decodeList(await request("/plans", { signal }), "plans", decodePlan); },
  async users(signal?: AbortSignal): Promise<Account[]> { return decodeList(await request("/users", { signal }), "users", decodeAccount); },
  async account(signal?: AbortSignal): Promise<Account> { return decodeAccount(await request("/account", { signal })); },
  async settings(signal?: AbortSignal): Promise<Settings> { return decodeSettings(await request("/settings", { signal })); },
  async createPlan(input: PlanInput): Promise<Plan> { return decodePlan(await request("/plans", { method: "POST", body: JSON.stringify(input) })); },
  async createUser(input: { username: string; password: string; plan_id: string }): Promise<Account> { return decodeAccount(await request("/users", { method: "POST", body: JSON.stringify(input) })); },
  async saveSettings(input: { port_start: number; port_end: number }, revision: number): Promise<Settings> { return decodeSettings(await request("/settings", { method: "PUT", body: JSON.stringify(input), headers: revisionHeader(revision) })); },
};
}
export const adminApi = createClient("admin");
export const userApi = createClient("user");
export type ApiClient = ReturnType<typeof createClient>;

// 错误对象保留到渲染时再翻译，切换语言不会清除提示或丢失原因。
const errorKeys: Record<string, MessageKey> = {
  port_reserved: "数字端口已分配给用户",
  port_pool_unset: "请先配置用户端口分配范围",
  port_exhausted: "没有足够的连续可用端口",
  name_conflict: "名称已存在",
  forbidden: "无权操作",
  invalid_response: "服务器返回的数据格式无效，请刷新重试。",
  quota_exceeded: "套餐流量已用尽，将在下次重置后自动恢复。",
  invalid_session: "登录信息无效，请重新登录。",
  invalid_rule: "服务器返回的规则格式无效，请刷新重试。",
  invalid_rules: "服务器返回的规则列表无效，请刷新重试。",
  network_error: "无法连接服务器，请检查网络后重试。",
  credentials_invalid: "用户名或密码错误",
  credentials: "用户名或密码错误",
  unauthorized: "登录已过期，请重新登录。",
  csrf_invalid: "请求校验失败，请刷新页面后重试",
  origin_invalid: "请求来源无效",
  login_limited: "登录尝试过于频繁，请稍后重试",
  validation_error: "请检查表单内容",
  revision_required: "请提供当前规则版本",
  revision_invalid: "规则版本无效",
  json_required: "请求必须使用 JSON",
  invalid_json: "请求格式或字段无效",
  not_found: "规则不存在",
  internal_error: "操作失败，请稍后重试。",
  shutting_down: "服务正在停止",
  rule_limit: "最多保存 1000 条规则",
  disabled: "请先启用规则",
  admin_conflict: "TCP 监听地址与面板管理端口冲突",
  forward_loop: "目标不能指向规则自身的监听地址",
};

function listenerMessage(message: string): string | undefined {
  const suffix = "；更改重叠地址时请先停用规则";
  const reason = message.endsWith(suffix) ? message.slice(0, -suffix.length) : message;
  const protocol = /^(TCP|UDP|TCP\+UDP) 监听失败，请检查地址、端口占用和权限$/.exec(reason)?.[1];
  const translated = protocol
    ? t("{protocol} 监听失败，请检查地址、端口占用和权限", { protocol })
    : knownMessage(reason);
  if (translated) return translated + (message.endsWith(suffix) ? t("；更改重叠地址时请先停用规则") : "");
  return undefined;
}

export function errorMessage(error: unknown): string {
  if (typeof error === "string") return knownMessage(error) ?? t("操作失败，请稍后重试。");
  if (error instanceof ApiError) {
    if (error.status === 412 || error.code === "revision_mismatch")
      return error.message === "配置已被修改，请刷新后重试" ? t("配置已被修改，请刷新后重试") : t("规则已在其他页面更新，请关闭表单并刷新后重新编辑。");
    if (error.code === "rule_conflict") {
      const protocol = /^(TCP|UDP|TCP\+UDP) 监听地址和端口已被规则使用$/.exec(error.message)?.[1];
      if (protocol) return t("{protocol} 监听地址和端口已被规则使用", { protocol });
    }
    if (error.code === "listen_conflict")
      return listenerMessage(error.message) ?? t("监听失败，请检查地址、端口占用和权限");
    if (error.code === "database_error")
      return knownMessage(error.message) ?? t("数据库操作失败，原数据保持不变");
    const key = errorKeys[error.code];
    if (key) {
      if (["invalid_json", "validation_error", "not_found"].includes(error.code))
        return knownMessage(error.message) ?? t(key);
      return t(key);
    }
    return knownMessage(error.message) ?? t("操作失败，请稍后重试。");
  }
  return t("操作失败，请稍后重试。");
}

export function fieldErrorMessage(field: keyof RuleInput, message?: string): string {
  if (!message) return "";
  // 字段与原因共同映射，未知原因不冒充格式或端口范围错误。
  const reasons: Record<keyof RuleInput, readonly MessageKey[]> = {
    name: ["请输入 1–80 个字符的规则名称。", "名称需要 1–80 个字符"],
    protocol: ["请选择 TCP、UDP 或 TCP+UDP"],
    listen_host: ["请输入有效的 IPv4 或 IPv6 地址。", "请输入本机 IPv4 或 IPv6 地址"],
    listen_port: ["请输入 1–65535 的整数端口。", "端口范围为 1–65535", "监听端口已占用", "该监听端口已被占用", "UDP 监听端口已被占用", "监听端口必须在分配范围内", "请选择用户分配范围内的端口"],
    target_host: ["请输入 IP 地址或域名，不含协议、路径或端口。", "目标必须是单播地址或域名", "请输入 IP 或域名，不包含协议、路径或端口"],
    target_port: ["请输入 1–65535 的整数端口。", "端口范围为 1–65535"],
    enabled: ["启用状态不能为空"],
    traffic_enabled: [],
  };
  const reason = reasons[field].find((key) => key === message);
  return reason ? t(reason) : t("请检查此字段");
}

export function ruleErrorMessage(rule: Rule): string {
  if (rule.last_error_code === "quota_reset_pending") return t("流量周期重置暂未完成，正在重试");
  return listenerMessage(rule.last_error ?? "")
    ?? (rule.last_error_code === "listen_failed"
      ? t("{protocol} 监听失败，请检查地址、端口占用和权限", { protocol: rule.protocol.toUpperCase() })
      : t("监听失败，请检查地址、端口占用和权限"));
}
