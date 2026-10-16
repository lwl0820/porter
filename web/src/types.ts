export type Protocol = "tcp" | "udp";
export type RuleStatus = "running" | "disabled" | "error";

export interface RuleInput {
  name: string;
  protocol: Protocol;
  listen_host: string;
  listen_port: number;
  target_host: string;
  target_port: number;
  enabled: boolean;
}

export interface Rule extends RuleInput {
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

export const statusLabels: Record<RuleStatus, string> = {
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
  };
}
