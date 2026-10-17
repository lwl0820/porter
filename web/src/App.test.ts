import { mount, flushPromises, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.vue";
import { setSession } from "./api";
import { languageStorageKey, setLanguage } from "./i18n";
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
  traffic_enabled: false,
  traffic: { upload_bytes: "0", download_bytes: "0", upload_bytes_per_second: 0, download_bytes_per_second: 0 },
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
let failureCode = "validation";
let failureMessage = "该监听端口已被占用";
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
  localStorage.clear();
  setLanguage("zh-CN");
  localStorage.clear();
  failureCode = "validation";
  failureMessage = "该监听端口已被占用";
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
              code: failureCode,
              message: failureMessage,
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
      if (current && path.endsWith("/traffic/reset")) {
        current.traffic = { upload_bytes: "0", download_bytes: "0", upload_bytes_per_second: 0, download_bytes_per_second: 0 };
        current.revision++;
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
  vi.restoreAllMocks();
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
      traffic_enabled: false,
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

describe("TCP+UDP 组合规则", () => {
  it("一次创建为一条规则，编辑回显并可切换协议，整体启停和删除各发送一次请求", async () => {
    records = [];
    await start();
    await button("新增规则").trigger("click");
    await flushPromises();
    expect(wrapper!.findAll('input[name="protocol"]')).toHaveLength(3);
    expect(
      wrapper!.get('input[value="tcp+udp"]').attributes("aria-label"),
    ).toBe("TCP+UDP");
    await fillRule("双协议入口");
    await wrapper!.get('input[value="tcp+udp"]').setValue();
    await wrapper!.get(".editor").trigger("submit");
    await flushPromises();
    const creations = calls.filter(
      (call) => call.path === "/api/rules" && call.options.method === "POST",
    );
    expect(creations).toHaveLength(1);
    expect(JSON.parse(String(creations[0]!.options.body))).toEqual({
      name: "双协议入口",
      protocol: "tcp+udp",
      listen_host: "0.0.0.0",
      listen_port: 5353,
      target_host: "10.0.0.2",
      target_port: 53,
      enabled: true,
      traffic_enabled: false,
    });
    expect(wrapper!.findAll(".desktop-table tbody tr")).toHaveLength(1);
    expect(wrapper!.findAll(".mobile-cards .rule-card")).toHaveLength(1);
    expect(wrapper!.get(".summary-card:first-child strong").text()).toBe("1条");
    expect(wrapper!.get(".desktop-table .protocol-badge").text()).toBe(
      "TCP+UDP",
    );
    expect(wrapper!.get(".desktop-table .protocol-badge").classes()).toContain(
      "both",
    );
    expect(wrapper!.get(".mobile-cards .protocol-badge").classes()).toContain(
      "both",
    );

    for (const [before, next] of [
      ["tcp+udp", "tcp"],
      ["tcp", "tcp+udp"],
      ["tcp+udp", "udp"],
      ["udp", "tcp+udp"],
    ]) {
      await wrapper!
        .get('.desktop-table [aria-label="编辑 双协议入口"]')
        .trigger("click");
      await flushPromises();
      expect(wrapper!.get(`input[value="${before}"]`).element).toHaveProperty(
        "checked",
        true,
      );
      await wrapper!.get(`input[value="${next}"]`).setValue();
      await wrapper!.get(".editor").trigger("submit");
      await flushPromises();
      expect(wrapper!.findAll(".desktop-table tbody tr")).toHaveLength(1);
      expect(wrapper!.get(".desktop-table .protocol-badge").text()).toBe(
        next!.toUpperCase(),
      );
    }
    const edits = calls.filter((call) => call.options.method === "PUT");
    expect(edits).toHaveLength(4);
    expect(edits.every((call) => call.path === "/api/rules/new-rule")).toBe(
      true,
    );
    expect(new Headers(edits[3]!.options.headers).get("If-Match")).toBe('"4"');

    await wrapper!
      .get('.desktop-table [aria-label="停用 双协议入口"]')
      .trigger("click");
    await flushPromises();
    expect(calls.filter((call) => call.options.method === "PUT")).toHaveLength(
      5,
    );
    const disabled = calls
      .filter((call) => call.options.method === "PUT")
      .at(-1)!;
    expect(JSON.parse(String(disabled.options.body))).toMatchObject({
      protocol: "tcp+udp",
      enabled: false,
    });
    expect(wrapper!.get(".desktop-table .status-badge").text()).toBe("已停用");
    await wrapper!
      .get('.desktop-table [aria-label="启用 双协议入口"]')
      .trigger("click");
    await flushPromises();
    expect(calls.filter((call) => call.options.method === "PUT")).toHaveLength(
      6,
    );
    expect(wrapper!.get(".desktop-table .status-badge").text()).toBe("运行中");
    expect(wrapper!.get(".summary-card:first-child strong").text()).toBe("1条");
    await wrapper!
      .get('.desktop-table [aria-label="删除 双协议入口"]')
      .trigger("click");
    await flushPromises();
    await button("确认删除").trigger("click");
    await flushPromises();
    expect(
      calls.filter((call) => call.options.method === "DELETE"),
    ).toHaveLength(1);
    expect(wrapper!.get(".summary-card:first-child strong").text()).toBe("0条");
    expect(wrapper!.text()).toContain("从第一条规则开始");
  });

  it("TCP 和 UDP 筛选各包含组合规则，组合筛选只保留一条组合记录", async () => {
    records = [
      { ...example, name: "TCP 入口" },
      { ...example, id: "udp-only", name: "UDP 入口", protocol: "udp" },
      { ...example, id: "both", name: "组合入口", protocol: "tcp+udp" },
    ];
    await start();
    expect(wrapper!.findAll(".desktop-table tbody tr")).toHaveLength(3);
    expect(wrapper!.get(".summary-card:first-child strong").text()).toBe("3条");
    for (const filter of ["tcp", "udp", "tcp+udp"]) {
      await wrapper!.get('[aria-label="按协议筛选"]').setValue(filter);
      const names = wrapper!
        .findAll(".desktop-table .rule-name-cell strong")
        .map((item) => item.text());
      expect(names).toEqual(
        filter === "tcp+udp"
          ? ["组合入口"]
          : [`${filter.toUpperCase()} 入口`, "组合入口"],
      );
      expect(wrapper!.findAll(".mobile-cards .rule-card")).toHaveLength(
        names.length,
      );
      expect(wrapper!.get(".summary-card:first-child strong").text()).toBe(
        "3条",
      );
    }
    await wrapper!.get('[aria-label="搜索规则"]').setValue("组合");
    expect(wrapper!.findAll(".desktop-table tbody tr")).toHaveLength(1);
    expect(wrapper!.get(".panel-footer").text()).toContain(
      "显示 1 条，共 3 条规则",
    );
    await wrapper!.get('[aria-label="按协议筛选"]').setValue("");
    await wrapper!.get('[aria-label="搜索规则"]').setValue("");
    expect(wrapper!.findAll(".desktop-table tbody tr")).toHaveLength(3);
  });

  it("组合错误整体重试一次，保存冲突后保留组合选择和输入", async () => {
    records = [
      {
        ...example,
        protocol: "tcp+udp",
        status: "error",
        last_error: "UDP 监听端口已被占用",
      },
    ];
    await start();
    expect(wrapper!.get(".desktop-table").text()).toContain(
      "UDP 监听端口已被占用",
    );
    await wrapper!
      .get('.desktop-table [aria-label="重试 应用入口"]')
      .trigger("click");
    await flushPromises();
    expect(calls.filter((call) => call.path.endsWith("/retry"))).toHaveLength(
      1,
    );
    expect(wrapper!.get(".desktop-table .status-badge").text()).toBe("运行中");
    expect(wrapper!.get(".desktop-table .protocol-badge").text()).toBe(
      "TCP+UDP",
    );
    await wrapper!
      .get('.desktop-table [aria-label="编辑 应用入口"]')
      .trigger("click");
    await flushPromises();
    await wrapper!.get("#rule-name").setValue("双协议新名称");
    failurePath = "/api/rules/rule-1";
    failureStatus = 409;
    failureFields = { listen_port: "UDP 监听端口已被占用" };
    await wrapper!.get(".editor").trigger("submit");
    await flushPromises();
    expect(wrapper!.get('input[value="tcp+udp"]').element).toHaveProperty(
      "checked",
      true,
    );
    expect(wrapper!.get("#rule-name").element).toHaveProperty(
      "value",
      "双协议新名称",
    );
    expect(document.activeElement?.id).toBe("listen-port");
    expect(wrapper!.get(".desktop-table .rule-name-cell strong").text()).toBe(
      "应用入口",
    );
  });
});

describe("流量统计", () => {
  it("新增默认关闭，编辑保存统计开关并回显", async () => {
    await start();
    await button("新增规则").trigger("click");
    await flushPromises();
    expect(wrapper!.get<HTMLInputElement>('[aria-label="流量统计"]').element.checked).toBe(false);
    await button("取消").trigger("click");
    await wrapper!.get('.desktop-table [aria-label="编辑 应用入口"]').trigger("click");
    await flushPromises();
    await wrapper!.get('[aria-label="流量统计"]').setValue(true);
    await wrapper!.get(".editor").trigger("submit");
    await flushPromises();
    const update = calls.find((call) => call.options.method === "PUT")!;
    expect(JSON.parse(String(update.options.body)).traffic_enabled).toBe(true);
    expect(wrapper!.text()).toContain("当前速度 · 累计流量");
    await wrapper!.get('.desktop-table [aria-label="编辑 应用入口"]').trigger("click");
    await flushPromises();
    expect(wrapper!.get<HTMLInputElement>('[aria-label="流量统计"]').element.checked).toBe(true);
  });

  it("桌面和窄屏显示上下行累计量、速度与关闭状态", async () => {
    records[0]!.traffic = { upload_bytes: "2048", download_bytes: "4096", upload_bytes_per_second: 0, download_bytes_per_second: 0 };
    await start();
    for (const surface of [".desktop-table", ".mobile-cards"]) {
      const region = wrapper!.get(surface);
      expect(region.text()).toContain("统计已关闭");
      expect(region.text()).toContain("上行0 B/s2.0 KiB");
      expect(region.text()).toContain("下行0 B/s4.0 KiB");
      expect(region.get(".traffic-direction strong").attributes("title")).toBe("2048 B");
      expect(region.get(".rule-traffic button").attributes("aria-label")).toBe("重置 应用入口 的流量");
    }
  });

  it("取消不重置，成功重置后后续写入使用新版本", async () => {
    records[0]!.traffic = { ...example.traffic, upload_bytes: "1234" };
    await start();
    await button("重置流量").trigger("click");
    await flushPromises();
    expect(wrapper!.get('dialog[aria-label="重置流量"]').text()).toContain("不会影响转发连接或统计开关");
    await button("取消").trigger("click");
    expect(calls.some((call) => call.path.endsWith("/traffic/reset"))).toBe(false);
    expect(wrapper!.get(".traffic-direction strong").attributes("title")).toBe("1234 B");
    await button("重置流量").trigger("click");
    await button("确认重置").trigger("click");
    await flushPromises();
    expect(wrapper!.find("dialog").exists()).toBe(false);
    expect(wrapper!.get(".traffic-direction strong").attributes("title")).toBe("0 B");
    const reset = calls.find((call) => call.path.endsWith("/traffic/reset"))!;
    expect(reset.options.method).toBe("POST");
    expect(new Headers(reset.options.headers).get("If-Match")).toBe('"1"');
    expect(new Headers(reset.options.headers).get("X-CSRF-Token")).toBe("csrf-demo");
    await wrapper!.get('.desktop-table [role="switch"]').trigger("click");
    await flushPromises();
    const update = calls.find((call) => call.options.method === "PUT")!;
    expect(new Headers(update.options.headers).get("If-Match")).toBe('"2"');
  });

  it.each([500, 412])("重置失败 %s 保留累计值并提示", async (status) => {
    records[0]!.traffic = { ...example.traffic, upload_bytes: "4321" };
    await start();
    failurePath = "/api/rules/rule-1/traffic/reset";
    failureStatus = status;
    await button("重置流量").trigger("click");
    await button("确认重置").trigger("click");
    await flushPromises();
    expect(wrapper!.get(".traffic-direction strong").attributes("title")).toBe("4321 B");
    expect(wrapper!.get('dialog[aria-label="重置流量"] [role="alert"]').text()).toContain(status === 412 ? "其他页面更新" : "监听端口已被占用");
    expect(button("确认重置").attributes("disabled")).toBeUndefined();
  });

  it("等待重置期间禁止重复提交与其他变更", async () => {
    await start();
    const original = fetch;
    let resolveReset!: (value: Response) => void;
    let requests = 0;
    vi.stubGlobal("fetch", vi.fn((path: string, options: RequestInit) => {
      if (path.endsWith("/traffic/reset")) {
        requests++;
        return new Promise<Response>((resolve) => { resolveReset = resolve; });
      }
      return original(path, options);
    }));
    await button("重置流量").trigger("click");
    await button("确认重置").trigger("click");
    await button("正在重置").trigger("click");
    expect(requests).toBe(1);
    expect(wrapper!.get('.desktop-table [role="switch"]').attributes("disabled")).toBeDefined();
    expect(button("取消").attributes("disabled")).toBeDefined();
    resolveReset(response({ ...example, revision: 2 }));
    await flushPromises();
    expect(wrapper!.find("dialog").exists()).toBe(false);
    expect(wrapper!.get('.desktop-table [role="switch"]').attributes("disabled")).toBeUndefined();
  });

  it("重置前发起的旧列表请求不会覆盖新统计和版本", async () => {
    records[0]!.traffic = { ...example.traffic, upload_bytes: "9999" };
    await start();
    const oldResponse = response({ rules: structuredClone(records) });
    const original = fetch;
    let resolveList!: (value: Response) => void;
    let pending = true;
    vi.stubGlobal("fetch", vi.fn((path: string, options: RequestInit) => {
      if (pending && path === "/api/rules" && !options.method) {
        pending = false;
        return new Promise<Response>((resolve) => { resolveList = resolve; });
      }
      return original(path, options);
    }));
    await button("刷新").trigger("click");
    await button("重置流量").trigger("click");
    await button("确认重置").trigger("click");
    await flushPromises();
    resolveList(oldResponse);
    await flushPromises();
    expect(wrapper!.get(".traffic-direction strong").attributes("title")).toBe("0 B");
    await wrapper!.get('.desktop-table [role="switch"]').trigger("click");
    await flushPromises();
    const update = calls.find((call) => call.options.method === "PUT")!;
    expect(new Headers(update.options.headers).get("If-Match")).toBe('"2"');
  });
});


describe("中英文界面", () => {
  it("默认中文，切换立即同步文档与规则，刷新并退出登录仍记住偏好", async () => {
    await start();
    expect(document.documentElement.lang).toBe("zh-CN");
    expect(document.title).toBe("Porter · 端口转发");
    const callsBefore = calls.length;
    await wrapper!.get(".language-switcher select").setValue("en");
    expect(document.documentElement.lang).toBe("en");
    expect(document.title).toBe("Porter · Port forwarding");
    expect(localStorage.getItem(languageStorageKey)).toBe("en");
    expect(wrapper!.get("h1").text()).toBe("Port forwarding");
    expect(wrapper!.get(".desktop-table").text()).toContain("应用入口");
    expect(wrapper!.find('[aria-label="Sign out"]').exists()).toBe(true);
    expect(wrapper!.find('[aria-label="Refresh rules"]').exists()).toBe(true);
    expect(calls).toHaveLength(callsBefore);
    wrapper!.unmount();
    await start();
    expect(wrapper!.get(".language-switcher select").element).toHaveProperty("value", "en");
    await button("Sign out").trigger("click");
    await flushPromises();
    expect(wrapper!.text()).toContain("Welcome back");
    expect(wrapper!.get(".language-switcher select").element).toHaveProperty("value", "en");
  });

  it("启动等待时即可切换语言", async () => {
    let resolveSession!: (response: Response) => void;
    vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>((resolve) => { resolveSession = resolve; })));
    wrapper = mount(App, { attachTo: document.body });
    await wrapper.get(".language-switcher select").setValue("en");
    expect(wrapper.text()).toContain("Connecting to Porter…");
    resolveSession(response({ error: { code: "unauthorized" } }, 401));
    await flushPromises();
    expect(wrapper.text()).toContain("Welcome back");
  });

  it("非法偏好和不可用存储回退中文，仍允许本页切换", async () => {
    localStorage.setItem(languageStorageKey, "other");
    await start();
    expect(document.documentElement.lang).toBe("zh-CN");
    wrapper!.unmount();
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new Error("存储不可用"); });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("存储不可用"); });
    await start();
    await wrapper!.get(".language-switcher select").setValue("en");
    expect(wrapper!.get("h1").text()).toBe("Port forwarding");
    expect(document.documentElement.lang).toBe("en");
  });

  it("登录缺失输入使用所选语言提示并聚焦，保留必填语义且不发请求", async () => {
    authenticated = false;
    await start();
    const callsBefore = calls.length;
    expect(wrapper!.get("#username").attributes("required")).toBeDefined();
    expect(wrapper!.get("#password").attributes("required")).toBeDefined();
    expect(wrapper!.get("form").attributes("novalidate")).toBeDefined();
    await wrapper!.get("form").trigger("submit");
    await flushPromises();
    expect(wrapper!.get('[role="alert"]').text()).toContain("请输入管理员用户名");
    expect(document.activeElement).toBe(wrapper!.get("#username").element);
    await wrapper!.get(".language-switcher select").setValue("en");
    expect(wrapper!.get('[role="alert"]').text()).toContain("Enter administrator username");
    await wrapper!.get("#username").setValue("admin");
    await wrapper!.get("form").trigger("submit");
    await flushPromises();
    expect(wrapper!.get('[role="alert"]').text()).toContain("Enter password");
    expect(document.activeElement).toBe(wrapper!.get("#password").element);
    expect(wrapper!.get("#username").element).toHaveProperty("value", "admin");
    expect(calls).toHaveLength(callsBefore);
  });

  it("登录错误随切换更新，账号密码与请求次数保持不变", async () => {
    authenticated = false;
    await start();
    await wrapper!.get("#username").setValue("admin");
    await wrapper!.get("#password").setValue("wrong");
    await wrapper!.get("form").trigger("submit");
    await flushPromises();
    expect(wrapper!.get('[role="alert"]').text()).toContain("用户名或密码错误");
    const callsBefore = calls.length;
    await wrapper!.get(".language-switcher select").setValue("en");
    expect(wrapper!.get('[role="alert"]').text()).toContain("Incorrect username or password");
    expect(wrapper!.get("#username").element).toHaveProperty("value", "admin");
    expect(wrapper!.get("#password").element).toHaveProperty("value", "wrong");
    expect(calls).toHaveLength(callsBefore);
  });

  it("弹窗内切换保留输入和已有校验，英文保存后通知也能切回中文", async () => {
    await start();
    await button("新增规则").trigger("click");
    await wrapper!.get("#rule-name").setValue("用户自定义名称");
    await wrapper!.get(".editor").trigger("submit");
    expect(wrapper!.get("#error-listen_port").text()).toContain("请输入 1–65535");
    const callsBefore = calls.length;
    await wrapper!.get("dialog .language-switcher select").setValue("en");
    expect(wrapper!.get("#error-listen_port").text()).toBe("Enter an integer port from 1 to 65535.");
    expect(wrapper!.get("#rule-name").element).toHaveProperty("value", "用户自定义名称");
    expect(wrapper!.get(".protocol-choice").text()).toContain("Reliable connection-based transport");
    expect(calls).toHaveLength(callsBefore);
    await fillRule("用户自定义名称");
    await wrapper!.get(".editor").trigger("submit");
    await flushPromises();
    expect(wrapper!.find("dialog").exists()).toBe(false);
    expect(wrapper!.get('[role="status"]').text()).toContain("Rule created");
    await wrapper!.get(".language-switcher select").setValue("zh-CN");
    expect(wrapper!.get('[role="status"]').text()).toContain("规则已创建");
    expect(calls.filter((call) => call.options.method === "POST")).toHaveLength(1);
  });

  it("服务端单播字段错误与监听冲突按原始原因响应式翻译", async () => {
    await start();
    await button("新增规则").trigger("click");
    await fillRule();
    failurePath = "/api/rules";
    failureStatus = 400;
    failureCode = "validation_error";
    failureMessage = "请检查表单内容";
    failureFields = { target_host: "目标必须是单播地址或域名" };
    await wrapper!.get(".editor").trigger("submit");
    await flushPromises();
    await wrapper!.get("dialog .language-switcher select").setValue("en");
    expect(wrapper!.get("#error-target_host").text()).toBe("The target must be a unicast address or hostname.");
    expect(wrapper!.get('dialog [role="alert"]').text()).toContain("Please check the form fields");
    failureStatus = 409;
    failureCode = "listen_conflict";
    failureMessage = "UDP 监听失败，请检查地址、端口占用和权限；更改重叠地址时请先停用规则";
    failureFields = {};
    await wrapper!.get(".editor").trigger("submit");
    await flushPromises();
    expect(wrapper!.get('dialog [role="alert"]').text()).toContain("UDP listener failed");
    expect(wrapper!.get('dialog [role="alert"]').text()).toContain("Disable the rule before changing overlapping addresses");
    await wrapper!.get("dialog .language-switcher select").setValue("zh-CN");
    expect(wrapper!.get('dialog [role="alert"]').text()).toContain(failureMessage);
  });

  it("规则监听错误保留协议诊断，删除及重置确认随切换更新", async () => {
    records = [{ ...example, status: "error", protocol: "tcp+udp", last_error_code: "listen_failed", last_error: "UDP 监听失败，请检查地址、端口占用和权限" }];
    localStorage.setItem(languageStorageKey, "en");
    await start();
    expect(wrapper!.get(".desktop-table .rule-error").text()).toContain("UDP listener failed");
    await wrapper!.get('.desktop-table [aria-label="Delete 应用入口"]').trigger("click");
    expect(wrapper!.get("dialog").text()).toContain("Delete 应用入口?");
    await wrapper!.get("dialog .language-switcher select").setValue("zh-CN");
    expect(wrapper!.get("dialog").text()).toContain("确定删除 应用入口 吗？");
    await button("保留规则").trigger("click");
    await wrapper!.get('.desktop-table [aria-label="重置 应用入口 的流量"]').trigger("click");
    await wrapper!.get("dialog .language-switcher select").setValue("en");
    expect(wrapper!.get("dialog").text()).toContain("Clear the total upload and download traffic for 应用入口?");
    expect(calls.filter((call) => call.options.method)).toHaveLength(0);
  });

  it("网络失败与已有列表在切换时保留", async () => {
    await start();
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("Failed to fetch")));
    await wrapper!.get('[aria-label="刷新规则"]').trigger("click");
    await flushPromises();
    await wrapper!.get(".language-switcher select").setValue("en");
    expect(wrapper!.get('[role="alert"]').text()).toContain("Unable to reach the server");
    expect(wrapper!.get('[role="alert"]').text()).toContain("Showing the last synced data.");
    expect(wrapper!.get(".desktop-table").text()).toContain("应用入口");
  });
});
