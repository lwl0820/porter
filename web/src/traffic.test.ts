import { afterEach, describe, expect, it, vi } from "vitest";
import { adminApi as api } from "./api";
import { formatBytes, formatSpeed, usagePercent } from "./traffic";

const rule = {
  owner_id: "admin", owner: { id: "admin", username: "admin", role: "admin" },
  id: "r", name: "规则", protocol: "tcp", listen_host: "127.0.0.1", listen_port: 1234,
  target_host: "example.com", target_port: 443, enabled: true, traffic_enabled: true,
  revision: 1, created_at: "2026-10-17T00:00:00Z", updated_at: "2026-10-17T00:00:00Z", status: "running",
  traffic: { upload_bytes: "9007199254740993", download_bytes: "0", upload_bytes_per_second: 1536, download_bytes_per_second: 0 },
};

afterEach(() => vi.unstubAllGlobals());

describe("流量格式与边界", () => {
  it("大整数字节的进度按整数计算并限制为有效百分比", () => {
    expect(usagePercent("9007199254740992", "9007199254740993")).toBe(99.99);
    expect(usagePercent("4611686018427387903", "9223372036854775807")).toBe(49.99);
    expect(usagePercent("999", "10")).toBe(100);
    expect(usagePercent("0", "0")).toBe(0);
    expect(usagePercent("0", "9223372036854775807")).toBe(0);
  });
  it("以 BigInt 格式化累计量，并显示速度单位", () => {
    expect(formatBytes("0")).toBe("0 B");
    expect(formatBytes("1023")).toBe("1023 B");
    expect(formatBytes("1536")).toBe("1.5 KiB");
    expect(formatBytes("9223372036854775807")).toBe("8.0 EiB");
    expect(formatSpeed(0)).toBe("0 B/s");
    expect(formatSpeed(1536)).toBe("1.5 KiB/s");
  });
  it("累计量超过 JS 安全整数仍保留十进制原文", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ rules: [rule] }))));
    const rules = await api.rules();
    expect(rules[0]!.traffic.upload_bytes).toBe("9007199254740993");
  });
  it.each(["-1", "1.5", "01", "NaN", "9223372036854775808", 123])("拒绝非法累计量 %s", async (value) => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ rules: [{ ...rule, traffic: { ...rule.traffic, upload_bytes: value } }] }))));
    await expect(api.rules()).rejects.toThrow("规则格式无效");
  });
  it.each([-1, null, "12"])("拒绝非法速度 %s", async (value) => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ rules: [{ ...rule, traffic: { ...rule.traffic, upload_bytes_per_second: value } }] }))));
    await expect(api.rules()).rejects.toThrow("规则格式无效");
  });
});
