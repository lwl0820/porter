import { mount, flushPromises, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.vue";
import { setSession } from "./api";
import type { Rule, RuleInput } from "./types";

const example: Rule = {
  id: "rule-1",
  name: "应用入口",
  protocol: "tcp",
  listen_host: "0.0.0.0",
  listen_port: 8443,
  target_host: "app.example.com",
  target_port: 443,
  enabled: true,
  revision: 1,
  created_at: "2026-10-16T12:00:00Z",
  updated_at: "2026-10-16T12:00:00Z",
  status: "running",
};
let wrapper: VueWrapper | undefined;
let records: Rule[];
let authenticated: boolean;
let calls: { path: string; options: RequestInit }[];
let failurePath = "";
let failureStatus = 0;
let failureFields: Record<string, string> = {};

function response(value: unknown, status = 200): Response {
  return new Response(status === 204 ? null : JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
async function start(): Promise<VueWrapper> {
  wrapper = mount(App, { attachTo: document.body });
  await flushPromises();
  return wrapper;
}
function button(text: string) {
  const match = wrapper
    ?.findAll("button")
    .find((item) => item.text().includes(text));
  if (!match) throw new Error(`未找到按钮：${text}`);
  return match;
}
async function fillRule(name = "DNS 入口"): Promise<void> {
  await wrapper!.get("#rule-name").setValue(name);
  await wrapper!.get("#listen-port").setValue("5353");
  await wrapper!.get("#target-host").setValue("10.0.0.2");
  await wrapper!.get("#target-port").setValue("53");
}

beforeEach(() => {
  records = [{ ...example }];
  authenticated = true;
  calls = [];
  failurePath = "";
  failureStatus = 0;
  failureFields = {};
  setSession();
  Object.defineProperty(HTMLDialogElement.prototype, "showModal", {
    configurable: true,
    value: function (this: HTMLDialogElement) {
      this.setAttribute("open", "");
    },
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, options: RequestInit = {}) => {
      calls.push({ path, options });
      if (failurePath === path)
        return response(
          {
            error: {
              code: "validation",
              message: "该监听端口已被占用",
              fields: failureFields,
            },
          },
          failureStatus,
        );
      if (path === "/api/auth/login") {
        const credentials = JSON.parse(String(options.body)) as {
          username: string;
          password: string;
        };
        if (
          credentials.username !== "admin" ||
          credentials.password !== "secret"
        )
          return response(
            { error: { code: "credentials", message: "用户名或密码错误" } },
            401,
          );
        authenticated = true;
        return response({ username: "admin", csrf_token: "csrf-demo" });
      }
      if (!authenticated)
        return response(
          { error: { code: "unauthorized", message: "请先登录" } },
          401,
        );
      if (path === "/api/auth/session")
        return response({ username: "admin", csrf_token: "csrf-demo" });
      if (path === "/api/auth/logout") {
        authenticated = false;
        return response(null, 204);
      }
      if (path === "/api/rules" && !options.method)
        return response({ rules: records });
      if (path === "/api/rules" && options.method === "POST") {
        const input = JSON.parse(String(options.body)) as RuleInput;
        const created: Rule = {
          ...example,
          ...input,
          id: "new-rule",
          status: input.enabled ? "running" : "disabled",
        };
        records.push(created);
        return response(created, 201);
      }
      const current = records.find((rule) => path.includes(rule.id));
      if (current && options.method === "DELETE") {
        records = records.filter((rule) => rule.id !== current.id);
        return response(null, 204);
      }
      if (current && options.method === "PUT") {
        const input = JSON.parse(String(options.body)) as RuleInput;
        Object.assign(current, input, {
          revision: current.revision + 1,
          status: input.enabled ? "running" : "disabled",
        });
        return response(current);
      }
      if (current && path.endsWith("/retry")) {
        current.status = "running";
        current.last_error = "";
        return response(current);
      }
      return response({ error: { message: "接口不存在" } }, 404);
    }),
  );
});

afterEach(() => {
  wrapper?.unmount();
  wrapper = undefined;
  document.body.innerHTML = "";
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("管理员访问", () => {
  it("错误凭据保留在登录页，正确登录后可退出", async () => {
    authenticated = false;
    await start();
    await wrapper!.get("#username").setValue("admin");
    await wrapper!.get("#password").setValue("wrong");
    await wrapper!.get("form").trigger("submit");
    await flushPromises();
    expect(wrapper!.get('[role="alert"]').text()).toContain("用户名或密码错误");
    await wrapper!.get("#password").setValue("secret");
    await wrapper!.get("form").trigger("submit");
    await flushPromises();
    expect(wrapper!.find(".dashboard").exists()).toBe(true);
    expect(wrapper!.find("#password").exists()).toBe(false);
    expect(wrapper!.get(".logout-button").attributes("aria-label")).toBe(
      "退出登录",
    );
    expect(wrapper!.get(".refresh-button").attributes("aria-label")).toBe(
      "刷新规则",
    );
    await button("退出登录").trigger("click");
    await flushPromises();
    expect(wrapper!.find("#username").exists()).toBe(true);
    expect(wrapper!.find(".dashboard").exists()).toBe(false);
    const logout = calls.find((call) => call.path.endsWith("/logout"))!;
    expect(new Headers(logout.options.headers).get("X-CSRF-Token")).toBe(
      "csrf-demo",
    );
  });

  it("登录过期后关闭规则表单并回到登录页", async () => {
    await start();
    await button("新增规则").trigger("click");
    await flushPromises();
    authenticated = false;
    await fillRule();
    await wrapper!.get(".editor").trigger("submit");
    await flushPromises();
    expect(wrapper!.find("dialog").exists()).toBe(false);
    expect(wrapper!.get(".login-page").text()).toContain("登录已过期");
    expect(wrapper!.find(".dashboard").exists()).toBe(false);
  });
});

describe("转发规则管理", () => {
  it("支持名称、地址、协议及状态筛选与清除", async () => {
    records.push({
      ...example,
      id: "dns",
      name: "DNS 服务",
      protocol: "udp",
      enabled: false,
      status: "disabled",
      target_port: 53,
    });
    await start();
    await wrapper!.get('[aria-label="搜索规则"]').setValue("DNS");
    expect(wrapper!.findAll(".desktop-table tbody tr")).toHaveLength(1);
    expect(wrapper!.get(".desktop-table tbody").text()).toContain("DNS 服务");
    await wrapper!.get('[aria-label="按协议筛选"]').setValue("tcp");
    expect(wrapper!.text()).toContain("没有找到匹配的规则");
    await button("清除筛选").trigger("click");
    expect(wrapper!.findAll(".desktop-table tbody tr")).toHaveLength(2);
    await wrapper!.get('[aria-label="搜索规则"]').setValue("app.example");
    await wrapper!.get('[aria-label="按状态筛选"]').setValue("disabled");
    expect(wrapper!.findAll(".desktop-table tbody tr")).toHaveLength(1);
    expect(wrapper!.get(".desktop-table tbody").text()).toContain("DNS 服务");
  });

  it("新增 UDP 规则前校验端口，保存仅发送允许字段和 CSRF", async () => {
    records = [];
    await start();
    expect(wrapper!.text()).toContain("从第一条规则开始");
    await button("新增规则").trigger("click");
    await flushPromises();
    await wrapper!.get(".editor").trigger("submit");
    await flushPromises();
    expect(wrapper!.text()).toContain("请输入 1–65535 的整数端口");
    expect(document.activeElement?.id).toBe("rule-name");
    expect(calls.filter((call) => call.options.method === "POST")).toHaveLength(
      0,
    );
    await fillRule();
    await wrapper!.get('input[value="udp"]').setValue();
    await wrapper!.get(".editor").trigger("submit");
    await flushPromises();
    expect(wrapper!.find("dialog").exists()).toBe(false);
    expect(wrapper!.get(".desktop-table").text()).toContain("DNS 入口");
    const created = calls.find(
      (call) => call.path === "/api/rules" && call.options.method === "POST",
    )!;
    expect(JSON.parse(String(created.options.body))).toEqual({
      name: "DNS 入口",
      protocol: "udp",
      listen_host: "0.0.0.0",
      listen_port: 5353,
      target_host: "10.0.0.2",
      target_port: 53,
      enabled: true,
    });
    expect(new Headers(created.options.headers).get("X-CSRF-Token")).toBe(
      "csrf-demo",
    );
    expect(created.options.credentials).toBe("same-origin");
  });

  it("字段冲突保留编辑内容并聚焦错误字段，版本冲突有明确提示", async () => {
    await start();
    await wrapper!
      .get('.desktop-table [aria-label="编辑 应用入口"]')
      .trigger("click");
    await flushPromises();
    await wrapper!.get("#rule-name").setValue("新的名称");
    failurePath = "/api/rules/rule-1";
    failureStatus = 409;
    failureFields = { listen_port: "监听端口已占用" };
    await wrapper!.get(".editor").trigger("submit");
    await flushPromises();
    expect(wrapper!.get("#rule-name").element).toHaveProperty(
      "value",
      "新的名称",
    );
    expect(wrapper!.text()).toContain("监听端口已占用");
    expect(document.activeElement?.id).toBe("listen-port");
    failureStatus = 412;
    failureFields = {};
    await wrapper!.get(".editor").trigger("submit");
    await flushPromises();
    expect(wrapper!.get("dialog").text()).toContain("规则已在其他页面更新");
  });

  it("编辑带 revision，启停和删除对应真实操作并先确认", async () => {
    await start();
    await wrapper!
      .get('.desktop-table [aria-label="编辑 应用入口"]')
      .trigger("click");
    await flushPromises();
    await wrapper!.get("#rule-name").setValue("新入口");
    await wrapper!.get(".editor").trigger("submit");
    await flushPromises();
    const updated = calls.find((call) => call.options.method === "PUT")!;
    expect(new Headers(updated.options.headers).get("If-Match")).toBe('"1"');
    expect(
      Object.keys(JSON.parse(String(updated.options.body)) as object),
    ).not.toContain("revision");
    await wrapper!
      .get('.desktop-table [aria-label="停用 新入口"]')
      .trigger("click");
    await flushPromises();
    expect(wrapper!.get(".desktop-table").text()).toContain("已停用");
    const updates = calls.filter((call) => call.options.method === "PUT");
    expect(new Headers(updates[1]!.options.headers).get("If-Match")).toBe(
      '"2"',
    );
    await wrapper!
      .get('.desktop-table [aria-label="删除 新入口"]')
      .trigger("click");
    await flushPromises();
    expect(wrapper!.get("dialog").text()).toContain("现有会话会被断开");
    expect(
      calls.filter((call) => call.options.method === "DELETE"),
    ).toHaveLength(0);
    await button("确认删除").trigger("click");
    await flushPromises();
    expect(wrapper!.text()).toContain("从第一条规则开始");
    const deleted = calls.find((call) => call.options.method === "DELETE")!;
    expect(new Headers(deleted.options.headers).get("If-Match")).toBe('"3"');
  });

  it("错误规则展示原因，重试后刷新实际运行状态", async () => {
    records = [
      { ...example, status: "error", last_error: "监听端口已被其他程序占用" },
    ];
    await start();
    expect(wrapper!.get(".desktop-table").text()).toContain(
      "监听端口已被其他程序占用",
    );
    await wrapper!
      .get('.desktop-table [aria-label="重试 应用入口"]')
      .trigger("click");
    await flushPromises();
    expect(wrapper!.get(".desktop-table").text()).toContain("运行中");
    expect(
      new Headers(
        calls.find((call) => call.path.endsWith("/retry"))!.options.headers,
      ).get("If-Match"),
    ).toBe('"1"');
  });

  it("刷新失败保留已有列表，可重试恢复；隐藏页面暂停轮询", async () => {
    vi.useFakeTimers();
    await start();
    failurePath = "/api/rules";
    failureStatus = 500;
    await button("刷新").trigger("click");
    await flushPromises();
    expect(wrapper!.text()).toContain("当前保留上次同步的数据");
    expect(wrapper!.get(".desktop-table").text()).toContain("应用入口");
    failurePath = "";
    await button("重试").trigger("click");
    await flushPromises();
    expect(wrapper!.find(".list-notice").exists()).toBe(false);
    const state = vi
      .spyOn(document, "visibilityState", "get")
      .mockReturnValue("hidden");
    const before = calls.length;
    await vi.advanceTimersByTimeAsync(15000);
    expect(calls).toHaveLength(before);
    state.mockReturnValue("visible");
    await vi.advanceTimersByTimeAsync(5000);
    expect(calls).toHaveLength(before + 1);
  });
});
