import type { MessageKey } from "./i18n";

export const protocols = ["tcp", "udp", "tcp+udp"] as const;
export type Protocol = (typeof protocols)[number];

export const protocolDetails: Record<
  Protocol,
  {
    label: string;
    description: MessageKey;
    className: string;
    icon: string;
  }
> = {
  tcp: {
    label: "TCP",
    description: "可靠的连接传输",
    className: "tcp",
    icon: "arrows",
  },
  udp: {
    label: "UDP",
    description: "轻量的数据报传输",
    className: "udp",
    icon: "network",
  },
  "tcp+udp": {
    label: "TCP+UDP",
    description: "同时转发两种协议",
    className: "both",
    icon: "layers",
  },
};

export function isProtocol(value: unknown): value is Protocol {
  return protocols.some((protocol) => protocol === value);
}

export function matchesProtocol(
  value: Protocol,
  filter: Protocol | "",
): boolean {
  return (
    !filter || value === filter || (value === "tcp+udp" && filter !== "tcp+udp")
  );
}
export type RuleStatus = "running" | "disabled" | "error" | "quota_exceeded";

export interface RuleInput {
  name: string;
  protocol: Protocol;
  listen_host: string;
  listen_port: number;
  target_host: string;
  target_port: number;
  enabled: boolean;
  traffic_enabled: boolean;
}

export interface Traffic {
  upload_bytes: string;
  download_bytes: string;
  upload_bytes_per_second: number;
  download_bytes_per_second: number;
}

export interface Rule extends RuleInput {
  owner_id: string;
  owner?: Owner;
  traffic: Traffic;
  id: string;
  revision: number;
  created_at: string;
  updated_at: string;
  status: RuleStatus;
  last_error_code?: string;
  last_error?: string;
}

export type Role = "admin" | "user";
export interface Owner { id: string; username: string; role: Role }
export interface Session {
  user_id: string;
  role: Role;
  username: string;
  csrf_token: string;
}
export type FieldErrors = Partial<Record<string, string>>;

export function address(host: string, port: number): string {
  return `${host.includes(":") ? `[${host}]` : host}:${port}`;
}

export const statusLabels: Record<RuleStatus, MessageKey> = {
  quota_exceeded: "额度已用尽",
  running: "运行中",
  disabled: "已停用",
  error: "错误",
};

export function ruleInput(rule: Rule): RuleInput {
  return {
    name: rule.name,
    protocol: rule.protocol,
    listen_host: rule.listen_host,
    listen_port: rule.listen_port,
    target_host: rule.target_host,
    target_port: rule.target_port,
    enabled: rule.enabled,
    traffic_enabled: rule.traffic_enabled,
  };
}

export type AccountingMode = "both" | "max" | "upload" | "download";
export type ResetMode = "calendar_month" | "anniversary" | "fixed_day";
export interface PlanInput { name: string; port_count: number; traffic_limit_bytes: string; accounting_mode: AccountingMode; reset_mode: ResetMode; reset_day: number | null }
export interface Plan extends PlanInput { id: string; created_at: string }
export interface User extends Owner { plan_id: string | null; port_start: number; port_end: number; created_at: string }
export interface Usage { cycle_start: string; cycle_end: string; upload_bytes: string; download_bytes: string; used_bytes: string; limit_bytes: string; exceeded: boolean }
export interface Account { user: User; plan: Plan | null; usage: Usage | null }
export interface Settings { port_start: number; port_end: number; revision: number }
export const accountingLabels: Record<AccountingMode, MessageKey> = { both: "双向（上行＋下行）", max: "单向（上下行较大值）", upload: "只计上行", download: "只计下行" };
export const resetLabels: Record<ResetMode, MessageKey> = { calendar_month: "自然月重置", anniversary: "按开户日期每月重置", fixed_day: "固定日期重置" };
