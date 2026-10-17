package cli

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"text/tabwriter"

	"github.com/lwl0820/porter/internal/controlapi"
	"github.com/lwl0820/porter/internal/model"
)

func (r *runner) errorMessage(e *controlapi.Error) string {
	messages := map[string][2]string{
		"language_required":      {"尚无语言设置且没有控制终端，请指定 --lang zh-CN 或 --lang en。", "No saved language or controlling terminal; specify --lang en or --lang zh-CN."},
		"language_invalid":       {"语言参数无效，请使用 --lang zh-CN 或 --lang en。", "Invalid language; use --lang en or --lang zh-CN."},
		"preference_invalid":     {"/etc/porter/cli.json 或配置目录不可信、格式或权限无效；请修复，或用 --lang 仅本次指定语言。", "Invalid or unsafe /etc/porter/cli.json or configuration directory; repair its format/permissions, or use --lang for this invocation."},
		"preference_unreadable":  {"无法读取系统语言设置；本次可从终端选择，且不会保存。", "Cannot read the system language setting; a terminal selection applies only to this invocation."},
		"preference_readonly":    {"当前调用不会保存语言设置；请用原生 root/sudo 管理菜单保存。", "This invocation does not save the language; use the native root/sudo management menu to save it."},
		"preference_save_failed": {"语言设置未能保存（权限、配置、安装锁或安装状态变化）；本次继续使用所选语言。", "Could not save the language (permissions, configuration, installation lock, or changed installation state); using the selection for this invocation."},
		"usage_error":            {"命令或参数无效/缺失，请查看 --help：", "Invalid or missing command/argument; see --help: "},
		"no_changes":             {"至少指定一个需要修改的规则字段。", "Specify at least one rule field to update."},
		"menu_json":              {"交互菜单不支持 --json，请指定规则动作。", "The interactive menu does not support --json; specify a rule action."},
		"terminal_required":      {"交互管理需要控制终端，请使用完整参数命令。", "Interactive management requires a controlling terminal; use a complete parameter command."},
		"native_only":            {"规则命令仅支持 Linux 非 Docker 原生环境。", "Rule commands require native Linux outside Docker."},
		"root_required":          {"请使用 root 或 sudo 执行规则命令。", "Run rule commands as root or with sudo."},
		"forbidden":              {"本机服务仅允许真实 root 对端，请使用 sudo。", "The local service accepts only root peers; use sudo."},
		"service_unavailable":    {"无法连接本机服务。请检查 Porter 已运行、控制路径一致，升级后已重启服务；可用 --socket 指定路径。", "Cannot connect to the local service. Check that Porter is running, the control paths match, and the service was restarted after upgrade; use --socket to select a path."},
		"request_timeout":        {"请求超时，操作可能已生效；请先查询结果，不要直接重复提交。", "Request timed out; the operation may have completed. Check its result before submitting again."},
		"invalid_response":       {"本机服务响应无效，请检查程序与服务版本。", "Invalid local service response; check client and service versions."},
		"validation_error":       {"规则字段无效，请检查下列字段。", "Invalid rule configuration; check the fields below."},
		"not_found":              {"规则或所属用户不存在，请重新查询。", "The rule or owner does not exist; reload the list."},
		"revision_mismatch":      {"规则已被修改，请重新加载后操作。", "The rule changed; reload before trying again."},
		"revision_required":      {"请提供当前规则版本。", "Supply the current rule revision."},
		"revision_invalid":       {"规则版本必须为正整数。", "The rule revision must be a positive integer."},
		"rule_conflict":          {"监听地址与现有规则冲突（TCP/UDP）。", "The listener overlaps an existing rule (TCP/UDP)."},
		"listen_conflict":        {"无法准备监听（TCP/UDP）：端口占用、权限不足或地址不可用；请检查后重试。", "Cannot prepare listener (TCP/UDP): port occupied, insufficient permission, or unavailable address. Check and retry."},
		"admin_conflict":         {"TCP 监听与管理端口冲突。", "The TCP listener overlaps the management port."},
		"port_reserved":          {"数字端口已分配给用户。", "The numeric port is reserved for a user."},
		"quota_exceeded":         {"所属用户流量额度已耗尽或正在重置。", "The owner's traffic quota is exhausted or being reset."},
		"rule_limit":             {"最多保存 1000 条规则。", "At most 1000 rules can be saved."},
		"forward_loop":           {"目标不能指向规则自身的监听地址。", "The target must not point to the rule's own listener."},
		"database_error":         {"持久化失败，原配置保持不变。", "Persistence failed; the previous configuration is preserved."},
		"shutting_down":          {"服务正在停止，请稍后查询。", "The service is shutting down; check again later."},
		"json_required":          {"请求必须使用 JSON。", "The request must use JSON."},
		"invalid_json":           {"请求格式或字段无效。", "The request format or fields are invalid."},
		"listen_failed":          {"监听未启动，请检查地址、端口占用及权限。", "Listener failed; check address, occupied ports, and permissions."},
		"quota_reset_pending":    {"流量周期正在重置，请稍后查询。", "The traffic cycle is being reset; check again later."},
		"internal_error":         {"操作失败，请检查服务日志后重试。", "Operation failed; check the service logs before retrying."},
	}
	pair, ok := messages[e.Code]
	if !ok {
		pair = messages["internal_error"]
	}
	result := r.text(pair[0], pair[1])
	if e.Code == "usage_error" {
		result += e.Message
	}
	return result
}
func (r *runner) fieldError(key string) string {
	fields := map[string][2]string{
		"name":        {"名称需要 1–80 个字符。", "Use 1–80 characters for the name."},
		"protocol":    {"请选择 tcp、udp 或 tcp+udp。", "Choose tcp, udp, or tcp+udp."},
		"listen_host": {"请输入本机 IPv4 或 IPv6 地址。", "Enter a local IPv4 or IPv6 address."},
		"listen_port": {"请输入 1–65535 且符合归属端口范围的端口。", "Enter a port from 1–65535 within the owner's allocated range."},
		"target_host": {"请输入单播 IP 或域名，不包含协议、路径或端口。", "Enter a unicast IP or hostname without scheme, path, or port."},
		"target_port": {"端口范围为 1–65535。", "Use a port from 1–65535."},
	}
	pair, ok := fields[key]
	if !ok {
		return r.text("字段值无效。", "Invalid field value.")
	}
	return r.text(pair[0], pair[1])
}
func (r *runner) help() {
	fmt.Fprintln(r.out, r.text("用法：porter [rules [动作 [ID]]] [参数]\n裸命令或不带动作进入交互菜单；参数方式缺少字段会直接报错。", "Usage: porter [rules [ACTION [ID]]] [OPTIONS]\nUse bare porter or omit ACTION for the interactive menu; parameter commands never prompt for missing fields."))
	fmt.Fprintln(r.out, "  add     --name NAME --protocol tcp|udp|tcp+udp --listen-host IP --listen-port PORT --target-host HOST --target-port PORT")
	fmt.Fprintln(r.out, "          [--enabled=true|false] [--traffic-enabled=true|false] [--owner-id ID]")
	fmt.Fprintln(r.out, "  list    [--owner-id ID]\n  get ID\n  update ID [--name NAME] [--protocol tcp|udp|tcp+udp] [--listen-host IP] [--listen-port PORT]")
	fmt.Fprintln(r.out, "            [--target-host HOST] [--target-port PORT] [--enabled=true|false] [--traffic-enabled=true|false] [--revision N]\n  delete ID [--revision N]")
	fmt.Fprintln(r.out, r.text("delete 立即删除，无二次确认；菜单删除需确认。\n通用参数可放在动作前或后：--lang zh-CN|en、--socket PATH、--json、--help。\n未指定 --lang 时使用 /etc/porter/cli.json 保存的语言；首次从终端选择，回车默认英文。\n--lang 仅覆盖本次；菜单可切换并保存语言。无保存值的无人值守调用须指定 --lang。\n--json 仅支持参数方式。新增默认启用、展示统计关闭、归属管理员。\nupdate 至少修改一个字段，未传字段及归属保留；发生版本冲突需重新加载。\n仅原生 Linux root/sudo 可操作运行中的服务，无需 Web 密码。", "delete removes immediately without confirmation; the menu asks for confirmation.\nCommon options may precede or follow ACTION: --lang zh-CN|en, --socket PATH, --json, --help.\nWithout --lang, use the language saved in /etc/porter/cli.json; on first use, choose on the terminal (Enter defaults to English).\n--lang overrides this invocation only; change and save language in the menu. Unattended calls without a saved setting require --lang.\n--json requires a parameter action. New rules default to enabled, traffic display off, and administrator ownership.\nupdate requires at least one changed field; omitted fields and ownership are preserved. Reload on revision conflicts.\nNative Linux root/sudo only; requires the running service, without a Web password."))
}
func (r *runner) state(value string) string {
	switch value {
	case "running":
		return r.text("运行中", "Running")
	case "disabled":
		return r.text("已停用", "Disabled")
	case "error":
		return r.text("错误", "Error")
	case "quota_exceeded":
		return r.text("额度耗尽", "Quota exhausted")
	}
	return value
}
func (r *runner) yes(value bool) string {
	if value {
		return r.text("是", "Yes")
	}
	return r.text("否", "No")
}
func (r *runner) render(w io.Writer, value any) {
	switch v := value.(type) {
	case model.Rule:
		r.detail(w, v)
	case controlapi.RuleList:
		r.table(w, v.Rules)
	case map[string]any:
		fmt.Fprintln(w, r.text("已删除规则：", "Deleted rule: "), v["deleted"])
	}
}
func (r *runner) table(w io.Writer, items []model.Rule) {
	if len(items) == 0 {
		fmt.Fprintln(w, r.text("暂无规则。", "No rules."))
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, r.text("序号\tID\t名称\t协议\t监听\t目标\t启用\t状态\t归属\t版本", "No.\tID\tName\tProtocol\tListener\tTarget\tEnabled\tStatus\tOwner\tRevision"))
	for i, v := range items {
		fmt.Fprintf(tw, "%d\t%s\t%q\t%s\t%s\t%s\t%s\t%s\t%s\t%d\n", i+1, v.ID, v.Name, v.Protocol, net.JoinHostPort(v.ListenHost, strconv.Itoa(v.ListenPort)), net.JoinHostPort(v.TargetHost, strconv.Itoa(v.TargetPort)), r.yes(v.Enabled), r.state(v.Status), v.OwnerID, v.Revision)
	}
	_ = tw.Flush()
}
func (r *runner) detail(w io.Writer, v model.Rule) {
	r.table(w, []model.Rule{v})
	fmt.Fprintf(w, "%s: %s\n", r.text("展示统计", "Traffic display"), r.yes(v.TrafficEnabled))
	fmt.Fprintf(w, "%s: %d B; %s: %d B\n", r.text("上传累计", "Uploaded"), v.Traffic.UploadBytes, r.text("下载累计", "Downloaded"), v.Traffic.DownloadBytes)
	fmt.Fprintf(w, "%s: %.2f B/s; %s: %.2f B/s\n", r.text("上传速度", "Upload rate"), v.Traffic.UploadBytesPerSecond, r.text("下载速度", "Download rate"), v.Traffic.DownloadBytesPerSecond)
	fmt.Fprintf(w, "%s: %s\n%s: %s\n", r.text("创建时间", "Created"), v.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), r.text("更新时间", "Updated"), v.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
	if v.Owner != nil {
		fmt.Fprintf(w, "%s: %q (%s)\n", r.text("所属账号", "Owner account"), v.Owner.Username, r.text(map[string]string{"admin": "管理员", "user": "用户"}[v.Owner.Role], map[string]string{"admin": "Administrator", "user": "User"}[v.Owner.Role]))
	}
	if v.LastErrorCode != "" {
		fmt.Fprintf(w, "%s: %s [%s]\n", r.text("监听错误", "Listener error"), r.errorMessage(&controlapi.Error{Code: v.LastErrorCode}), v.LastErrorCode)
	}
}
