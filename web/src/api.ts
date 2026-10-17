import { knownMessage, t, type MessageKey } from "./i18n";
import { isProtocol } from "./types";
import type { FieldErrors, Rule, RuleInput, Session } from "./types";

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

let csrf = "";
let unauthorized: (() => void) | undefined;

export function setSession(session?: Session): void {
  csrf = session?.csrf_token ?? "";
}
export function onUnauthorized(handler?: () => void): void {
  unauthorized = handler;
}

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function decodeSession(value: unknown): Session {
  if (
    !record(value) ||
    typeof value.username !== "string" ||
    typeof value.csrf_token !== "string"
  ) {
    throw new ApiError("登录信息无效，请重新登录。", 0, "invalid_session");
  }
  return { username: value.username, csrf_token: value.csrf_token };
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
    !["running", "disabled", "error"].includes(String(value.status)) ||
    (value.last_error !== undefined && typeof value.last_error !== "string") ||
    (value.last_error_code !== undefined &&
      typeof value.last_error_code !== "string")
  ) {
    throw new ApiError("服务器返回的规则格式无效，请刷新重试。", 0, "invalid_rule");
  }
  return value as unknown as Rule;
}

async function request(
  path: string,
  options: RequestInit = {},
): Promise<unknown> {
  const headers = new Headers(options.headers);
  if (options.body) headers.set("Content-Type", "application/json");
  if (options.method && options.method !== "GET" && csrf)
    headers.set("X-CSRF-Token", csrf);
  let response: Response;
  try {
    response = await fetch(`/api${path}`, {
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
    if (response.status === 401 && path !== "/auth/login") unauthorized?.();
    const failure = record(body) && record(body.error) ? body.error : {};
    const fields: FieldErrors = {};
    if (record(failure.fields)) {
      for (const key of [
        "name",
        "protocol",
        "listen_host",
        "listen_port",
        "target_host",
        "target_port",
        "enabled",
        "traffic_enabled",
      ] as const) {
        const message = failure.fields[key];
        if (typeof message === "string") fields[key] = message;
      }
    }
    const fallback =
      response.status === 412
        ? "规则已在其他页面更新，请关闭表单并刷新后重新编辑。"
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
export const api = {
  async session(): Promise<Session> {
    return decodeSession(await request("/auth/session"));
  },
  async login(username: string, password: string): Promise<Session> {
    return decodeSession(
      await request("/auth/login", {
        method: "POST",
        body: JSON.stringify({ username, password }),
      }),
    );
  },
  async logout(): Promise<void> {
    await request("/auth/logout", { method: "POST" });
  },
  async rules(signal?: AbortSignal): Promise<Rule[]> {
    const body = await request("/rules", { signal });
    if (!record(body) || !Array.isArray(body.rules))
      throw new ApiError("服务器返回的规则列表无效，请刷新重试。", 0, "invalid_rules");
    return body.rules.map(decodeRule);
  },
  async create(input: RuleInput): Promise<Rule> {
    return decodeRule(
      await request("/rules", { method: "POST", body: JSON.stringify(input) }),
    );
  },
  async update(rule: Rule, input: RuleInput): Promise<Rule> {
    return decodeRule(
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
    return decodeRule(await request(`/rules/${encodeURIComponent(rule.id)}/traffic/reset`, {
      method: "POST", headers: revisionHeader(rule.revision),
    }));
  },
  async retry(rule: Rule): Promise<Rule> {
    return decodeRule(
      await request(`/rules/${encodeURIComponent(rule.id)}/retry`, {
        method: "POST",
        headers: revisionHeader(rule.revision),
      }),
    );
  },
};

// 错误对象保留到渲染时再翻译，切换语言不会清除提示或丢失原因。
const errorKeys: Record<string, MessageKey> = {
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
      return t("规则已在其他页面更新，请关闭表单并刷新后重新编辑。");
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
    listen_port: ["请输入 1–65535 的整数端口。", "端口范围为 1–65535", "监听端口已占用", "该监听端口已被占用", "UDP 监听端口已被占用"],
    target_host: ["请输入 IP 地址或域名，不含协议、路径或端口。", "目标必须是单播地址或域名", "请输入 IP 或域名，不包含协议、路径或端口"],
    target_port: ["请输入 1–65535 的整数端口。", "端口范围为 1–65535"],
    enabled: ["启用状态不能为空"],
    traffic_enabled: [],
  };
  const reason = reasons[field].find((key) => key === message);
  return reason ? t(reason) : t("请检查此字段");
}

export function ruleErrorMessage(rule: Rule): string {
  return listenerMessage(rule.last_error ?? "")
    ?? (rule.last_error_code === "listen_failed"
      ? t("{protocol} 监听失败，请检查地址、端口占用和权限", { protocol: rule.protocol.toUpperCase() })
      : t("监听失败，请检查地址、端口占用和权限"));
}
