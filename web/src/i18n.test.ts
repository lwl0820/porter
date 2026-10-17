import { afterEach, describe, expect, it } from "vitest";
import { ApiError, errorMessage, fieldErrorMessage } from "./api";
import { setLanguage, t } from "./i18n";

afterEach(() => {
  setLanguage("zh-CN");
  localStorage.clear();
});

describe("错误本地化边界", () => {
  it("错误码提供稳定提示，保留错误原始数据供切换重译", () => {
    const failure = new ApiError("服务端原始错误", 403, "csrf_invalid");
    setLanguage("en");
    expect(errorMessage(failure)).toBe("Request verification failed. Refresh the page and try again.");
    setLanguage("zh-CN");
    expect(errorMessage(failure)).toBe("请求校验失败，请刷新页面后重试");
    expect(failure.message).toBe("服务端原始错误");
    expect(failure.status).toBe(403);
  });

  it("版本错误始终引导关闭表单刷新，未知错误使用当前语言通用提示", () => {
    const failure = new ApiError("任意错误", 412, "unknown");
    setLanguage("en");
    expect(errorMessage(failure)).toContain("Close the form and refresh");
    expect(errorMessage(new ApiError("未识别的服务端错误", 500, "future_code"))).toBe("Operation failed. Please try again later.");
    expect(errorMessage(new Error("内部信息"))).toBe("Operation failed. Please try again later.");
    expect(fieldErrorMessage("target_host", "未来新增限制")).toBe("Please check this field.");
    expect(fieldErrorMessage("listen_port", "未知端口错误")).toBe("Please check this field.");
  });

  it("组合协议冲突和不同数据库失败原因不丢失", () => {
    setLanguage("en");
    expect(errorMessage(new ApiError("TCP+UDP 监听地址和端口已被规则使用", 409, "rule_conflict"))).toContain("TCP+UDP listen address and port");
    expect(errorMessage(new ApiError("规则保存失败，原配置保持不变", 500, "database_error"))).toBe("Saving failed. The previous configuration is unchanged.");
    expect(errorMessage(new ApiError("删除失败，规则保持不变", 500, "database_error"))).toBe("Deletion failed. The rule is unchanged.");
    expect(errorMessage(new ApiError("重置失败，原统计保持不变", 500, "database_error"))).toBe("Reset failed. The previous counters are unchanged.");
    expect(errorMessage(new ApiError("请求只能包含一个 JSON 对象", 400, "invalid_json"))).toContain("exactly one JSON object");
    expect(fieldErrorMessage("target_host", "目标必须是单播地址或域名")).toContain("unicast");
    expect(fieldErrorMessage("listen_host", "请输入本机 IPv4 或 IPv6 地址")).toContain("local IPv4 or IPv6");
    expect(fieldErrorMessage("target_port", "端口范围为 1–65535")).toContain("between 1 and 65535");
  });

  it("插值保持用户原始值，英文计数使用单复数", () => {
    setLanguage("en");
    expect(t("确定删除 {name} 吗？", { name: "应用 {total}" })).toBe("Delete 应用 {total}?");
    expect(t("共 {total} 条规则", { total: 1 })).toBe("1 rule");
    expect(t("共 {total} 条规则", { total: 2 })).toBe("2 rules");
    setLanguage("zh-CN");
    expect(t("共 {total} 条规则", { total: 1 })).toBe("共 1 条规则");
  });
});
