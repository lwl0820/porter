import { mount, flushPromises, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import AdminApp from "./AdminApp.vue";
import UserApp from "./UserApp.vue";
import { adminApi, createClient, userApi } from "./api";
import { setLanguage } from "./i18n";
import { gibToBytes } from "./traffic";
import type { Account, Plan, Rule, Session } from "./types";

const stamp = "2026-10-17T00:00:00Z";
const plan: Plan = { id: "plan-a", name: "基础套餐", port_count: 10, traffic_limit_bytes: "9007199254740993", accounting_mode: "max", reset_mode: "fixed_day", reset_day: 31, created_at: stamp };
function account(id: string, username = id, port = 10000): Account {
 return { user: { id, username, role: "user", plan_id: plan.id, port_start: port, port_end: port + 9, created_at: stamp }, plan, usage: { cycle_start: stamp, cycle_end: "2026-10-31T00:00:00Z", upload_bytes: "9007199254740993", download_bytes: "30", used_bytes: "9007199254740993", limit_bytes: plan.traffic_limit_bytes, exceeded: true } };
}
function rule(id: string, ownerId: string, username = ownerId): Rule {
 return { id, name: `${id}入口`, owner_id: ownerId, owner: { id: ownerId, username, role: ownerId === "admin" ? "admin" : "user" }, protocol: "tcp", listen_host: "0.0.0.0", listen_port: ownerId === "b" ? 11000 : 10000, target_host: "example.com", target_port: 443, enabled: true, traffic_enabled: false, status: "quota_exceeded", traffic: { upload_bytes: "0", download_bytes: "0", upload_bytes_per_second: 0, download_bytes_per_second: 0 }, revision: 2, created_at: stamp, updated_at: stamp };
}
function response(body: unknown, status = 200): Response { return new Response(status === 204 ? null : JSON.stringify(body), { status }); }
const adminSession: Session = { user_id: "admin", username: "管理员", role: "admin", csrf_token: "admin-token" };
const userSession: Session = { user_id: "a", username: "甲用户", role: "user", csrf_token: "user-token" };
let wrapper: VueWrapper | undefined;
let calls: { path: string; options: RequestInit }[];
let records: Rule[];
let users: Account[];
beforeEach(() => {
 window.history.replaceState(null, "", "/admin/");
 localStorage.clear(); setLanguage("zh-CN");
 adminApi.setSession(); userApi.setSession();
 calls = []; users = [account("a", "甲用户"), account("b", "乙用户", 11000)]; records = [rule("one", "a", "甲用户"), rule("two", "b", "乙用户")];
 Object.defineProperty(HTMLDialogElement.prototype, "showModal", { configurable: true, value: function(this: HTMLDialogElement) { this.setAttribute("open", ""); } });
 vi.stubGlobal("fetch", vi.fn(async (path: string, options: RequestInit = {}) => {
  calls.push({ path, options });
  if (path.endsWith("/auth/login")) return response(path.includes("/admin/") ? adminSession : userSession);
  if (path.endsWith("/auth/session")) return response(path.includes("/admin/") ? adminSession : userSession);
  if (path === "/api/admin/users" && !options.method) return response({ users });
  if (path === "/api/admin/plans" && !options.method) return response({ plans: [plan] });
  if (path === "/api/admin/settings" && !options.method) return response({ port_start: 10000, port_end: 20000, revision: 3 });
  if (path === "/api/user/account") return response(users[0]);
  if (path.startsWith("/api/admin/rules") && !options.method) {
   const owner = new URL(path, "https://porter.test").searchParams.get("owner_id");
   return response({ rules: records.filter(item => !owner || item.owner_id === owner) });
  }
  if (path === "/api/user/rules" && !options.method) return response({ rules: records.filter(item => item.owner_id === "a").map(({ owner: _, ...item }) => { void _; return item; }) });
  if (path === "/api/admin/rules" && options.method === "POST") {
   const input = JSON.parse(String(options.body));
   return response({ ...rule("created", input.owner_id, "甲用户"), ...input }, 201);
  }
  if (path.includes("/rules/") && options.method === "PUT") return response({ ...records.find(item => path.endsWith(item.id)), ...JSON.parse(String(options.body)), revision: 3, status: "disabled" });
  if (path.endsWith("/plans") && options.method === "POST") return response({ ...plan, ...JSON.parse(String(options.body)) }, 201);
  if (path.endsWith("/users") && options.method === "POST") return response(account("new", JSON.parse(String(options.body)).username, 12000), 201);
  if (path.endsWith("/settings") && options.method === "PUT") return response({ ...JSON.parse(String(options.body)), revision: 4 });
  return response({}, 404);
 }));
});
afterEach(() => { wrapper?.unmount(); wrapper = undefined; document.body.innerHTML = ""; vi.restoreAllMocks(); vi.unstubAllGlobals(); });
function button(text: string) { return wrapper!.findAll("button").find(item => item.text().includes(text))!; }
async function startUser() { window.history.replaceState(null, "", "/user/"); wrapper = mount(UserApp, { attachTo: document.body }); await flushPromises(); }
async function startAdmin() { wrapper = mount(AdminApp, { attachTo: document.body }); await flushPromises(); }

describe("独立端与套餐界面", () => {
 it("管理员页头导航保留各页面入口、当前状态和切页收起行为", async () => {
  await startAdmin();
  const header = wrapper!.get("header");
  const nav = header.get('nav[aria-label="管理后台"]');
  expect(wrapper!.find("main nav").exists()).toBe(false);
  expect(nav.findAll("button").map(item => item.text())).toEqual(["转发规则", "套餐管理", "用户管理", "系统配置"]);
  expect(nav.get('[aria-current="page"]').text()).toBe("转发规则");
  const toggle = header.get('[aria-controls="admin-navigation"]');
  expect(toggle.attributes("aria-expanded")).toBe("false");
  await toggle.trigger("click");
  expect(toggle.attributes("aria-expanded")).toBe("true");
  await nav.findAll("button")[1]!.trigger("click"); await flushPromises();
  expect(wrapper!.get("h1").text()).toBe("套餐管理");
  expect(window.location.pathname).toBe("/admin/plans");
  expect(nav.get('[aria-current="page"]').text()).toBe("套餐管理");
  expect(toggle.attributes("aria-expanded")).toBe("false");
  expect(document.activeElement).toBe(toggle.element);
  await toggle.trigger("click");
  await nav.findAll("button")[1]!.trigger("keydown", { key: "Escape" });
  expect(toggle.attributes("aria-expanded")).toBe("false");
  expect(document.activeElement).toBe(toggle.element);
  await toggle.trigger("click"); await toggle.trigger("click");
  expect(toggle.attributes("aria-expanded")).toBe("false");
 });
 it("用户页头仅有本人页面且导航控件跟随语言切换", async () => {
  await startUser();
  const header = wrapper!.get("header");
  const nav = header.get('nav[aria-label="用户后台"]');
  expect(nav.findAll("button").map(item => item.text())).toEqual(["转发规则", "账户概览"]);
  await header.get('.language-switcher select').setValue("en");
  const toggle = header.get('[aria-label="Open navigation"]');
  await toggle.trigger("click");
  expect(toggle.attributes("aria-label")).toBe("Close navigation");
  await nav.findAll("button")[1]!.trigger("click"); await flushPromises();
  expect(window.location.pathname).toBe("/user/account");
  expect(nav.get('[aria-current="page"]').text()).toBe("Account overview");
  expect(toggle.attributes("aria-label")).toBe("Open navigation");
  expect(calls.every(item => item.path.startsWith("/api/user/"))).toBe(true);
 });
 it("资源保存期间导航和退出保持禁用，完成后可继续切页", async () => {
  await startAdmin(); await button("系统配置").trigger("click"); await flushPromises();
  const original = fetch;
  let resolve!: (value: Response) => void;
  vi.stubGlobal("fetch", vi.fn((path: string, options: RequestInit = {}) => {
   if (path.endsWith("/settings") && options.method === "PUT") return new Promise<Response>(done => { resolve = done; });
   return original(path, options);
  }));
  await wrapper!.get("form").trigger("submit");
  const nav = wrapper!.get("header nav");
  expect(nav.findAll("button").every(item => item.attributes("disabled") !== undefined)).toBe(true);
  expect(wrapper!.get('[aria-label="退出登录"]').attributes("disabled")).toBeDefined();
  await nav.findAll("button")[0]!.trigger("click");
  expect(window.location.pathname).toBe("/admin/settings");
  resolve(response({ port_start: 10000, port_end: 20000, revision: 4 })); await flushPromises();
  await nav.findAll("button")[0]!.trigger("click"); await flushPromises();
  expect(window.location.pathname).toBe("/admin/rules");
  expect(nav.get('[aria-current="page"]').text()).toBe("转发规则");
 });
 it("周期重置待恢复显示独立错误文案并保留管理员重试", async () => {
  records[0] = { ...records[0]!, status: "error", last_error_code: "quota_reset_pending", last_error: "流量周期重置暂未完成，正在重试" };
  await startAdmin();
  expect(wrapper!.get(".desktop-table .rule-error").text()).toBe("流量周期重置暂未完成，正在重试");
  expect(wrapper!.get('.desktop-table [aria-label="重试 one入口"]').attributes("disabled")).toBeUndefined();
  await wrapper!.get('.language-switcher select').setValue("en");
  expect(wrapper!.get(".desktop-table .rule-error").text()).toBe("Traffic cycle reset is pending. Retrying…");
  expect(wrapper!.get(".mobile-cards .mobile-rule-error").text()).toBe("Traffic cycle reset is pending. Retrying…");
  expect(wrapper!.get('.desktop-table [aria-label="Retry one入口"]').attributes("disabled")).toBeUndefined();
 });

 it("用户端仅请求本人接口并展示精确用量、UTC 与配额暂停", async () => {
  await startUser();
  expect(calls.every(item => item.path.startsWith("/api/user/"))).toBe(true);
  expect(wrapper!.text()).toContain("额度已用尽");
  expect(wrapper!.text()).toContain("2026-10-31 00:00:00 UTC");
  expect(wrapper!.get('.resource-grid dd').attributes("title")).toBe("9007199254740993 / 9007199254740993 B");
  expect(wrapper!.text()).toContain("单向（上下行较大值）");
  expect(wrapper!.find('[aria-label="按用户筛选"]').exists()).toBe(false);
  expect(wrapper!.text()).not.toContain("用户管理");
  await button("新增规则").trigger("click");
  expect(wrapper!.text()).toContain("可用端口：10000–10009");
  await wrapper!.get("#rule-name").setValue("越界规则");
  await wrapper!.get("#listen-port").setValue("9999");
  await wrapper!.get("#target-host").setValue("example.com");
  await wrapper!.get("#target-port").setValue("443");
  await wrapper!.get("form").trigger("submit");
  expect(wrapper!.text()).toContain("监听端口必须在分配范围内");
  expect(calls.some(item => item.options.method === "POST")).toBe(false);
 });
 it("管理端归属、用户筛选和原搜索协议状态组合，迟到请求不能覆盖", async () => {
  await startAdmin();
  expect(wrapper!.get(".desktop-table").text()).toContain("甲用户");
  expect(wrapper!.get(".mobile-cards").text()).toContain("乙用户");
  const select = wrapper!.get('[aria-label="按用户筛选"]');
  expect(select.text()).toContain("管理员");
  let resolveOld!: (value: Response) => void;
  const original = fetch;
  vi.stubGlobal("fetch", vi.fn((path: string, options: RequestInit) => path.includes("owner_id=a") ? new Promise<Response>(resolve => { resolveOld = resolve; }) : original(path, options)));
  await select.setValue("a");
  await select.setValue("b");
  await flushPromises();
  resolveOld(response({ rules: [records[0]] }));
  await flushPromises();
  expect(wrapper!.get(".desktop-table").text()).toContain("two入口");
  expect(wrapper!.get(".desktop-table").text()).not.toContain("one入口");
  await wrapper!.get('[aria-label="搜索规则"]').setValue("two");
  await wrapper!.get('[aria-label="按协议筛选"]').setValue("tcp");
  await wrapper!.get('[aria-label="按状态筛选"]').setValue("quota_exceeded");
  expect(wrapper!.get(".desktop-table").text()).toContain("two入口");
  await wrapper!.get('.desktop-table [role="switch"]').trigger("click"); await flushPromises();
  const write = calls.find(item => item.options.method === "PUT")!;
  expect(write.path).toBe("/api/admin/rules/two");
  expect(new Headers(write.options.headers).get("If-Match")).toBe('"2"');
  expect(JSON.parse(String(write.options.body))).not.toHaveProperty("owner_id");
 });
 it("管理员为指定用户新建时附带归属，并采用该用户端口段", async () => {
  await startAdmin(); await button("新增规则").trigger("click");
  await wrapper!.get("#rule-owner").setValue("a");
  expect(wrapper!.text()).toContain("可用端口：10000–10009");
  await wrapper!.get("#rule-name").setValue("用户入口"); await wrapper!.get("#listen-port").setValue("10003"); await wrapper!.get("#target-host").setValue("example.com"); await wrapper!.get("#target-port").setValue("443");
  await wrapper!.get("form").trigger("submit"); await flushPromises();
  expect(JSON.parse(String(calls.find(item => item.options.method === "POST")!.options.body))).toHaveProperty("owner_id", "a");
  await wrapper!.get('.desktop-table [aria-label="编辑 one入口"]').trigger("click");
  expect(wrapper!.get("#rule-owner").attributes("disabled")).toBeDefined();
 });
 it("套餐精确 GiB、四种计费和固定重置日序列化", async () => {
  await startAdmin(); await button("套餐管理").trigger("click"); await flushPromises();
  expect(wrapper!.get("#accounting-mode").findAll("option")).toHaveLength(4);
  expect(wrapper!.get("#reset-mode").findAll("option")).toHaveLength(3);
  await wrapper!.get("#plan-name").setValue("精确套餐"); await wrapper!.get("#traffic-limit").setValue("8388608.000000000931322574615478515625");
  await wrapper!.get("#accounting-mode").setValue("max"); await wrapper!.get("#reset-mode").setValue("fixed_day"); await wrapper!.get("#reset-day").setValue(31);
  await wrapper!.get("form").trigger("submit"); await flushPromises();
  expect(JSON.parse(String(calls.find(item => item.options.method === "POST")!.options.body))).toMatchObject({ traffic_limit_bytes: "9007199254740993", accounting_mode: "max", reset_mode: "fixed_day", reset_day: 31 });
  expect(wrapper!.text()).toContain("套餐已创建");
 });
 it("用户选择套餐并显示实际分配结果；配置保存使用版本头", async () => {
  await startAdmin(); await button("用户管理").trigger("click"); await flushPromises();
  await wrapper!.get("#new-username").setValue("新用户"); await wrapper!.get("#new-password").setValue("new-password"); await wrapper!.get("#user-plan").setValue(plan.id);
  await wrapper!.get("form").trigger("submit"); await flushPromises();
  expect(wrapper!.text()).toContain("12000–12009"); expect(wrapper!.get("#new-password").element).toHaveProperty("value", "");
  await button("系统配置").trigger("click"); await flushPromises();
  await wrapper!.get("#pool-start").setValue(20000); await wrapper!.get("#pool-end").setValue(30000); await wrapper!.get("form").trigger("submit"); await flushPromises();
  const write = calls.find(item => item.path.endsWith("settings") && item.options.method === "PUT")!;
  expect(new Headers(write.options.headers).get("If-Match")).toBe('"3"');
  expect(JSON.parse(String(write.options.body))).toEqual({ port_start: 20000, port_end: 30000 });
 });
 it("配置版本冲突后显式刷新取得新版本，保留本地化表单校验", async () => {
  await startAdmin(); await button("系统配置").trigger("click"); await flushPromises();
  const original = fetch;
  let revision = 3;
  vi.stubGlobal("fetch", vi.fn((path: string, options: RequestInit = {}) => {
   if (path.endsWith("/settings") && !options.method) return Promise.resolve(response({ port_start: 30000, port_end: 31000, revision: 9 }));
   if (path.endsWith("/settings") && options.method === "PUT" && revision === 3) { revision = 9; return Promise.resolve(response({ error: { code: "revision_mismatch" } }, 412)); }
   return original(path, options);
  }));
  await wrapper!.get("form").trigger("submit"); await flushPromises();
  expect(wrapper!.get('[role="alert"]').text()).toContain("配置已被修改");
  await button("刷新").trigger("click"); await flushPromises();
  expect(wrapper!.get("#pool-start").element).toHaveProperty("value", "30000");
  await wrapper!.get("form").trigger("submit"); await flushPromises();
  const write = calls.find(item => item.path.endsWith("settings") && item.options.method === "PUT")!;
  expect(new Headers(write.options.headers).get("If-Match")).toBe('"9"');
  await button("用户管理").trigger("click"); await flushPromises();
  await wrapper!.get('.language-switcher select').setValue("en");
  expect(wrapper!.get("form").attributes("novalidate")).toBeDefined();
  await wrapper!.get("form").trigger("submit"); await flushPromises();
  expect(wrapper!.text()).toContain("Username must contain 1–80 characters.");
  expect(document.activeElement?.id).toBe("new-username");
 });
 it("跨登录的旧写请求迟到失败不能污染新账号错误或解除新写入忙状态", async () => {
  await startAdmin();
  const original = fetch;
  let resolveRead!: (value: Response) => void;
  let resolveOld!: (value: Response) => void;
  let resolveNew!: (value: Response) => void;
  let delayRead = true, writes = 0;
  vi.stubGlobal("fetch", vi.fn((path: string, options: RequestInit = {}) => {
   if (path === "/api/admin/rules" && !options.method && delayRead) { delayRead = false; return new Promise<Response>(done => { resolveRead = done; }); }
   if (path.endsWith("/rules/one") && options.method === "PUT") {
    writes++;
    return new Promise<Response>(done => { if (writes === 1) resolveOld = done; else resolveNew = done; });
   }
   return original(path, options);
  }));
  await button("刷新").trigger("click");
  await wrapper!.get('.desktop-table [role="switch"]').trigger("click");
  resolveRead(response({ error: { code: "unauthorized" } }, 401)); await flushPromises();
  expect(wrapper!.find("#username").exists()).toBe(true);
  await wrapper!.get("#username").setValue("admin"); await wrapper!.get("#password").setValue("password"); await wrapper!.get("form").trigger("submit"); await flushPromises();
  await wrapper!.get('.desktop-table [role="switch"]').trigger("click");
  resolveOld(response({ error: { code: "listen_conflict", message: "旧请求失败" } }, 409)); await flushPromises();
  expect(wrapper!.get('.desktop-table [role="switch"]').attributes("disabled")).toBeDefined();
  expect(wrapper!.find('[role="alert"]').exists()).toBe(false);
  resolveNew(response({ ...records[0], enabled: false, status: "disabled", revision: 3 })); await flushPromises();
  expect(wrapper!.get('.desktop-table [role="switch"]').attributes("disabled")).toBeUndefined();
 });
 it("GiB 输入拒绝非整数字节和超 int64，但保持安全整数以上的精度", () => {
  expect(gibToBytes("8388608.000000000931322574615478515625")).toBe("9007199254740993");
  expect(gibToBytes("0.5")).toBe("536870912"); expect(gibToBytes("0.1")).toBeUndefined(); expect(gibToBytes("8589934592")).toBeUndefined();
 });
});

describe("客户端会话隔离", () => {
 it("固定请求前缀，CSRF 与退出/401 回调独立", async () => {
  const admin = createClient("admin"), user = createClient("user");
  const adminExpired = vi.fn(), userExpired = vi.fn();
  admin.setSession(adminSession); user.setSession(userSession); admin.onUnauthorized(adminExpired); user.onUnauthorized(userExpired);
  vi.stubGlobal("fetch", vi.fn(async (path: string, options: RequestInit) => { calls.push({ path, options }); return response(null, 204); }));
  await admin.logout(); admin.setSession(); await user.logout();
  expect(calls.map(item => [item.path, new Headers(item.options.headers).get("X-CSRF-Token")])).toEqual([["/api/admin/auth/logout", "admin-token"], ["/api/user/auth/logout", "user-token"]]);
  vi.stubGlobal("fetch", vi.fn(async () => response({ error: { code: "unauthorized" } }, 401)));
  await expect(admin.rules()).rejects.toThrow();
  expect(adminExpired).toHaveBeenCalledOnce(); expect(userExpired).not.toHaveBeenCalled();
 });
 it("会话拒绝另一个角色和缺少身份；旧会话401不能退出新登录", async () => {
  const admin = createClient("admin");
  vi.stubGlobal("fetch", vi.fn(async () => response(userSession)));
  await expect(admin.session()).rejects.toThrow("登录信息无效");
  vi.stubGlobal("fetch", vi.fn(async () => response({ username: "旧账号", csrf_token: "t" })));
  await expect(admin.session()).rejects.toThrow("登录信息无效");
  const expired = vi.fn(); admin.onUnauthorized(expired); admin.setSession(adminSession);
  let resolve!: (value: Response) => void;
  vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>(done => { resolve = done; })));
  const old = admin.rules(); admin.setSession({ ...adminSession, csrf_token: "new-token" });
  resolve(response({}, 401)); await expect(old).rejects.toThrow(); expect(expired).not.toHaveBeenCalled();
 });
});
