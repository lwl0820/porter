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
export type RuleStatus = "running" | "disabled" | "error";

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
  traffic: Traffic;
  id: string;
  revision: number;
  created_at: string;
  updated_at: string;
  status: RuleStatus;
  last_error_code?: string;
  last_error?: string;
}

export interface Session {
  username: string;
  csrf_token: string;
}
export type FieldErrors = Partial<Record<keyof RuleInput, string>>;

export function address(host: string, port: number): string {
  return `${host.includes(":") ? `[${host}]` : host}:${port}`;
}

export const statusLabels: Record<RuleStatus, MessageKey> = {
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
