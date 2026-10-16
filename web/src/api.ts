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
    throw new ApiError("登录信息无效，请重新登录。");
  }
  return { username: value.username, csrf_token: value.csrf_token };
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
    !Number.isInteger(value.revision) ||
    typeof value.created_at !== "string" ||
    typeof value.updated_at !== "string" ||
    !["running", "disabled", "error"].includes(String(value.status)) ||
    (value.last_error !== undefined && typeof value.last_error !== "string") ||
    (value.last_error_code !== undefined &&
      typeof value.last_error_code !== "string")
  ) {
    throw new ApiError("服务器返回的规则格式无效，请刷新重试。");
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
    throw new ApiError("无法连接服务器，请检查网络后重试。");
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
      throw new ApiError("服务器返回的规则列表无效，请刷新重试。");
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
  async retry(rule: Rule): Promise<Rule> {
    return decodeRule(
      await request(`/rules/${encodeURIComponent(rule.id)}/retry`, {
        method: "POST",
        headers: revisionHeader(rule.revision),
      }),
    );
  },
};

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "操作失败，请稍后重试。";
}
