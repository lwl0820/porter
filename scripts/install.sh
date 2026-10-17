#!/usr/bin/env bash
# 管道仅提供脚本，交互始终读取控制终端。
set +x
set -Eeuo pipefail
umask 077

LANGUAGE=en
message() {
    local key=$1 format=$1
    shift
    if [[ $LANGUAGE == en ]]; then
        case "$key" in
            # 英文词典在此集中维护，中文键也作为默认语言模板。
            '错误：') format='Error: ' ;;
            '终端输入已结束。') format='Terminal input ended.' ;;
            '语言必须是 zh-CN 或 en。') format='Language must be zh-CN or en.' ;;
            '仅支持 Linux。') format='Only Linux is supported.' ;;
            '无法读取系统发行信息。') format='Cannot read operating system information.' ;;
            '不支持此系统版本。') format='This operating system version is unsupported.' ;;
            '不支持的发行版或版本：%s %s。') format='Unsupported distribution or version: %s %s.' ;;
            '仅支持 amd64 和 arm64。') format='Only amd64 and arm64 are supported.' ;;
            '需要以 systemd 为 PID 1 的真实 Linux 宿主。') format='Requires a Linux host with systemd as PID 1.' ;;
            '缺少 systemctl。') format='systemctl is missing.' ;;
            '管理员用户名：') format='Admin username: ' ;;
            '用户名不能为空，请重新输入。\n') format='Username cannot be empty. Try again.\n' ;;
            '管理员密码：') format='Admin password: ' ;;
            '\n确认密码：') format='\nConfirm password: ' ;;
            '密码不能全部为空白，请重新输入。\n') format='Password cannot contain only whitespace. Try again.\n' ;;
            '两次密码不一致，请重新输入。\n') format='Passwords do not match. Try again.\n' ;;
            'Web 监听地址（如 0.0.0.0:8080 或 [::]:8080）：') format='Web listen address (e.g. 0.0.0.0:8080 or [::]:8080): ' ;;
            '请输入有效的 IPv4:端口 或 [IPv6]:端口，端口范围 1–65535。\n') format='Enter a valid IPv4:port or [IPv6]:port with port 1-65535.\n' ;;
            '现有 porter 账号不符合专用服务账号要求。') format='The existing porter account is not a dedicated service account.' ;;
            '专用 porter 组缺失。') format='The dedicated porter group is missing.' ;;
            'porter 用户与组不匹配。') format='The porter user and group do not match.' ;;
            '缺少 nologin，无法创建禁止登录的服务账号。') format='nologin is missing; cannot create a non-login service account.' ;;
            '安装目录必须是无控制字符的绝对路径。') format='Installation directory must be absolute and contain no control characters.' ;;
            '安装目录不能包含引号或反斜线（systemd 路径限制）。') format='Installation directory cannot contain quotes or backslashes (systemd restriction).' ;;
            '专用 porter 账号缺失，请先检查安装状态。') format='The dedicated porter account is missing. Check the installation state.' ;;
            'porter 组被其他账号使用，拒绝删除。') format='The porter group is used by other accounts; refusing removal.' ;;
            '服务仍在运行，未删除任何剩余文件。') format='Service is still running; no remaining files were deleted.' ;;
            '无法确认服务已停止，请检查 systemd。') format='Cannot confirm that the service stopped. Check systemd.' ;;
            '拒绝使用符号链接路径。') format='Refusing symbolic links in managed paths.' ;;
            '无法查询 systemd 服务。') format='Cannot query the systemd service.' ;;
            '发现已有 porter.service，拒绝覆盖未知服务。') format='An existing porter.service was found; refusing to take over an unknown service.' ;;
            '发现未托管的已有文件：%s。') format='Unmanaged existing resource: %s.' ;;
            '发现未知的 porter 账号或组，拒绝接管。') format='Unknown porter user or group found; refusing to take over.' ;;
            'Porter 已卸载，无需重复操作。\n') format='Porter is already uninstalled. No action needed.\n' ;;
            '另一个安装进程正在写入全局服务配置。') format='Another process has locked the global service configuration.' ;;
            '安装目录被锁定，请确认没有其他安装进程。') format='Installation directory is locked. Check for another installer process.' ;;
            '程序目录不是目录。') format='Program directory is not a directory.' ;;
            '安装状态无效或目录/架构不匹配，未覆盖已有文件。') format='Invalid installation state or directory/architecture mismatch; existing files were kept.' ;;
            '发现其他来源的 porter.service，拒绝覆盖。') format='porter.service belongs to another source; refusing to overwrite.' ;;
            '发现自定义 systemd 覆盖配置，拒绝卸载。') format='Custom systemd drop-ins found; refusing uninstall.' ;;
            '现有 unit 与本安装器不符，拒绝覆盖。') format='Existing unit does not match this installer; refusing to overwrite.' ;;
            '托管服务文件缺失，请先检查安装状态。') format='Managed service file is missing. Check the installation state.' ;;
            '已有程序不可执行，拒绝覆盖。') format='Existing program is not executable; refusing to overwrite.' ;;
            '程序版本与发布标签不符。') format='Program version does not match the release tag.' ;;
            '托管程序缺失，请先检查安装状态。') format='Managed program is missing. Check the installation state.' ;;
            '托管配置或数据的文件类型不符，拒绝卸载。') format='Managed configuration or data has an unexpected file type; refusing uninstall.' ;;
            '已卸载状态存在意外程序或服务，请先检查。') format='Unexpected program or service exists in the uninstalled state. Check it first.' ;;
            '卸载需要可交互终端，请在 SSH 终端运行。') format='Uninstall requires an interactive terminal. Run it in an SSH terminal.' ;;
            '将停止并禁用 porter.service，删除程序 %s 和服务文件 %s。\n') format='Will stop and disable porter.service, remove program %s and unit %s.\n' ;;
            '同时清理未提交的暂存文件：%s、%s、%s、%s。\n') format='Also remove uncommitted staging files: %s, %s, %s, %s.\n' ;;
            '彻底删除还将清除配置 %s、完整数据目录 %s、安装记录和专用账号/组。\n') format='Purge also removes configuration %s, the entire data directory %s, installation record and service user/group.\n' ;;
            '保留配置 %s、完整数据目录 %s、安装记录和专用账号/组。\n') format='Keep configuration %s, the entire data directory %s, installation record and service user/group.\n' ;;
            '确认卸载？输入 yes 继续，回车取消：') format='Confirm uninstall? Type yes to continue, or Enter to cancel: ' ;;
            '已取消，未修改安装。\n') format='Cancelled. Installation was not changed.\n' ;;
            '账号、规则及统计将永久丢失。输入 DELETE 确认彻底删除：') format='Accounts, rules and statistics will be permanently lost. Type DELETE to confirm purge: ' ;;
            '停止服务失败，请重试卸载；尚未删除文件。') format='Failed to stop service. Retry uninstall; files have not been deleted.' ;;
            '禁用开机启动失败，请重试卸载；尚未删除文件。') format='Failed to disable service. Retry uninstall; files have not been deleted.' ;;
            'systemd 重新加载失败，请重试卸载。') format='systemd reload failed. Retry uninstall.' ;;
            'Porter 已彻底删除。\n') format='Porter has been purged.\n' ;;
            'Porter 已卸载；配置、数据库和专用账号已保留。\n重新运行新版安装脚本可恢复原版本（需要原发行包仍可下载）。\n') format='Porter uninstalled; configuration, databases and service account were retained.\nRun the current installer again to restore the original version (requires the original release assets).\n' ;;
            '命令执行失败，请检查上方诊断后重试。') format='Command failed. Check the diagnostics above and retry.' ;;
            '%s 缺少值。') format='Missing value for %s.' ;;
            '未知参数：%s。') format='Unknown argument: %s.' ;;
            '--purge 仅可与 --uninstall 一起使用。') format='--purge requires --uninstall.' ;;
            '卸载不能指定 --version。') format='--version cannot be used with --uninstall.' ;;
            '版本格式应为 v1.0.0 或明确的预发布标签。') format='Version must be v1.0.0 or an explicit prerelease tag.' ;;
            '请使用 root 或 sudo 账号。') format='Use a root or sudo account.' ;;
            '无法取得 sudo 权限。') format='Cannot obtain sudo privileges.' ;;
            '卸载需要 Python 3，请先安装后重试。') format='Uninstall requires Python 3. Install it first and retry.' ;;
            '不能安装到根目录。') format='Cannot use the root directory.' ;;
            '程序目录不能放在 Porter 数据或配置目录内。') format='Program directory cannot be inside the Porter data or configuration directory.' ;;
            '拒绝使用符号链接路径：%s。') format='Refusing symbolic link path: %s.' ;;
            '首次配置需要可交互终端，请在 SSH 终端运行。') format='Initial setup requires an interactive terminal. Run it in an SSH terminal.' ;;
            '已有安装不能通过 --version 升级，请按 README 显式升级。') format='Cannot upgrade an existing installation with --version. Follow the README upgrade steps.' ;;
            '卸载尚未完成，请先重新运行 --uninstall。') format='Uninstall is incomplete. Run --uninstall again first.' ;;
            '完整安装的文件、数据目录或数据库缺失，请恢复备份；安装器不会重置配置或数据。') format='Complete installation is missing files, data directory or database. Restore a backup; configuration and data will not be reset.' ;;
            '保留的配置或数据库缺失，请恢复备份。') format='Retained configuration or database is missing. Restore a backup.' ;;
            '现有环境配置无效；未重置任何凭据。') format='Existing environment configuration is invalid; credentials were not reset.' ;;
            '无法获取公开 GitHub Release；请先确认正式版本已成功发布（尚无版本时 API 返回 404）。') format='Cannot fetch public GitHub Release. Check that a stable version was published (the API returns 404 when none exists).' ;;
            'Release 缺少匹配架构的发行包或 SHA256SUMS。') format='Release is missing the matching architecture archive or SHA256SUMS.' ;;
            '发行包下载失败。') format='Release archive download failed.' ;;
            '校验和下载失败。') format='Checksum download failed.' ;;
            '发行包摘要不符或不含唯一的 porter 普通文件。') format='Archive checksum mismatch or archive lacks a unique regular porter file.' ;;
            '安装期间出现已有安装状态，请重新运行。') format='Installation state appeared during installation. Run again.' ;;
            '安装状态已变化，请重新运行；未继续修改托管资源。') format='Installation state changed. Run again; no further managed resources were modified.' ;;
            '安装期间出现已有文件：%s。') format='Existing resource appeared during installation: %s.' ;;
            '安装期间出现同名账号或组，拒绝接管。') format='A user or group with the same name appeared during installation; refusing to take over.' ;;
            'systemd 启动失败，请检查 journalctl -u porter.service。') format='systemd start failed. Check journalctl -u porter.service.' ;;
            '服务未就绪，请检查 journalctl -u porter.service 和监听设置。') format='Service is not ready. Check journalctl -u porter.service and the listen address.' ;;
            '\nPorter 已就绪\n用户入口：%s/user/\n管理员入口：%s/admin/\n通配监听时，远程访问请改用服务器 IP。\n') format='\nPorter is ready\nUser portal: %s/user/\nAdmin portal: %s/admin/\nFor remote access with a wildcard listen address, use the server IP.\n' ;;
            '配置：%s（权限 600）；数据：%s；程序：%s\n') format='Configuration: %s (mode 600); data: %s; program: %s\n' ;;
            '日志：%sjournalctl -u porter.service -f\n停止：%ssystemctl stop porter.service\n启动：%ssystemctl start porter.service\n') format='Logs: %sjournalctl -u porter.service -f\nStop: %ssystemctl stop porter.service\nStart: %ssystemctl start porter.service\n' ;;
            '卸载不能指定 --upgrade。') format='--upgrade cannot be used with --uninstall.' ;;
            'CLI 配置类型、属主或权限无效。') format='CLI configuration type, ownership or permissions are invalid.' ;;
            '系统命令入口存在未知文件、链接或不可信目录。') format='System command conflicts with an unknown file, link or untrusted directory.' ;;
            '系统命令入口创建失败，未覆盖已有文件。') format='Cannot create the system command; existing files were not overwritten.' ;;
            '版本：%s；服务：%s；命令：%s rules（需要 root 或 sudo）。\n') format='Version: %s; service: %s; command: %s rules (requires root or sudo).\n' ;;
            '版本：%s；旧单程序布局不包含独立命令行工具。请使用 --upgrade 显式升级。\n') format='Version: %s; legacy layout has no standalone CLI. Use --upgrade to migrate explicitly.\n' ;;
            '升级恢复失败；请检查安装记录及备份后重试。') format='Upgrade recovery failed; inspect the installation record and backup before retrying.' ;;
            '已恢复稳定安装，请重新运行所需安装、升级或卸载命令。\n') format='Stable installation restored. Run the desired install, upgrade or uninstall command again.\n' ;;
            '--upgrade 需要已完成的托管安装。') format='--upgrade requires a completed managed installation.' ;;
            '目标版本无效或低于当前版本，拒绝降级。') format='Target version is invalid or older than the installed version; downgrade refused.' ;;
            '发行包摘要不符或缺少唯一的双程序普通文件。') format='Archive checksum mismatch or missing unique regular porter and porterd programs.' ;;
            '升级需要可交互终端，请在 SSH 终端运行。') format='Upgrade requires an interactive terminal. Run it in an SSH terminal.' ;;
            '将从 %s 升级到 %s，期间短暂停服并备份完整配置和数据。\n输入 yes 确认升级，回车取消：') format='Upgrade from %s to %s, with a brief outage and a full configuration and data backup.\nType yes to confirm upgrade, or Enter to cancel: ' ;;
            '升级失败，已尝试恢复原版本；请查看恢复诊断。') format='Upgrade failed; recovery of the original version was attempted. Check the recovery diagnostics.' ;;
            '历史升级备份可能包含凭据，仍保留在 %s；失败数据副本可能包含业务数据，仍保留在 %s/.porter-failed-upgrade.<标识> 中。请核对清理。\n') format='Historical upgrade backups may contain credentials and remain at %s; failed data copies may contain business data and remain at %s/.porter-failed-upgrade.<id>. Review and clean them manually.\n' ;;
            '数据目录本身是挂载根或无法校验挂载信息，不能安全执行升级回退。') format='The data directory is a mount root or mount information cannot be verified; upgrade rollback would be unsafe.' ;;
            '是否启用 Web 服务？[%s]：') format='Enable Web service? [%s]: ' ;;
            '是否安装命令行工具？[%s]：') format='Install command-line tool? [%s]: ' ;;
            '请输入 yes 或 no（也可输入 y 或 n）。\n') format='Enter yes or no (y or n is also accepted).\n' ;;
            '至少启用一种管理方式，请重新选择。\n') format='Enable at least one management method. Choose again.\n' ;;
            '是否修改管理组件？[no]：') format='Change management components? [no]: ' ;;
            '当前组件 Web=%s CLI=%s；目标 Web=%s CLI=%s。将备份配置及数据并重启服务，现有连接可能断开。\n备份保留在 %s/.tmp/upgrade.<标识>。输入 yes 确认，回车取消：') format='Current components Web=%s CLI=%s; target Web=%s CLI=%s. Configuration and data will be backed up and the service restarted; existing connections may close.\nBackup remains at %s/.tmp/upgrade.<id>. Type yes to confirm, or Enter to cancel: ' ;;
            '当前版本不支持组件选择，请先使用 --upgrade 显式升级到支持的版本。') format='This version does not support component selection. First use --upgrade to install a supporting version.' ;;
            '候选程序不支持组件配置，或候选配置预检失败；未停止服务。') format='Candidate program lacks component configuration support, or configuration preflight failed; service was not stopped.' ;;
            '版本：%s；服务：%s；命令行工具未启用。\n') format='Version: %s; service: %s; command-line tool is disabled.\n' ;;
            '\nPorter 已就绪（仅命令行，无 Web 监听）。\n') format='\nPorter is ready (command-line only, no Web listener).\n' ;;
            '组件切换失败，已尝试恢复原安装；请查看恢复诊断。') format='Component change failed; recovery of the original installation was attempted. Check recovery diagnostics.' ;;
            'Web 监听与已有转发规则冲突，或无法可靠读取规则；未停止服务。') format='Web listener conflicts with an existing forwarding rule, or rules could not be verified; service was not stopped.' ;;
            '当前组件 Web=%s CLI=%s。\n') format='Current components Web=%s CLI=%s.\n' ;;
            '组件未改变，已保留当前服务状态。\n') format='Components unchanged; current service state was retained.\n' ;;
            '安装完整，服务保持停止状态。\n') format='Installation is complete; service remains stopped.\n' ;;
            *) format='Operation failed.' ;;
        esac
    fi
    printf -- "$format" "$@"
}
fail() { message '错误：' >&2; message "$@" >&2; printf '\n' >&2; exit 1; }
help() {
    if [[ $LANGUAGE == en ]]; then
        cat <<'HELP'
Porter native interactive installer and uninstaller
Usage: bash install.sh [--install-dir /opt/porter] [--version v1.0.0] [--lang zh-CN|en]
       bash install.sh --upgrade [--version v1.0.0] [--install-dir /opt/porter] [--lang zh-CN|en]
       bash install.sh --uninstall [--purge] [--install-dir /opt/porter] [--lang zh-CN|en]
Install the latest stable GitHub Release; choose Web and CLI (both enabled by default, at least one).
Only Web requires an admin username, hidden password and Web IP:port.
Supports Ubuntu 22.04/24.04, Debian 12/13, RHEL/Rocky/AlmaLinux 9/10, amd64/arm64.
Requires systemd as PID 1 and root or sudo; no Docker or compiler needed.
Repeating installation keeps the version and can change components after confirmation; --upgrade explicitly upgrades.
Native releases contain porterd (service) and optional porter (CLI), with /usr/bin/porter on PATH when enabled.
Initial language selection: 1 English, 2 Chinese; Enter defaults to English.
CLI preferences in /etc/porter/cli.json are independent and retained by normal uninstall.
Uninstall keeps configuration, all database files and the service account by default.
--purge additionally deletes configuration, data and the service account after a second confirmation.
Reinstall after uninstall restores the recorded version and retained configuration/data.
Paths cannot contain quotes or backslashes. --version cannot be used with --uninstall.
HELP
    else
        cat <<'HELP'
Porter 原生交互安装与卸载器
用法：bash install.sh [--install-dir /opt/porter] [--version v1.0.0] [--lang zh-CN|en]
      bash install.sh --upgrade [--version v1.0.0] [--install-dir /opt/porter] [--lang zh-CN|en]
      bash install.sh --uninstall [--purge] [--install-dir /opt/porter] [--lang zh-CN|en]
默认下载最新正式 GitHub Release；逐项选择 Web 与命令行，默认双开，至少一种。
仅开启 Web 时输入用户名、隐藏密码及完整 Web IP:端口。
支持 Ubuntu 22.04/24.04、Debian 12/13、RHEL/Rocky/AlmaLinux 9/10，amd64/arm64。
要求 systemd 为 PID 1，以及 root 或 sudo 权限；无需 Docker 或编译工具。
重复运行保留版本，可确认后切换组件；--upgrade 显式升级或迁移。
原生产物包含 porterd 服务与可选 porter 客户端，启用后系统命令 /usr/bin/porter。
首次语言选择：1 英文、2 中文，回车默认英文。
CLI 独立偏好 /etc/porter/cli.json 在普通卸载时保留。
目录不支持引号或反斜线。
卸载默认保留配置、完整数据库和服务账号；--purge 额外确认后彻底删除。
卸载后重新安装会复用记录版本和保留的数据；--version 不能与 --uninstall 同用。
HELP
    fi
}
open_terminal() {
    if [[ ${TTY_OPEN:-0} != 1 ]]; then
        { exec 3<>/dev/tty; } 2>/dev/null || return 1
        TTY_OPEN=1
    fi
}
select_language() {
    [[ $LANGUAGE_EXPLICIT == 0 ]] || return 0
    if open_terminal; then
        printf 'Language / 语言 [1 English, 2 中文] (1): ' >&3
        local answer
        IFS= read -r answer <&3 || fail '终端输入已结束。'
        case "$answer" in
            ''|1|en) LANGUAGE=en ;;
            2|zh-CN) LANGUAGE=zh-CN ;;
            *) fail '语言必须是 zh-CN 或 en。' ;;
        esac
    fi
}

# 使用标准库校验地址、产物与安装状态；凭据只经标准输入传递。
PY_HELPER='
import hashlib, ipaddress, json, os, re, sys, tarfile, stat, shutil, subprocess, time, uuid, signal, fcntl, sqlite3, urllib.parse
VERSION = r"v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?"

def address(value):
    match = re.fullmatch(r"(?:\[([^\[\]]+)\]|([^:\[\]]+)):([0-9]+)", value)
    if not match:
        raise ValueError()
    host = match[1] or match[2]
    ip = ipaddress.ip_address(host)
    if "%" in host or (ip.version == 6) != bool(match[1]) or not 1 <= int(match[3]) <= 65535:
        raise ValueError()
    target = ("::1" if ip.version == 6 else "127.0.0.1") if ip.is_unspecified else host
    return "http://" + ("[" + target + "]" if ip.version == 6 else target) + ":" + str(int(match[3]))

def quote(value):
    for char in ("\\", "\"", "`", "$"):
        value = value.replace(char, "\\" + char)
    return "\"" + value + "\""

def read_environment(text):
    values = {}
    for line in text.splitlines():
        if not line or line.startswith("#"):
            continue
        match = re.fullmatch(r"([A-Z_]+)=\"((?:\\[\\\"`$]|[^\"\\])*)\"", line)
        if not match or not match[1].startswith("PORTER_") or match[1] in values:
            raise ValueError()
        values[match[1]] = re.sub(r"\\([\\\"`$])", r"\1", match[2])
    if "PORTER_DB_PATH" not in values:
        raise ValueError()
    return values

def features(state):
    if state["schema"] == 3:
        return state["features"]
    return {"web": True, "cli": state["schema"] == 2}

def environment(values, database, selected, strict=False):
    if values["PORTER_DB_PATH"] != database:
        raise ValueError()
    if strict and any(values.get("PORTER_" + key.upper() + "_ENABLED") != str(enabled).lower() for key, enabled in selected.items()):
        raise ValueError()
    if selected["web"]:
        if not values.get("PORTER_ADMIN_USERNAME", "").strip() or not values.get("PORTER_ADMIN_PASSWORD", "").strip():
            raise ValueError()
        return address(values.get("PORTER_HTTP_ADDR", ""))
    return "-"

def programs(state):
    if state["schema"] == 1:
        return ("porter",)
    return ("porterd", "porter") if features(state)["cli"] else ("porterd",)

def ready(config, database, selected):
    with open(config) as file:
        values = read_environment(file.read())
    health = environment(values, database, selected)
    endpoints = []
    if selected["web"]:
        endpoints.append(([], health + "/healthz", "web"))
    if selected["cli"]:
        socket = values.get("PORTER_CONTROL_SOCKET", os.path.join(os.path.dirname(database), "porter-control.sock"))
        endpoints.append((["--unix-socket", socket], "http://localhost/v1/rules", "cli"))
    for options, url, kind in endpoints:
        response = subprocess.run(["curl", "--fail", "--silent", "--show-error", "--noproxy", "*", "--connect-timeout", "2", "--max-time", "2", *options, url], capture_output=True, text=True, stdin=subprocess.DEVNULL)
        if response.returncode:
            return False
        try:
            value = json.loads(response.stdout)
        except ValueError:
            return False
        if kind == "web" and value != {"status": "ok"}:
            return False
        if kind == "cli" and (not isinstance(value, dict) or set(value) != {"rules"} or not isinstance(value["rules"], list)):
            return False
    if not selected["cli"] and os.path.lexists(values.get("PORTER_CONTROL_SOCKET", os.path.join(os.path.dirname(database), "porter-control.sock"))):
        return False
    return True

def web_rules_check(config, candidate, database, online):
    with open(candidate) as file:
        proposed = read_environment(file.read())
    value = proposed["PORTER_HTTP_ADDR"]
    match = re.fullmatch(r"(?:\[([^\[\]]+)\]|([^:\[\]]+)):([0-9]+)", value)
    if not match:
        raise ValueError()
    host, port = ipaddress.ip_address(match[1] or match[2]), int(match[3])
    if host.version == 6 and host.ipv4_mapped:
        host = host.ipv4_mapped
    if online:
        with open(config) as file:
            current = read_environment(file.read())
        socket = current.get("PORTER_CONTROL_SOCKET", os.path.join(os.path.dirname(database), "porter-control.sock"))
        response = subprocess.run(["curl", "--fail", "--silent", "--show-error", "--noproxy", "*", "--connect-timeout", "2", "--max-time", "2", "--unix-socket", socket, "http://localhost/v1/rules"], capture_output=True, text=True, stdin=subprocess.DEVNULL)
        if response.returncode:
            raise ValueError()
        payload = json.loads(response.stdout)
        if not isinstance(payload, dict) or set(payload) != {"rules"} or not isinstance(payload["rules"], list):
            raise ValueError()
        rows = []
        for rule in payload["rules"]:
            if not isinstance(rule, dict) or type(rule.get("enabled")) is not bool or rule.get("protocol") not in ("tcp", "udp", "tcp+udp") or not isinstance(rule.get("listen_host"), str) or type(rule.get("listen_port")) is not int:
                raise ValueError()
            rows.append((rule["enabled"], rule["protocol"], rule["listen_host"], rule["listen_port"]))
    else:
        # 固定已托管数据库只读查询；不创建、不迁移，也不跳过热日志恢复。
        info = os.lstat(database)
        if not stat.S_ISREG(info.st_mode):
            raise ValueError()
        uri = "file:" + urllib.parse.quote(os.path.abspath(database), safe="/") + "?mode=ro"
        connection = sqlite3.connect(uri, uri=True, timeout=2)
        try:
            connection.execute("PRAGMA query_only=ON")
            rows = connection.execute("SELECT enabled, protocol, listen_host, listen_port FROM rules WHERE enabled=1").fetchall()
        finally:
            connection.close()
    for enabled, protocol, listen_host, listen_port in rows:
        if not enabled or protocol == "udp" or listen_port != port:
            continue
        rule_host = ipaddress.ip_address(listen_host)
        if rule_host.version == 6 and rule_host.ipv4_mapped:
            rule_host = rule_host.ipv4_mapped
        if host.version == rule_host.version and (host == rule_host or host.is_unspecified or rule_host.is_unspecified):
            raise ValueError()

def regular(path):
    info = os.lstat(path)
    if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_nlink != 1:
        raise ValueError()
    return info

def trusted_parent(path):
    current = os.path.dirname(os.path.abspath(path))
    while current != "/":
        info = os.lstat(current)
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or stat.S_IMODE(info.st_mode) & 0o022:
            raise ValueError()
        current = os.path.dirname(current)

def check_cli(path):
    if os.path.lexists(path):
        info = regular(path)
        if stat.S_IMODE(info.st_mode) != 0o600 or info.st_size > 4096:
            raise ValueError()
        with open(path) as file:
            value = json.load(file)
        if set(value) != {"schema", "language"} or type(value["schema"]) is not int or value["schema"] != 1 or value["language"] not in ("en", "zh-CN"):
            raise ValueError()

def check_data_mount(path):
    # 同一文件系统上的 bind mount 也不能整体rename；不能仅依赖ismount/st_dev。
    with open("/proc/self/mountinfo", encoding="utf-8", errors="surrogateescape") as file:
        lines = file.readlines()
    if not lines:
        raise ValueError()
    for line in lines:
        fields = line.split()
        if len(fields) < 10 or " - " not in line or not fields[0].isdigit() or not fields[1].isdigit():
            raise ValueError()
        mountpoint = re.sub(r"\\([0-7]{3})", lambda match: chr(int(match[1], 8)), fields[4])
        if not mountpoint.startswith("/") or os.path.normpath(mountpoint) == os.path.abspath(path):
            raise ValueError()

def validate_state(state, directory, arch):
    if not isinstance(state, dict):
        raise ValueError()
    schema = state.get("schema")
    if type(schema) is not int or schema not in (1, 2, 3) or state["directory"] != directory or state["arch"] != arch or not re.fullmatch(VERSION, state["version"]) or state["status"] not in ("initializing", "complete", "uninstalling", "uninstalled", "upgrading"):
        raise ValueError()
    allowed = {"schema", "directory", "arch", "version", "status", "upgrade_id"}
    if schema in (2, 3):
        allowed |= {"layout", "command_link"}
    if schema == 3:
        allowed.add("features")
        selected = state.get("features")
        if not isinstance(selected, dict) or set(selected) != {"web", "cli"} or any(type(value) is not bool for value in selected.values()) or not any(selected.values()):
            raise ValueError()
        if not selected["cli"] and "command_link" in state:
            raise ValueError()
    if set(state) - allowed:
        raise ValueError()
    if schema in (2, 3) and state.get("layout") != "split":
        raise ValueError()
    if "command_link" in state and (schema not in (2, 3) or state["command_link"] != "/usr/bin/porter" or directory + "/porter" == "/usr/bin/porter"):
        raise ValueError()
    if state["status"] == "upgrading":
        if not re.fullmatch(r"upgrade\.[a-f0-9]{32}", state.get("upgrade_id", "")):
            raise ValueError()
    elif "upgrade_id" in state:
        raise ValueError()
    return state

def compare_versions(left, right):
    def parse(value):
        match = re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?", value)
        if not match:
            raise ValueError()
        pre = match[4].split(".") if match[4] else None
        if pre and any(part.isdigit() and len(part) > 1 and part[0] == "0" for part in pre):
            raise ValueError()
        return tuple(map(int, match.group(1, 2, 3))), pre
    a, ap = parse(left); b, bp = parse(right)
    if a != b:
        return (a > b) - (a < b)
    if ap is None or bp is None:
        return (ap is None) - (bp is None)
    for x, y in zip(ap, bp):
        if x == y:
            continue
        if x.isdigit() and y.isdigit():
            return (int(x) > int(y)) - (int(x) < int(y))
        if x.isdigit() != y.isdigit():
            return -1 if x.isdigit() else 1
        return (x > y) - (x < y)
    return (len(ap) > len(bp)) - (len(ap) < len(bp))

def atomic_json(path, value):
    temporary = path + ".new"
    if os.path.lexists(temporary):
        regular(temporary)
    descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC | os.O_NOFOLLOW, 0o600)
    with os.fdopen(descriptor, "w") as file:
        os.fchmod(file.fileno(), 0o600)
        json.dump(value, file)
        file.flush(); os.fsync(file.fileno())
    os.replace(temporary, path)
    descriptor = os.open(os.path.dirname(path), os.O_DIRECTORY)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)

def copy_owned(source, target):
    info = regular(source)
    shutil.copy2(source, target, follow_symlinks=False)
    os.chown(target, info.st_uid, info.st_gid)

def copy_data(source, target):
    # 运行时 Unix socket 不含持久数据，其余非普通节点拒绝而不是静默遗漏。
    info = os.lstat(source)
    if not stat.S_ISDIR(info.st_mode):
        raise ValueError()
    os.mkdir(target, stat.S_IMODE(info.st_mode))
    os.chown(target, info.st_uid, info.st_gid)
    for entry in os.scandir(source):
        mode = entry.stat(follow_symlinks=False).st_mode
        if stat.S_ISDIR(mode):
            copy_data(entry.path, os.path.join(target, entry.name))
        elif stat.S_ISREG(mode):
            info = os.lstat(entry.path)
            shutil.copy2(entry.path, os.path.join(target, entry.name), follow_symlinks=False)
            os.chown(os.path.join(target, entry.name), info.st_uid, info.st_gid)
        elif not stat.S_ISSOCK(mode):
            raise ValueError()
    shutil.copystat(source, target, follow_symlinks=False)

def upgrade(mode, args):
    directory, arch, statepath, config, data, unit, command, cli, language, *extra = args
    check_data_mount(data)
    def tell(zh, en):
        print(en if language == "en" else zh, flush=True)
    def ctl(*arguments, check=True):
        result = subprocess.run(["systemctl", *arguments], stdin=subprocess.DEVNULL, capture_output=True, text=True)
        if check and result.returncode:
            raise OSError("systemctl " + arguments[0])
        return result
    def inactive():
        result = ctl("is-active", "--quiet", "porter.service", check=False)
        if result.returncode not in (3, 4):
            raise OSError("service is not inactive")
    def install_file(source, target, permissions, preserve=False):
        temporary = os.path.join(directory, "." + os.path.basename(target) + ".new") if target.startswith(directory + "/") else target + ".new"
        if os.path.lexists(temporary):
            regular(temporary)
        subprocess.run(["install", "-m", permissions, source, temporary], check=True, stdin=subprocess.DEVNULL)
        if preserve:
            info = regular(source)
            os.chown(temporary, info.st_uid, info.st_gid)
            os.chmod(temporary, stat.S_IMODE(info.st_mode))
        os.replace(temporary, target)
    with open(statepath) as file:
        state = validate_state(json.load(file), directory, arch)
    folder = None
    transaction = None
    def load_transaction():
        nonlocal folder, transaction
        folder = os.path.join(directory, ".tmp", state["upgrade_id"])
        trusted_parent(os.path.join(folder, "transaction.json"))
        regular(os.path.join(folder, "transaction.json"))
        with open(os.path.join(folder, "transaction.json")) as file:
            transaction = json.load(file)
        legacy_keys = {"id", "phase", "active", "enabled", "cli", "new_link"}
        modern_keys = legacy_keys | {"operation", "old_features", "target_features", "link_present"}
        if not isinstance(transaction, dict) or set(transaction) not in (legacy_keys, modern_keys) or transaction.get("id") != state["upgrade_id"] or transaction.get("phase") not in ("prepared", "stopped", "backup_complete", "switching", "restoring", "restored") or any(type(transaction.get(key)) is not bool for key in ("active", "enabled", "cli", "new_link")):
            raise ValueError()
        if set(transaction) == modern_keys:
            if transaction["operation"] not in ("upgrade", "reconfigure") or type(transaction["link_present"]) is not bool:
                raise ValueError()
            for key in ("old_features", "target_features"):
                selected = transaction[key]
                if not isinstance(selected, dict) or set(selected) != {"web", "cli"} or any(type(value) is not bool for value in selected.values()) or not any(selected.values()):
                    raise ValueError()
        with open(os.path.join(folder, "install.json")) as file:
            old = validate_state(json.load(file), directory, arch)
        if old["status"] != "complete" or old["version"] != state["version"] or old["schema"] != state["schema"]:
            raise ValueError()
        if "operation" in transaction:
            if transaction["old_features"] != features(old):
                raise ValueError()
        else:
            # 兼容旧安装器写出的六字段升级日志，不改写其原安装快照。
            transaction.update(operation="upgrade", old_features=features(old), target_features={"web": True, "cli": True}, link_present=bool(old.get("command_link")) and not transaction["new_link"])
        return old
    def mark(phase):
        transaction["phase"] = phase
        atomic_json(os.path.join(folder, "transaction.json"), transaction)
    def service_intent():
        ctl("enable" if transaction["enabled"] else "disable", "porter.service")
        if transaction["active"]:
            ctl("start", "porter.service")
        else:
            ctl("stop", "porter.service"); inactive()
    def rollback():
        old = load_transaction()
        phase = transaction["phase"]
        ctl("stop", "porter.service"); inactive()
        if phase in ("switching", "restoring"):
            mark("restoring")
            for name in programs(old):
                install_file(os.path.join(folder, name), os.path.join(directory, name), "0755", preserve=True)
            for name in {"porter", "porterd"} - set(programs(old)):
                path = os.path.join(directory, name)
                if os.path.lexists(path) and (name == "porterd" or transaction["target_features"]["cli"]):
                    regular(path); os.unlink(path)
            install_file(os.path.join(folder, "porter.service"), unit, "0644", preserve=True)
            install_file(os.path.join(folder, "porter.env"), config, "0600", preserve=True)
            if transaction["cli"]:
                install_file(os.path.join(folder, "cli.json"), cli, "0600", preserve=True)
            elif os.path.lexists(cli):
                check_cli(cli); os.unlink(cli)
            if command != directory + "/porter" and (transaction["new_link"] or old.get("command_link")):
                if os.path.lexists(command):
                    if not os.path.islink(command) or os.readlink(command) != directory + "/porter" or os.lstat(command).st_uid != 0:
                        raise ValueError()
                    if not transaction["link_present"]:
                        os.unlink(command)
                elif transaction["link_present"]:
                    os.symlink(directory + "/porter", command)
            # 先构造完整恢复目录，旧备份始终保留；目录整体替换，不合并数据库。
            restored = os.path.join(os.path.dirname(data), ".porter-restore-" + state["upgrade_id"])
            if os.path.exists(restored):
                shutil.rmtree(restored)
            copy_data(os.path.join(folder, "data"), restored)
            displaced = os.path.join(os.path.dirname(data), ".porter-failed-" + state["upgrade_id"])
            if os.path.exists(data):
                if os.path.exists(displaced):
                    shutil.rmtree(displaced)
                os.rename(data, displaced)
            os.rename(restored, data)
            mark("restored")
        ctl("daemon-reload")
        service_intent()
        install_file(os.path.join(folder, "install.json"), statepath, "0600", preserve=True)
        tell("已恢复原版本；备份保留于：" + folder, "Original version restored; backup retained at: " + folder)
    if mode == "recover":
        try:
            rollback()
        except Exception:
            tell("升级恢复失败，保留安装记录及备份：" + str(folder), "Upgrade recovery failed; installation record and backup retained: " + str(folder))
            raise
        return
    target, staging, web, client = extra
    selected = {"web": web == "true", "cli": client == "true"}
    if not any(selected.values()):
        raise ValueError()
    if state["status"] != "complete" or compare_versions(target, state["version"]) < 0:
        raise ValueError()
    active_result = ctl("is-active", "--quiet", "porter.service", check=False)
    if active_result.returncode not in (0, 3, 4):
        raise ValueError()
    enabled_result = ctl("is-enabled", "--quiet", "porter.service", check=False)
    if enabled_result.returncode not in (0, 1, 3, 4):
        raise ValueError()
    identifier = "upgrade." + uuid.uuid4().hex
    folder = os.path.join(directory, ".tmp", identifier)
    os.mkdir(folder, 0o700)
    copy_owned(statepath, os.path.join(folder, "install.json"))
    transaction = {"id": identifier, "phase": "prepared", "active": active_result.returncode == 0, "enabled": enabled_result.returncode == 0, "cli": os.path.exists(cli), "new_link": selected["cli"] and command != directory + "/porter" and not os.path.lexists(command), "operation": mode, "old_features": features(state), "target_features": selected, "link_present": command != directory + "/porter" and bool(state.get("command_link")) and os.path.lexists(command)}
    atomic_json(os.path.join(folder, "transaction.json"), transaction)
    state["status"] = "upgrading"; state["upgrade_id"] = identifier
    atomic_json(statepath, state)
    def interrupted(signum, frame):
        raise InterruptedError()
    signal.signal(signal.SIGTERM, interrupted); signal.signal(signal.SIGINT, interrupted)
    try:
        ctl("stop", "porter.service"); inactive(); mark("stopped")
        if not features(state)["web"] and selected["web"]:
            web_rules_check(config, os.path.join(staging, "porter.env"), os.path.join(data, "porter.db"), False)
        for name in programs(state):
            copy_owned(os.path.join(directory, name), os.path.join(folder, name))
        copy_owned(unit, os.path.join(folder, "porter.service"))
        copy_owned(config, os.path.join(folder, "porter.env"))
        if transaction["cli"]:
            copy_owned(cli, os.path.join(folder, "cli.json"))
        copy_data(data, os.path.join(folder, "data"))
        mark("backup_complete")
        # 此标记先于任何替换；恢复时只信完整备份。
        mark("switching")
        for name in (("porterd", "porter") if selected["cli"] else ("porterd",)):
            install_file(os.path.join(staging, name), os.path.join(directory, name), "0755")
        install_file(os.path.join(staging, "porter.service"), unit, "0644")
        install_file(os.path.join(staging, "porter.env"), config, "0600")
        if (state["schema"] == 1 or features(state)["cli"]) and not selected["cli"]:
            if state.get("command_link") and os.path.lexists(command):
                if not os.path.islink(command) or os.readlink(command) != directory + "/porter" or os.lstat(command).st_uid != 0:
                    raise ValueError()
                os.unlink(command)
            regular(os.path.join(directory, "porter")); os.unlink(os.path.join(directory, "porter"))
        if transaction["new_link"]:
            os.symlink(directory + "/porter", command)
        ctl("daemon-reload"); ctl("start", "porter.service")
        available = False
        for attempt in range(30):
            if ctl("is-active", "--quiet", "porter.service", check=False).returncode == 0 and ready(config, os.path.join(data, "porter.db"), selected):
                available = True; break
            subprocess.run(["sleep", "2"], check=True)
        if not available or ctl("is-active", "--quiet", "porter.service", check=False).returncode:
            raise OSError("health check failed")
        service_intent()
        new = {"schema": 3, "layout": "split", "directory": directory, "arch": arch, "version": target, "status": "complete", "features": selected}
        if selected["cli"] and command != directory + "/porter":
            new["command_link"] = command
        atomic_json(statepath, new)
        # complete 的原子提交是成功边界；之后只输出，不再执行可能失败的变更。
    except BaseException:
        signal.signal(signal.SIGTERM, signal.SIG_IGN); signal.signal(signal.SIGINT, signal.SIG_IGN)
        try:
            rollback()
        except Exception:
            tell("升级恢复失败，请重跑安装器。备份：" + folder, "Upgrade recovery failed. Run the installer again. Backup: " + folder)
        raise
    if mode == "reconfigure":
        tell("组件切换完成；回退备份保留于：" + folder, "Components changed; rollback backup retained at: " + folder)
    else:
        tell("升级完成；回退备份保留于：" + folder, "Upgrade completed; rollback backup retained at: " + folder)

try:
    mode, *args = sys.argv[1:]
    if mode == "trim":
        print(sys.stdin.read().strip())
    elif mode == "address":
        print(address(args[0]))
    elif mode == "env-write":
        username, password, listen, _ = sys.stdin.read().split("\0")
        values = {}
        if len(args) > 3 and args[3] != "-":
            with open(args[3]) as file:
                values = read_environment(file.read())
        values["PORTER_DB_PATH"] = args[0]
        if username:
            values.update(PORTER_ADMIN_USERNAME=username, PORTER_ADMIN_PASSWORD=password, PORTER_HTTP_ADDR=listen)
        values.update(PORTER_WEB_ENABLED=args[1], PORTER_CLI_ENABLED=args[2])
        selected = {"web": args[1] == "true", "cli": args[2] == "true"}
        environment(values, args[0], selected, True)
        for key, value in values.items():
            print(key + "=" + quote(value))
    elif mode == "env-read":
        values = read_environment(sys.stdin.read())
        selected = {"web": args[1] == "true", "cli": args[2] == "true"}
        print(environment(values, args[0], selected, len(args) > 3 and args[3] == "3"))
    elif mode == "web-valid":
        values = read_environment(sys.stdin.read())
        environment(values, args[0], {"web": True, "cli": False})
    elif mode == "preflight":
        executable, config, database, web, cli = args
        with open(config) as file:
            values = read_environment(file.read())
        environment(values, database, {"web": web == "true", "cli": cli == "true"}, True)
        process_env = {key: value for key, value in os.environ.items() if not key.startswith("PORTER_")}
        process_env.update(values)
        help_result = subprocess.run([executable, "--help"], capture_output=True, text=True, env=process_env, stdin=subprocess.DEVNULL)
        if help_result.returncode or "--check-config" not in help_result.stdout + help_result.stderr:
            raise ValueError()
        result = subprocess.run([executable, "--check-config"], capture_output=True, env=process_env, stdin=subprocess.DEVNULL)
        if result.returncode:
            raise ValueError()
    elif mode == "web-rules-check":
        web_rules_check(args[0], args[1], args[2], args[3] == "online")
    elif mode == "ready":
        if not ready(args[0], args[1], {"web": args[2] == "true", "cli": args[3] == "true"}):
            raise ValueError()
    elif mode == "release":
        release = json.load(sys.stdin)
        version = release["tag_name"]
        if not re.fullmatch(VERSION, version) or release.get("draft") or (not args[1] and release.get("prerelease")):
            raise ValueError()
        if args[1] and version != args[1]:
            raise ValueError()
        archive = "porter-" + version + "-linux-" + args[0] + ".tar.gz"
        names = [asset["name"] for asset in release["assets"]]
        if names.count(archive) != 1 or names.count("SHA256SUMS") != 1:
            raise ValueError()
        print(version)
    elif mode == "unpack":
        archive, checksums, output, layout = args
        filename = os.path.basename(archive)
        matches = []
        with open(checksums, encoding="utf-8") as file:
            for line in file:
                match = re.fullmatch(r"([a-fA-F0-9]{64}) [ *](.+)", line.rstrip("\n"))
                if match and match[2] == filename:
                    matches.append(match[1].lower())
        with open(archive, "rb") as file:
            digest = hashlib.sha256()
            for block in iter(lambda: file.read(1048576), b""):
                digest.update(block)
        if len(matches) != 1 or digest.hexdigest() != matches[0]:
            raise ValueError()
        with tarfile.open(archive, "r:gz") as bundle:
            for name in (("porter", "porterd") if layout == "split" else ("porter",)):
                members = [member for member in bundle.getmembers() if member.name == name]
                if len(members) != 1 or not members[0].isreg() or not 0 < members[0].size <= 268435456:
                    raise ValueError()
                with bundle.extractfile(members[0]) as source, open(os.path.join(output, name), "xb") as destination:
                    shutil.copyfileobj(source, destination)
    elif mode == "state-write":
        schema = int(args[6])
        state = {"schema": schema, "directory": args[0], "arch": args[1], "version": args[2], "status": args[3]}
        if schema in (2, 3):
            state["layout"] = "split"
            if args[5] == "yes":
                state["command_link"] = "/usr/bin/porter"
        if schema == 3:
            state["features"] = {"web": args[7] == "true", "cli": args[8] == "true"}
        validate_state(state, args[0], args[1])
        json.dump(state, sys.stdout)
    elif mode == "state-read":
        state = validate_state(json.load(sys.stdin), args[0], args[1])
        selected = features(state)
        print(state["version"], state["status"], "legacy" if state["schema"] == 1 else "split", "yes" if state.get("command_link") else "no", str(selected["web"]).lower(), str(selected["cli"]).lower(), state["schema"])
    elif mode == "state-directory":
        state = json.load(sys.stdin)
        if type(state.get("schema")) is not int or state.get("schema") not in (1, 2, 3) or not isinstance(state.get("directory"), str) or "\n" in state["directory"]:
            raise ValueError()
        print(state["directory"])
    elif mode == "link-check":
        command, target, managed = args
        trusted_parent(command)
        if os.path.lexists(command):
            info = os.lstat(command)
            if managed != "yes" or not stat.S_ISLNK(info.st_mode) or info.st_uid != 0 or os.readlink(command) != target:
                raise ValueError()
    elif mode == "snapshot":
        result = []
        for path in args:
            if os.path.lexists(path):
                info = regular(path)
                with open(path, "rb") as file:
                    result.append((path, info.st_ino, info.st_mode, info.st_uid, hashlib.sha256(file.read()).hexdigest()))
            else:
                result.append((path, None))
        print(json.dumps(result))
    elif mode in ("cli-check", "cli-purge"):
        check_cli(args[0])
        temporary = []
        folder = os.path.dirname(args[0])
        if os.path.isdir(folder):
            for entry in os.scandir(folder):
                if re.fullmatch(r".cli-[a-f0-9]{24}\.new", entry.name):
                    info = regular(entry.path)
                    if stat.S_IMODE(info.st_mode) != 0o600:
                        raise ValueError()
                    temporary.append(entry.path)
        if mode == "cli-purge":
            for path in temporary + ([args[0]] if os.path.lexists(args[0]) else []):
                os.unlink(path)
    elif mode == "data-check":
        check_data_mount(args[0])
    elif mode == "semver":
        if compare_versions(args[0], args[1]) < 0:
            raise ValueError()
    elif mode == "unlock":
        path, pid = args
        owner = os.path.join(path, "owner.json")
        regular(owner)
        with open(owner) as file:
            data = json.load(file)
        with open("/proc/" + pid + "/stat") as file:
            start = file.read().rsplit(")", 1)[1].split()[19]
        if data != {"pid": int(pid), "start": start}:
            raise ValueError()
        os.unlink(owner)
        os.rmdir(path)
    elif mode == "lock":
        path, pid = args
        try:
            os.mkdir(path, 0o700)
        except FileExistsError:
            info = os.lstat(path)
            if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or stat.S_IMODE(info.st_mode) & 0o077:
                raise ValueError()
            owner = os.path.join(path, "owner.json")
            regular(owner)
            descriptor = os.open(owner, os.O_RDONLY | os.O_NOFOLLOW)
            try:
                fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
                if os.stat(path).st_ino != info.st_ino or os.fstat(descriptor).st_ino != os.lstat(owner).st_ino:
                    raise ValueError()
                with os.fdopen(os.dup(descriptor)) as file:
                    data = json.load(file)
                if set(data) != {"pid", "start"} or set(os.listdir(path)) != {"owner.json"} or type(data.get("pid")) is not int or data["pid"] <= 0 or not isinstance(data.get("start"), str) or not data["start"].isdigit():
                    raise ValueError()
                proc = "/proc/" + str(data["pid"]) + "/stat"
                if os.path.exists(proc):
                    with open(proc) as file:
                        if file.read().rsplit(")", 1)[1].split()[19] == data.get("start"):
                            raise ValueError()
                os.unlink(owner)
                os.rmdir(path)
                os.mkdir(path, 0o700)
            finally:
                os.close(descriptor)
        with open("/proc/" + pid + "/stat") as file:
            start = file.read().rsplit(")", 1)[1].split()[19]
        with open(os.path.join(path, "owner.json"), "x") as file:
            json.dump({"pid": int(pid), "start": start}, file)
    elif mode in ("upgrade", "reconfigure", "recover"):
        upgrade(mode, args)
    elif mode == "group-users":
        for line in sys.stdin:
            fields = line.rstrip("\n").split(":")
            if len(fields) != 7 or (fields[3] == args[0] and fields[0] != "porter"):
                raise ValueError()
    elif mode == "paths":
        for path in args:
            current = os.path.abspath(path)
            while current != "/":
                if os.path.islink(current):
                    raise ValueError()
                current = os.path.dirname(current)
    elif mode == "unit":
        # 冒号前缀禁用命令参数的环境展开；百分号仍需转义说明符。
        executable = args[0].replace("\\", "\\\\").replace("\"", "\\\"").replace("%", "%%")
        envfile = args[1].replace("\\", "\\\\").replace("\"", "\\\"").replace("%", "%%")
        data = args[2].replace("%", "%%")
        print("[Unit]\nDescription=Porter\nAfter=network.target\n\n[Service]\nType=simple\nUser=porter\nGroup=porter\nEnvironmentFile=" + envfile + "\nExecStart=:\"" + executable + "\"\nWorkingDirectory=" + data + "\nStateDirectory=porter\nStateDirectoryMode=0700\nRestart=on-failure\nRestartSec=3s\nUMask=0077\nAmbientCapabilities=CAP_NET_BIND_SERVICE\nCapabilityBoundingSet=CAP_NET_BIND_SERVICE\nNoNewPrivileges=true\nProtectSystem=strict\n\n[Install]\nWantedBy=multi-user.target")
except (ValueError, KeyError, TypeError, OSError, tarfile.TarError, subprocess.SubprocessError, sqlite3.Error):
    sys.exit(1)
'
py() { python3 -c "$PY_HELPER" "$@"; }
root() { "${PRIV[@]}" "$@" </dev/null; }
restore_terminal() {
    if [[ ${TTY_HIDDEN:-0} == 1 ]]; then stty "$TTY_STATE" <&3 || true; TTY_HIDDEN=0; fi
}
cleanup() {
    restore_terminal
    if [[ -n ${STAGING:-} ]]; then root rm -rf -- "$STAGING"; fi
    if [[ -n ${GLOBAL_LOCK:-} ]]; then root python3 -c "$PY_HELPER" unlock "$GLOBAL_LOCK" "$$" || true; fi
    if [[ -n ${LOCK:-} ]]; then root python3 -c "$PY_HELPER" unlock "$LOCK" "$$" || true; fi
    if [[ ${REMOVE_INSTALL_DIR:-0} == 1 ]]; then
        root rmdir -- "$INSTALL_DIR/.tmp" 2>/dev/null || true
        root rmdir -- "$INSTALL_DIR" 2>/dev/null || true
    fi
    if [[ ${REMOVE_CONFIG_DIR:-0} == 1 ]]; then root rmdir -- /etc/porter 2>/dev/null || true; fi
}
check_system() {
    [[ $(uname -s) == Linux ]] || fail '仅支持 Linux。'
    [[ -r /etc/os-release ]] || fail '无法读取系统发行信息。'
    . /etc/os-release
    case "$ID:$VERSION_ID" in
        ubuntu:22.04|ubuntu:24.04|debian:12|debian:13) FAMILY=apt ;;
        rhel:9*|rhel:10*|rocky:9*|rocky:10*|almalinux:9*|almalinux:10*)
            [[ ${VERSION_ID%%.*} == 9 || ${VERSION_ID%%.*} == 10 ]] || fail '不支持此系统版本。'
            FAMILY=dnf ;;
        *) fail '不支持的发行版或版本：%s %s。' "$ID" "$VERSION_ID" ;;
    esac
    case "$(uname -m)" in
        x86_64) ARCH=amd64 ;;
        aarch64|arm64) ARCH=arm64 ;;
        *) fail '仅支持 amd64 和 arm64。' ;;
    esac
    [[ $(cat /proc/1/comm) == systemd && -d /run/systemd/system ]] || fail '需要以 systemd 为 PID 1 的真实 Linux 宿主。'
    command -v systemctl >/dev/null 2>&1 || fail '缺少 systemctl。'
}
collect_input() {
    open_terminal || fail '首次配置需要可交互终端，请在 SSH 终端运行。'
    unset USERNAME PASSWORD CONFIRM LISTEN
    while true; do
        message '管理员用户名：' >&3
        IFS= read -r USERNAME <&3 || fail '终端输入已结束。'
        USERNAME=$(printf '%s' "$USERNAME" | py trim)
        [[ -n $USERNAME ]] && break
        message '用户名不能为空，请重新输入。\n' >&3
    done
    TTY_STATE=$(stty -g <&3); TTY_HIDDEN=1; stty -echo <&3
    while true; do
        message '管理员密码：' >&3
        IFS= read -rs PASSWORD <&3 || fail '终端输入已结束。'
        message '\n确认密码：' >&3
        IFS= read -rs CONFIRM <&3 || fail '终端输入已结束。'
        printf '\n' >&3
        if [[ -z $(printf '%s' "$PASSWORD" | py trim) ]]; then
            message '密码不能全部为空白，请重新输入。\n' >&3
        elif [[ $PASSWORD != "$CONFIRM" ]]; then
            message '两次密码不一致，请重新输入。\n' >&3
        else break; fi
    done
    unset CONFIRM; restore_terminal
    while true; do
        message 'Web 监听地址（如 0.0.0.0:8080 或 [::]:8080）：' >&3
        IFS= read -r LISTEN <&3 || fail '终端输入已结束。'
        if HEALTH_URL=$(py address "$LISTEN"); then break; fi
        message '请输入有效的 IPv4:端口 或 [IPv6]:端口，端口范围 1–65535。\n' >&3
    done
}


ask_boolean() {
    local key=$1 default=$2 answer
    while true; do
        message "$key" "$([[ $default == true ]] && printf yes || printf no)" >&3
        IFS= read -r answer <&3 || fail '终端输入已结束。'
        case "$answer" in
            '') SELECTED=$default; return ;;
            yes|y|Y|YES|是|1) SELECTED=true; return ;;
            no|n|N|NO|否|0) SELECTED=false; return ;;
            *) message '请输入 yes 或 no（也可输入 y 或 n）。\n' >&3 ;;
        esac
    done
}
select_features() {
    open_terminal || fail '首次配置需要可交互终端，请在 SSH 终端运行。'
    local default_web=$WEB_ENABLED default_cli=$CLI_ENABLED
    while true; do
        ask_boolean '是否启用 Web 服务？[%s]：' "$default_web"; WEB_ENABLED=$SELECTED
        ask_boolean '是否安装命令行工具？[%s]：' "$default_cli"; CLI_ENABLED=$SELECTED
        [[ $WEB_ENABLED == true || $CLI_ENABLED == true ]] && return 0
        message '至少启用一种管理方式，请重新选择。\n' >&3
    done
}
program_names() {
    if [[ $LAYOUT == legacy ]]; then printf 'porter';
    elif [[ $CLI_ENABLED == true ]]; then printf 'porterd porter';
    else printf 'porterd'; fi
}
write_candidate() {
    local existing=${1:--}
    printf '%s\0%s\0%s\0' "${USERNAME:-}" "${PASSWORD:-}" "${LISTEN:-}" | "${PRIV[@]}" python3 -c "$PY_HELPER" env-write "$DATA/porter.db" "$WEB_ENABLED" "$CLI_ENABLED" "$existing" | "${PRIV[@]}" tee "$STAGING/porter.env" >/dev/null
    unset USERNAME PASSWORD LISTEN
}
check_web_conflicts() {
    local probe=offline code
    if root systemctl is-active --quiet porter.service; then probe=online;
    else code=$?; [[ $code == 3 || $code == 4 ]] || fail '无法查询 systemd 服务。'; fi
    root python3 -c "$PY_HELPER" web-rules-check "$CONFIG" "$STAGING/porter.env" "$DATA/porter.db" "$probe" || fail 'Web 监听与已有转发规则冲突，或无法可靠读取规则；未停止服务。'
}
preflight() {
    root python3 -c "$PY_HELPER" preflight "$1" "$STAGING/porter.env" "$DATA/porter.db" "$WEB_ENABLED" "$CLI_ENABLED" || fail '候选程序不支持组件配置，或候选配置预检失败；未停止服务。'
}
probe_ready() {
    root python3 -c "$PY_HELPER" ready "$CONFIG" "$DATA/porter.db" "$WEB_ENABLED" "$CLI_ENABLED"
}
show_ready() {
    show_layout
    if [[ $WEB_ENABLED == true ]]; then
        message '\nPorter 已就绪\n用户入口：%s/user/\n管理员入口：%s/admin/\n通配监听时，远程访问请改用服务器 IP。\n' "$HEALTH_URL" "$HEALTH_URL"
    else
        message '\nPorter 已就绪（仅命令行，无 Web 监听）。\n'
    fi
}

fetch() {
    root curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
        --connect-timeout 10 --max-time 180 "$1" -o "$2"
}
write_state() {
    py state-write "$INSTALL_DIR" "$ARCH" "$VERSION" "$1" "$LAYOUT" "$LINK_MANAGED" "$STATE_SCHEMA" "$WEB_ENABLED" "$CLI_ENABLED" | "${PRIV[@]}" tee "$STAGING/install.json" >/dev/null
    root install -m 0600 "$STAGING/install.json" "$STATE.new"
    root mv -- "$STATE.new" "$STATE"
}
check_account() {
    local account group uid gid home shell ignored
    if account=$(getent passwd porter); then
        IFS=: read -r ignored ignored uid gid ignored home shell <<<"$account"
        [[ $uid =~ ^[0-9]+$ && $uid != 0 && $home == "$DATA" && ( $shell == /usr/sbin/nologin || $shell == /sbin/nologin ) ]] || fail '现有 porter 账号不符合专用服务账号要求。'
        group=$(getent group porter) || fail '专用 porter 组缺失。'
        [[ $(printf '%s' "$group" | cut -d: -f3) == "$gid" ]] || fail 'porter 用户与组不匹配。'
    else
        if ! getent group porter >/dev/null; then root groupadd --system porter; fi
        local nologin
        if root test -x /usr/sbin/nologin; then nologin=/usr/sbin/nologin;
        elif root test -x /sbin/nologin; then nologin=/sbin/nologin;
        else fail '缺少 nologin，无法创建禁止登录的服务账号。'; fi
        root useradd --system --gid porter --home-dir "$DATA" --shell "$nologin" --no-create-home porter
    fi
}
validate_directory() {
    [[ $INSTALL_DIR == /* && ! $INSTALL_DIR =~ [[:cntrl:]] ]] || fail '安装目录必须是无控制字符的绝对路径。'
    case "$INSTALL_DIR" in *\\*|*\"*|*\'*) fail '安装目录不能包含引号或反斜线（systemd 路径限制）。' ;; esac
}
check_uninstall_account() {
    local account group uid gid home shell ignored members
    if account=$(getent passwd porter); then
        IFS=: read -r ignored ignored uid gid ignored home shell <<<"$account"
        [[ $uid =~ ^[0-9]+$ && $uid != 0 && $home == "$DATA" && ( $shell == /usr/sbin/nologin || $shell == /sbin/nologin ) ]] || fail '现有 porter 账号不符合专用服务账号要求。'
        group=$(getent group porter) || fail '专用 porter 组缺失。'
        [[ $(printf '%s' "$group" | cut -d: -f3) == "$gid" ]] || fail 'porter 用户与组不匹配。'
    elif [[ $status == complete ]]; then
        fail '专用 porter 账号缺失，请先检查安装状态。'
    fi
    if group=$(getent group porter); then
        IFS=: read -r ignored ignored gid members <<<"$group"
        [[ $gid =~ ^[0-9]+$ && $gid != 0 && -z $members ]] || fail 'porter 组被其他账号使用，拒绝删除。'
        if (( PURGE )); then
            getent passwd | py group-users "$gid" || fail 'porter 组被其他账号使用，拒绝删除。'
        fi
    fi
}
check_inactive() {
    local code
    if root systemctl is-active --quiet porter.service; then
        fail '服务仍在运行，未删除任何剩余文件。'
    else
        code=$?
        [[ $code == 3 || $code == 4 ]] || fail '无法确认服务已停止，请检查 systemd。'
    fi
}
uninstall() {
    local status saved_version fragment dropins expected actual answer path
    root python3 -c "$PY_HELPER" paths "$INSTALL_DIR" /etc/porter "$CONFIG" "$STATE" "$UNIT" "$DATA" "$DATA/porter.db" "$BINARY" "$STATE.new" "$CONFIG.new" "$UNIT.new" "$INSTALL_DIR/.porter.new" "$INSTALL_DIR/.tmp" "$SERVER" "$INSTALL_DIR/.porterd.new" "$CLI_CONFIG" || fail '拒绝使用符号链接路径。'
    if ! root test -e "$STATE"; then
        fragment=$(root systemctl show --property=FragmentPath --value porter.service) || fail '无法查询 systemd 服务。'
        [[ -z $fragment ]] || fail '发现已有 porter.service，拒绝覆盖未知服务。'
        for path in "$CONFIG" "$UNIT" "$DATA" "$BINARY" "$CONFIG.new" "$STATE.new" "$UNIT.new" "$INSTALL_DIR/.porter.new" "$INSTALL_DIR/.porterd.new"; do
            root test ! -e "$path" || fail '发现未托管的已有文件：%s。' "$path"
        done
        if [[ $INSTALL_DIR/porter != "$COMMAND" ]]; then
            root test ! -e "$INSTALL_DIR/porter" && root test ! -L "$INSTALL_DIR/porter" || fail '发现未托管的已有文件：%s。' "$INSTALL_DIR/porter"
        fi
        if getent passwd porter >/dev/null || getent group porter >/dev/null; then fail '发现未知的 porter 账号或组，拒绝接管。'; fi
        check_inactive
        message 'Porter 已卸载，无需重复操作。\n'
        return
    fi
    # 与安装共用锁；失败清理仅释放本次成功获取的锁。
    root python3 -c "$PY_HELPER" lock /etc/porter/.install-lock "$$" || fail '另一个安装进程正在写入全局服务配置。'
    GLOBAL_LOCK=/etc/porter/.install-lock
    if root test -d "$INSTALL_DIR"; then
        root python3 -c "$PY_HELPER" lock "$INSTALL_DIR/.install-lock" "$$" || fail '安装目录被锁定，请确认没有其他安装进程。'
        LOCK=$INSTALL_DIR/.install-lock
    elif root test -e "$INSTALL_DIR"; then fail '程序目录不是目录。'; fi
    local record
    record=$(root cat "$STATE" | py state-read "$INSTALL_DIR" "$ARCH") || fail '安装状态无效或目录/架构不匹配，未覆盖已有文件。'
    read -r saved_version status LAYOUT LINK_MANAGED WEB_ENABLED CLI_ENABLED STATE_SCHEMA <<<"$record"
    if [[ $LAYOUT == split ]]; then BINARY=$SERVER; else BINARY=$INSTALL_DIR/porter; fi
    VERSION=$saved_version
    root python3 -c "$PY_HELPER" paths "$BINARY" || fail '拒绝使用符号链接路径。'
    root python3 -c "$PY_HELPER" cli-check "$CLI_CONFIG" || fail 'CLI 配置类型、属主或权限无效。'
    if [[ $LAYOUT == split && $CLI_ENABLED == true ]]; then check_command; fi
    for path in "$CONFIG" "$DATA/porter.db" "$CONFIG.new" "$STATE.new" "$UNIT.new" "$INSTALL_DIR/.porter.new" "$INSTALL_DIR/.porterd.new"; do
        if root test -e "$path"; then
            root test -f "$path" || fail '托管配置或数据的文件类型不符，拒绝卸载。'
        fi
    done
    if root test -e "$DATA"; then
        root test -d "$DATA" || fail '托管配置或数据的文件类型不符，拒绝卸载。'
    fi
    local dropins
    dropins=$(root systemctl show --property=DropInPaths --value porter.service) || fail '无法查询 systemd 服务。'
    [[ -z $dropins ]] || fail '发现自定义 systemd 覆盖配置，拒绝卸载。'
    fragment=$(root systemctl show --property=FragmentPath --value porter.service) || fail '无法查询 systemd 服务。'
    [[ -z $fragment || $fragment == "$UNIT" ]] || fail '发现其他来源的 porter.service，拒绝覆盖。'
    dropins=$(root systemctl show --property=DropInPaths --value porter.service) || fail '无法查询 systemd 服务。'
    [[ -z $dropins ]] || fail '发现自定义 systemd 覆盖配置，拒绝卸载。'
    if root test -e "$UNIT"; then
        root test -f "$UNIT" || fail '现有 unit 与本安装器不符，拒绝覆盖。'
        expected=$(py unit "$BINARY" "$CONFIG" "$DATA")
        actual=$(root cat "$UNIT")
        [[ $actual == "$expected" ]] || fail '现有 unit 与本安装器不符，拒绝覆盖。'
    elif [[ $status == complete || ( -n $fragment && $status != uninstalling ) ]]; then
        fail '托管服务文件缺失，请先检查安装状态。'
    fi
    if root test -e "$BINARY"; then
        root test -f "$BINARY" && root test -x "$BINARY" || fail '已有程序不可执行，拒绝覆盖。'
        [[ $(root "$BINARY" --version) == "$VERSION" ]] || fail '程序版本与发布标签不符。'
    elif [[ $status == complete ]]; then fail '托管程序缺失，请先检查安装状态。'; fi
    if [[ $LAYOUT == split && $CLI_ENABLED == true ]] && root test -e "$INSTALL_DIR/porter"; then
        root test -f "$INSTALL_DIR/porter" && root test -x "$INSTALL_DIR/porter" && [[ $(root "$INSTALL_DIR/porter" --version) == "$VERSION" ]] || fail '程序版本与发布标签不符。'
    elif [[ $LAYOUT == split && $CLI_ENABLED == true && $status == complete ]]; then fail '托管程序缺失，请先检查安装状态。'; fi
    check_uninstall_account
    if [[ $STATE_SCHEMA == 3 && $status != uninstalling ]] && root test -f "$CONFIG"; then
        root cat "$CONFIG" | py env-read "$DATA/porter.db" "$WEB_ENABLED" "$CLI_ENABLED" "$STATE_SCHEMA" >/dev/null || fail '现有环境配置无效；未重置任何凭据。'
    fi
    if [[ $status == uninstalled && $PURGE == 0 ]]; then
        root test ! -e "$BINARY" && root test ! -e "$UNIT" || fail '已卸载状态存在意外程序或服务，请先检查。'
        check_inactive
        message 'Porter 已卸载，无需重复操作。\n'
        return
    fi
    open_terminal || fail '卸载需要可交互终端，请在 SSH 终端运行。'
    message '将停止并禁用 porter.service，删除程序 %s 和服务文件 %s。\n' "$BINARY" "$UNIT" >&3
    message '同时清理未提交的暂存文件：%s、%s、%s、%s。\n' "$CONFIG.new" "$STATE.new" "$UNIT.new" "$INSTALL_DIR/.porter.new" >&3
    if (( PURGE )); then
        message '历史升级备份可能包含凭据，仍保留在 %s；失败数据副本可能包含业务数据，仍保留在 %s/.porter-failed-upgrade.<标识> 中。请核对清理。\n' "$INSTALL_DIR/.tmp" "${DATA%/*}" >&3
        message '彻底删除还将清除配置 %s、完整数据目录 %s、安装记录和专用账号/组。\n' "$CONFIG" "$DATA" >&3
    else
        message '保留配置 %s、完整数据目录 %s、安装记录和专用账号/组。\n' "$CONFIG" "$DATA" >&3
    fi
    message '确认卸载？输入 yes 继续，回车取消：' >&3
    IFS= read -r answer <&3 || fail '终端输入已结束。'
    if [[ $answer != yes ]]; then message '已取消，未修改安装。\n'; return; fi
    if (( PURGE )); then
        message '账号、规则及统计将永久丢失。输入 DELETE 确认彻底删除：' >&3
        IFS= read -r answer <&3 || fail '终端输入已结束。'
        if [[ $answer != DELETE ]]; then message '已取消，未修改安装。\n'; return; fi
    fi
    root install -d -m 0755 "$INSTALL_DIR"
    root install -d -m 0700 "$INSTALL_DIR/.tmp"
    STAGING=$(root mktemp -d "$INSTALL_DIR/.tmp/uninstall.XXXXXXXX")
    write_state uninstalling
    if root test -f "$UNIT"; then
        root systemctl stop porter.service || fail '停止服务失败，请重试卸载；尚未删除文件。'
        root systemctl disable porter.service || fail '禁用开机启动失败，请重试卸载；尚未删除文件。'
    fi
    check_inactive
    if [[ $LAYOUT == split && $CLI_ENABLED == true ]]; then
        check_command
        if [[ $LINK_MANAGED == yes ]]; then root rm -f -- "$COMMAND"; fi
        root rm -f -- "$INSTALL_DIR/porter"
    fi
    root rm -f -- "$UNIT" "$BINARY" "$CONFIG.new" "$STATE.new" "$UNIT.new" "$INSTALL_DIR/.porter.new" "$INSTALL_DIR/.porterd.new"
    root systemctl daemon-reload || fail 'systemd 重新加载失败，请重试卸载。'
    if (( PURGE )); then
        root rm -rf -- "$DATA"
        root python3 -c "$PY_HELPER" cli-purge "$CLI_CONFIG"
        root rm -f -- "$CONFIG"
        if getent passwd porter >/dev/null; then root userdel porter; fi
        if getent group porter >/dev/null; then root groupdel porter; fi
        # 安装记录最后删除，中断时仍可确认归属并继续。
        root rm -f -- "$STATE"
        message 'Porter 已彻底删除。\n'
        REMOVE_CONFIG_DIR=1
    else
        write_state uninstalled
        message 'Porter 已卸载；配置、数据库和专用账号已保留。\n重新运行新版安装脚本可恢复原版本（需要原发行包仍可下载）。\n'
    fi
    REMOVE_INSTALL_DIR=1
}
check_command() {
    [[ $INSTALL_DIR/porter != "$COMMAND" ]] || return 0
    root python3 -c "$PY_HELPER" link-check "$COMMAND" "$INSTALL_DIR/porter" "$LINK_MANAGED" || fail '系统命令入口存在未知文件、链接或不可信目录。'
}
ensure_command() {
    [[ $INSTALL_DIR/porter != "$COMMAND" ]] || return 0
    check_command
    if [[ $LINK_MANAGED != yes ]]; then
        LINK_MANAGED=yes
        write_state "$status"
    fi
    if ! root test -L "$COMMAND"; then
        root ln -s -- "$INSTALL_DIR/porter" "$COMMAND" || fail '系统命令入口创建失败，未覆盖已有文件。'
    fi
}
show_layout() {
    if [[ $LAYOUT == split && $CLI_ENABLED == true ]]; then
        message '版本：%s；服务：%s；命令：%s rules（需要 root 或 sudo）。\n' "$VERSION" "$SERVER" "$COMMAND"
    elif [[ $LAYOUT == split ]]; then
        message '版本：%s；服务：%s；命令行工具未启用。\n' "$VERSION" "$SERVER"
    else
        message '版本：%s；旧单程序布局不包含独立命令行工具。请使用 --upgrade 显式升级。\n' "$VERSION"
    fi
}
managed_snapshot() {
    local paths=("$STATE" "$CONFIG" "$UNIT" "$BINARY" "$CLI_CONFIG")
    if [[ $LAYOUT == split && $CLI_ENABLED == true ]]; then paths+=("$INSTALL_DIR/porter"); fi
    root python3 -c "$PY_HELPER" snapshot "${paths[@]}"
}

check_managed() {
    local fragment dropins path expected
    root python3 -c "$PY_HELPER" cli-check "$CLI_CONFIG" || fail 'CLI 配置类型、属主或权限无效。'
    fragment=$(root systemctl show --property=FragmentPath --value porter.service) || fail '无法查询 systemd 服务。'
    [[ -z $fragment || $fragment == "$UNIT" ]] || fail '发现其他来源的 porter.service，拒绝覆盖。'
    dropins=$(root systemctl show --property=DropInPaths --value porter.service) || fail '无法查询 systemd 服务。'
    [[ -z $dropins ]] || fail '发现自定义 systemd 覆盖配置，拒绝卸载。'
    root test -f "$CONFIG" && root test -f "$UNIT" && root test -d "$DATA" && root test -f "$DATA/porter.db" || fail '完整安装的文件、数据目录或数据库缺失，请恢复备份；安装器不会重置配置或数据。'
    expected=$(py unit "$BINARY" "$CONFIG" "$DATA")
    [[ $(root cat "$UNIT") == "$expected" ]] || fail '现有 unit 与本安装器不符，拒绝覆盖。'
    local binaries=("$BINARY")
    [[ $LAYOUT != split || $CLI_ENABLED != true ]] || binaries+=("$INSTALL_DIR/porter")
    for path in "${binaries[@]}"; do
        root test -f "$path" && root test -x "$path" || fail '托管程序缺失，请先检查安装状态。'
        [[ $(root "$path" --version) == "$VERSION" ]] || fail '程序版本与发布标签不符。'
    done
    check_uninstall_account
    HEALTH_URL=$(root cat "$CONFIG" | py env-read "$DATA/porter.db" "$WEB_ENABLED" "$CLI_ENABLED" "$STATE_SCHEMA") || fail '现有环境配置无效；未重置任何凭据。'
    if [[ $LAYOUT == split && $CLI_ENABLED == true ]]; then check_command; fi
}
lock_upgrade() {
    root python3 -c "$PY_HELPER" lock "$INSTALL_DIR/.install-lock" "$$" || fail '安装目录被锁定，请确认没有其他安装进程。'
    LOCK=$INSTALL_DIR/.install-lock
    root python3 -c "$PY_HELPER" lock /etc/porter/.install-lock "$$" || fail '另一个安装进程正在写入全局服务配置。'
    GLOBAL_LOCK=/etc/porter/.install-lock
    local current
    current=$(root cat "$STATE" | py state-read "$INSTALL_DIR" "$ARCH") || fail '安装状态已变化，请重新运行；未继续修改托管资源。'
    [[ $current == "$state_record" ]] || fail '安装状态已变化，请重新运行；未继续修改托管资源。'
}
recover_upgrade() {
    lock_upgrade
    root python3 -c "$PY_HELPER" recover "$INSTALL_DIR" "$ARCH" "$STATE" "$CONFIG" "$DATA" "$UNIT" "$COMMAND" "$CLI_CONFIG" "$LANGUAGE" || fail '升级恢复失败；请检查安装记录及备份后重试。'
    message '已恢复稳定安装，请重新运行所需安装、升级或卸载命令。\n'
}
run_upgrade() {
    (( managed )) && [[ $status == complete ]] || fail '--upgrade 需要已完成的托管安装。'
    check_managed
    root python3 -c "$PY_HELPER" data-check "$DATA" || fail '数据目录本身是挂载根或无法校验挂载信息，不能安全执行升级回退。'
    # 旧布局不能认领已有的同目标手动链接，也不能覆盖陌生服务程序。
    if [[ $LAYOUT == legacy ]]; then
        root test ! -e "$SERVER" && root test ! -L "$SERVER" || fail '发现未托管的已有文件：%s。' "$SERVER"
    fi
    root install -d -m 0700 "$INSTALL_DIR/.tmp"
    STAGING=$(root mktemp -d "$INSTALL_DIR/.tmp/install.XXXXXXXX")
    local endpoint target archive answer before
    endpoint=https://api.github.com/repos/lwl0820/porter/releases/latest
    [[ -z $REQUESTED_VERSION ]] || endpoint=https://api.github.com/repos/lwl0820/porter/releases/tags/$REQUESTED_VERSION
    fetch "$endpoint" "$STAGING/release.json" || fail '无法获取公开 GitHub Release；请先确认正式版本已成功发布（尚无版本时 API 返回 404）。'
    target=$(root cat "$STAGING/release.json" | py release "$ARCH" "$REQUESTED_VERSION") || fail 'Release 缺少匹配架构的发行包或 SHA256SUMS。'
    py semver "$target" "$VERSION" || fail '目标版本无效或低于当前版本，拒绝降级。'
    if [[ $LAYOUT == split && $target == "$VERSION" ]]; then
        lock_upgrade
        check_managed
        if [[ $CLI_ENABLED == true ]]; then ensure_command; fi
        show_layout
        return
    fi
    archive=porter-$target-linux-$ARCH.tar.gz
    fetch "https://github.com/lwl0820/porter/releases/download/$target/$archive" "$STAGING/$archive" || fail '发行包下载失败。'
    fetch "https://github.com/lwl0820/porter/releases/download/$target/SHA256SUMS" "$STAGING/SHA256SUMS" || fail '校验和下载失败。'
    root python3 -c "$PY_HELPER" unpack "$STAGING/$archive" "$STAGING/SHA256SUMS" "$STAGING" split || fail '发行包摘要不符或缺少唯一的双程序普通文件。'
    local name
    for name in porter porterd; do
        root chmod 0755 "$STAGING/$name"
        [[ $(root "$STAGING/$name" --version) == "$target" ]] || fail '程序版本与发布标签不符。'
    done
    write_candidate "$CONFIG"
    preflight "$STAGING/porterd"
    if [[ ${old_web:-true} == false && $WEB_ENABLED == true ]]; then check_web_conflicts; fi
    py unit "$SERVER" "$CONFIG" "$DATA" | "${PRIV[@]}" tee "$STAGING/porter.service" >/dev/null
    before=$(managed_snapshot) || fail '安装状态已变化，请重新运行；未继续修改托管资源。'
    open_terminal || fail '升级需要可交互终端，请在 SSH 终端运行。'
    message '将从 %s 升级到 %s，期间短暂停服并备份完整配置和数据。\n输入 yes 确认升级，回车取消：' "$VERSION" "$target" >&3
    IFS= read -r answer <&3 || fail '终端输入已结束。'
    if [[ $answer != yes ]]; then message '已取消，未修改安装。\n'; return; fi
    lock_upgrade
    check_managed
    root python3 -c "$PY_HELPER" data-check "$DATA" || fail '数据目录本身是挂载根或无法校验挂载信息，不能安全执行升级回退。'
    [[ $(managed_snapshot) == "$before" ]] || fail '安装状态已变化，请重新运行；未继续修改托管资源。'
    if [[ $LAYOUT == legacy ]]; then
        root test ! -e "$SERVER" && root test ! -L "$SERVER" || fail '发现未托管的已有文件：%s。' "$SERVER"
    fi
    "${PRIV[@]}" python3 -c "$PY_HELPER" upgrade "$INSTALL_DIR" "$ARCH" "$STATE" "$CONFIG" "$DATA" "$UNIT" "$COMMAND" "$CLI_CONFIG" "$LANGUAGE" "$target" "$STAGING" "$WEB_ENABLED" "$CLI_ENABLED" </dev/null &
    UPGRADE_PID=$!
    if wait "$UPGRADE_PID"; then UPGRADE_PID=;
    else UPGRADE_PID=; fail '升级失败，已尝试恢复原版本；请查看恢复诊断。'; fi
    VERSION=$target; LAYOUT=split; BINARY=$SERVER; STATE_SCHEMA=3
    show_layout
}


run_reconfigure() {
    [[ $STATE_SCHEMA == 3 ]] || fail '当前版本不支持组件选择，请先使用 --upgrade 显式升级到支持的版本。'
    check_managed
    local old_web=$WEB_ENABLED old_cli=$CLI_ENABLED before answer
    select_features
    if [[ $old_web == "$WEB_ENABLED" && $old_cli == "$CLI_ENABLED" ]]; then
        show_layout
        message '组件未改变，已保留当前服务状态。\n'
        return
    fi
    root python3 -c "$PY_HELPER" data-check "$DATA" || fail '数据目录本身是挂载根或无法校验挂载信息，不能安全执行升级回退。'
    local new_web=$WEB_ENABLED new_cli=$CLI_ENABLED
    WEB_ENABLED=$old_web; CLI_ENABLED=$old_cli
    before=$(managed_snapshot) || fail '安装状态已变化，请重新运行；未继续修改托管资源。'
    WEB_ENABLED=$new_web; CLI_ENABLED=$new_cli
    if [[ $CLI_ENABLED == true && $old_cli == false ]]; then
        root test ! -e "$INSTALL_DIR/porter" && root test ! -L "$INSTALL_DIR/porter" || fail '发现未托管的已有文件：%s。' "$INSTALL_DIR/porter"
        check_command
    fi
    if [[ $WEB_ENABLED == true ]] && ! root cat "$CONFIG" | py web-valid "$DATA/porter.db"; then collect_input; fi
    root install -d -m 0700 "$INSTALL_DIR/.tmp"
    STAGING=$(root mktemp -d "$INSTALL_DIR/.tmp/install.XXXXXXXX")
    write_candidate "$CONFIG"
    root install -m 0755 "$SERVER" "$STAGING/porterd"
    if [[ $CLI_ENABLED == true ]]; then
        if [[ $old_cli == true ]]; then root install -m 0755 "$INSTALL_DIR/porter" "$STAGING/porter";
        else
            local archive=porter-$VERSION-linux-$ARCH.tar.gz release_version
            fetch "https://api.github.com/repos/lwl0820/porter/releases/tags/$VERSION" "$STAGING/release.json" || fail '无法获取公开 GitHub Release；请先确认正式版本已成功发布（尚无版本时 API 返回 404）。'
            release_version=$(root cat "$STAGING/release.json" | py release "$ARCH" "$VERSION") || fail 'Release 缺少匹配架构的发行包或 SHA256SUMS。'
            fetch "https://github.com/lwl0820/porter/releases/download/$VERSION/$archive" "$STAGING/$archive" || fail '发行包下载失败。'
            fetch "https://github.com/lwl0820/porter/releases/download/$VERSION/SHA256SUMS" "$STAGING/SHA256SUMS" || fail '校验和下载失败。'
            root rm -- "$STAGING/porterd"
            root python3 -c "$PY_HELPER" unpack "$STAGING/$archive" "$STAGING/SHA256SUMS" "$STAGING" split || fail '发行包摘要不符或缺少唯一的双程序普通文件。'
            local name
            for name in porter porterd; do
                root chmod 0755 "$STAGING/$name"
                [[ $(root "$STAGING/$name" --version) == "$VERSION" ]] || fail '程序版本与发布标签不符。'
            done
        fi
    fi
    preflight "$STAGING/porterd"
    if [[ ${old_web:-true} == false && $WEB_ENABLED == true ]]; then check_web_conflicts; fi
    py unit "$SERVER" "$CONFIG" "$DATA" | "${PRIV[@]}" tee "$STAGING/porter.service" >/dev/null
    message '当前组件 Web=%s CLI=%s；目标 Web=%s CLI=%s。将备份配置及数据并重启服务，现有连接可能断开。\n备份保留在 %s/.tmp/upgrade.<标识>。输入 yes 确认，回车取消：' "$old_web" "$old_cli" "$new_web" "$new_cli" "$INSTALL_DIR" >&3
    IFS= read -r answer <&3 || fail '终端输入已结束。'
    if [[ $answer != yes ]]; then message '已取消，未修改安装。\n'; return; fi
    WEB_ENABLED=$old_web; CLI_ENABLED=$old_cli
    lock_upgrade
    check_managed
    root python3 -c "$PY_HELPER" data-check "$DATA" || fail '数据目录本身是挂载根或无法校验挂载信息，不能安全执行升级回退。'
    [[ $(managed_snapshot) == "$before" ]] || fail '安装状态已变化，请重新运行；未继续修改托管资源。'
    WEB_ENABLED=$new_web; CLI_ENABLED=$new_cli
    if [[ $new_cli == true && $old_cli == false ]]; then
        root test ! -e "$INSTALL_DIR/porter" && root test ! -L "$INSTALL_DIR/porter" || fail '发现未托管的已有文件：%s。' "$INSTALL_DIR/porter"
        check_command
    fi
    preflight "$STAGING/porterd"
    if [[ $old_web == false && $WEB_ENABLED == true ]]; then check_web_conflicts; fi
    "${PRIV[@]}" python3 -c "$PY_HELPER" reconfigure "$INSTALL_DIR" "$ARCH" "$STATE" "$CONFIG" "$DATA" "$UNIT" "$COMMAND" "$CLI_CONFIG" "$LANGUAGE" "$VERSION" "$STAGING" "$WEB_ENABLED" "$CLI_ENABLED" </dev/null &
    UPGRADE_PID=$!
    if wait "$UPGRADE_PID"; then UPGRADE_PID=;
    else UPGRADE_PID=; fail '组件切换失败，已尝试恢复原安装；请查看恢复诊断。'; fi
    HEALTH_URL=$(root cat "$CONFIG" | py env-read "$DATA/porter.db" "$WEB_ENABLED" "$CLI_ENABLED" 3)
    show_ready
}

handle_signal() {
    local code=$1
    trap '' INT TERM
    if [[ -n ${UPGRADE_PID:-} ]]; then
        kill -TERM "$UPGRADE_PID" 2>/dev/null || true
        wait "$UPGRADE_PID" 2>/dev/null || true
    fi
    exit "$code"
}
main() {
    trap cleanup EXIT; trap 'fail "命令执行失败，请检查上方诊断后重试。"' ERR; trap 'handle_signal 130' INT; trap 'handle_signal 143' TERM
    INSTALL_DIR=/opt/porter; REQUESTED_VERSION=; UNINSTALL=0; PURGE=0; UPGRADE=0; DIRECTORY_EXPLICIT=0; LAYOUT=split; LINK_MANAGED=no; STATE_SCHEMA=3; WEB_ENABLED=true; CLI_ENABLED=true
    LANGUAGE_EXPLICIT=0
    local managed=0 status=initializing saved_version fragment ready=0 attempt response arg
    local arguments=("$@")
    # 先确定语言，确保帮助和参数错误不受参数顺序影响。
    for (( arg=0; arg<${#arguments[@]}; arg++ )); do
        if [[ ${arguments[arg]} == --lang ]]; then
            (( arg + 1 < ${#arguments[@]} )) || fail '%s 缺少值。' --lang
            LANGUAGE=${arguments[arg+1]}; LANGUAGE_EXPLICIT=1
            [[ $LANGUAGE == zh-CN || $LANGUAGE == en ]] || { LANGUAGE=en; fail '语言必须是 zh-CN 或 en。'; }
        fi
    done
    local show_help=0
    while (( $# )); do
        case "$1" in
            --help|-h) show_help=1; shift ;;
            --lang|--install-dir|--version)
                [[ $# -ge 2 && -n $2 && $2 != --* ]] || fail '%s 缺少值。' "$1"
                case "$1" in
                    --install-dir) INSTALL_DIR=$2; DIRECTORY_EXPLICIT=1 ;;
                    --version) REQUESTED_VERSION=$2 ;;
                esac
                shift 2 ;;
            --uninstall) UNINSTALL=1; shift ;;
            --purge) PURGE=1; shift ;;
            --upgrade) UPGRADE=1; shift ;;
            *) fail '未知参数：%s。' "$1" ;;
        esac
    done
    if (( show_help )); then help; return; fi
    (( ! PURGE || UNINSTALL )) || fail '--purge 仅可与 --uninstall 一起使用。'
    (( ! UNINSTALL || ! UPGRADE )) || fail '卸载不能指定 --upgrade。'
    [[ $UNINSTALL == 0 || -z $REQUESTED_VERSION ]] || fail '卸载不能指定 --version。'
    select_language
    validate_directory
    [[ -z $REQUESTED_VERSION || $REQUESTED_VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || fail '版本格式应为 v1.0.0 或明确的预发布标签。'
    check_system
    PRIV=()
    if [[ $(id -u) != 0 ]]; then
        command -v sudo >/dev/null 2>&1 || fail '请使用 root 或 sudo 账号。'
        PRIV=(sudo); sudo -v </dev/null || fail '无法取得 sudo 权限。'
    fi
    CONFIG=/etc/porter/porter.env; STATE=/etc/porter/install.json
    UNIT=/etc/systemd/system/porter.service; DATA=/var/lib/porter
    if (( UNINSTALL || UPGRADE )); then
        command -v python3 >/dev/null 2>&1 || fail '卸载需要 Python 3，请先安装后重试。'
        root python3 -c "$PY_HELPER" paths /etc/porter "$STATE" || fail '拒绝使用符号链接路径。'
        if root test -f "$STATE" && (( ! DIRECTORY_EXPLICIT )); then
            INSTALL_DIR=$(root cat "$STATE" | py state-directory) || fail '安装状态无效或目录/架构不匹配，未覆盖已有文件。'
            validate_directory
        fi
    fi
    [[ $(root realpath -ms -- "$INSTALL_DIR") == $(root realpath -m -- "$INSTALL_DIR") ]] || fail '拒绝使用符号链接路径。'
    INSTALL_DIR=$(root realpath -m -- "$INSTALL_DIR")
    [[ $INSTALL_DIR != / ]] || fail '不能安装到根目录。'
    CONFIG=/etc/porter/porter.env; STATE=/etc/porter/install.json
    UNIT=/etc/systemd/system/porter.service; DATA=/var/lib/porter; SERVER=$INSTALL_DIR/porterd; BINARY=$SERVER; COMMAND=/usr/bin/porter; CLI_CONFIG=/etc/porter/cli.json
    [[ $INSTALL_DIR != "$DATA" && $INSTALL_DIR != "$DATA/"* && $INSTALL_DIR != /etc/porter && $INSTALL_DIR != /etc/porter/* ]] || fail '程序目录不能放在 Porter 数据或配置目录内。'
    for path in /etc/porter "$CONFIG" "$STATE" "$UNIT" "$DATA" "$DATA/porter.db" "$BINARY"; do
        root test ! -L "$path" || fail '拒绝使用符号链接路径：%s。' "$path"
    done
    if (( UNINSTALL )); then
        if root test -f "$STATE"; then
            local state_record
            state_record=$(root cat "$STATE" | py state-read "$INSTALL_DIR" "$ARCH") || fail '安装状态无效或目录/架构不匹配，未覆盖已有文件。'
            read -r VERSION status LAYOUT LINK_MANAGED WEB_ENABLED CLI_ENABLED STATE_SCHEMA <<<"$state_record"
            if [[ $status == upgrading ]]; then recover_upgrade; return; fi
        fi
        uninstall; return
    fi
    if ! root test -e "$CONFIG" && ! root test -e "$STATE"; then
        open_terminal || fail '首次配置需要可交互终端，请在 SSH 终端运行。'
    fi
    local packages=(ca-certificates)
    command -v python3 >/dev/null 2>&1 || packages+=(python3)
    command -v curl >/dev/null 2>&1 || packages+=(curl)
    # 入口已使用 curl；本地执行时也按需补齐，并确保系统 CA 证书可用。
    if [[ ${#packages[@]} -gt 1 || ! -s /etc/ssl/certs/ca-certificates.crt && ! -s /etc/pki/tls/certs/ca-bundle.crt ]]; then
        if [[ $FAMILY == apt ]]; then
            root env DEBIAN_FRONTEND=noninteractive apt-get update
            root env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-remove --no-install-recommends "${packages[@]}"
        else root dnf install -y --setopt=obsoletes=False "${packages[@]}"; fi
    fi
    root python3 -c "$PY_HELPER" paths "$INSTALL_DIR" /etc/porter "$CONFIG" "$STATE" "$UNIT" "$DATA" "$DATA/porter.db" "$BINARY" "$STATE.new" "$CONFIG.new" "$UNIT.new" "$INSTALL_DIR/.porter.new" "$INSTALL_DIR/.tmp" "$SERVER" "$INSTALL_DIR/.porterd.new" "$CLI_CONFIG" || fail '拒绝使用符号链接路径。'
    if root test -e "$STATE"; then
        local state_record
        state_record=$(root cat "$STATE" | py state-read "$INSTALL_DIR" "$ARCH") || fail '安装状态无效或目录/架构不匹配，未覆盖已有文件。'
        read -r saved_version status LAYOUT LINK_MANAGED WEB_ENABLED CLI_ENABLED STATE_SCHEMA <<<"$state_record"
        if [[ $LAYOUT == split ]]; then BINARY=$SERVER; else BINARY=$INSTALL_DIR/porter; fi
        VERSION=$saved_version; managed=1
        root python3 -c "$PY_HELPER" paths "$BINARY" || fail '拒绝使用符号链接路径。'
        [[ $UPGRADE == 1 || -z $REQUESTED_VERSION || $REQUESTED_VERSION == "$VERSION" ]] || fail '已有安装不能通过 --version 升级，请按 README 显式升级。'
    else
        fragment=$(root systemctl show --property=FragmentPath --value porter.service) || fail '无法查询 systemd 服务。'
        [[ -z $fragment ]] || fail '发现已有 porter.service，拒绝覆盖未知服务。'
        for path in "$CONFIG" "$UNIT" "$DATA" "$BINARY" "$SERVER"; do
            root test ! -e "$path" || fail '发现未托管的已有文件：%s。' "$path"
        done
        if getent passwd porter >/dev/null || getent group porter >/dev/null; then fail '发现未知的 porter 账号或组，拒绝接管。'; fi
    fi
    if [[ $status == upgrading ]]; then recover_upgrade; return; fi
    if (( UPGRADE )); then run_upgrade; return; fi
    [[ $status != uninstalling ]] || fail '卸载尚未完成，请先重新运行 --uninstall。'
    if [[ $status == complete ]]; then
        if [[ $LAYOUT == split && $CLI_ENABLED == true ]]; then
            root test -f "$INSTALL_DIR/porter" && root test -x "$INSTALL_DIR/porter" || fail '完整安装的文件、数据目录或数据库缺失，请恢复备份；安装器不会重置配置或数据。'
        fi
        root test -x "$BINARY" && root test -f "$CONFIG" && root test -f "$UNIT" &&
            root test -d "$DATA" && root test -f "$DATA/porter.db" ||
            fail '完整安装的文件、数据目录或数据库缺失，请恢复备份；安装器不会重置配置或数据。'
    fi
    if [[ $status == complete ]] && open_terminal; then
        local change
        message '当前组件 Web=%s CLI=%s。\n' "$WEB_ENABLED" "$CLI_ENABLED" >&3
        message '是否修改管理组件？[no]：' >&3
        IFS= read -r change <&3 || fail '终端输入已结束。'
        case "$change" in
            y|Y|yes|YES|是|1) run_reconfigure; return ;;
            ''|n|N|no|NO|否|0) ;;
            *) fail '请输入 yes 或 no（也可输入 y 或 n）。\n' ;;
        esac
    fi
    if [[ $status == uninstalled ]]; then
        if [[ $LAYOUT == split && $CLI_ENABLED == true ]]; then root test ! -e "$INSTALL_DIR/porter" || fail '已卸载状态存在意外程序或服务，请先检查。'; fi
        root test -f "$CONFIG" && root test -d "$DATA" && root test -f "$DATA/porter.db" || fail '保留的配置或数据库缺失，请恢复备份。'
        root test ! -e "$BINARY" && root test ! -e "$UNIT" || fail '已卸载状态存在意外程序或服务，请先检查。'
    fi
    if root test -e "$CONFIG"; then
        HEALTH_URL=$(root cat "$CONFIG" | py env-read "$DATA/porter.db" "$WEB_ENABLED" "$CLI_ENABLED" "$STATE_SCHEMA") || fail '现有环境配置无效；未重置任何凭据。'
        if [[ ${TTY_OPEN:-0} == 1 ]]; then exec 3>&-; TTY_OPEN=0; fi
    else
        select_features
        if [[ $WEB_ENABLED == true ]]; then collect_input; else HEALTH_URL=-; fi
    fi
    if [[ $LAYOUT == split ]]; then
        BINARY=$SERVER
        if (( ! managed )) && [[ $CLI_ENABLED == true ]]; then
            root test ! -e "$INSTALL_DIR/porter" && root test ! -L "$INSTALL_DIR/porter" || fail '发现未托管的已有文件：%s。' "$INSTALL_DIR/porter"
        fi
        if [[ $CLI_ENABLED == true ]]; then check_command; fi
    fi
    root install -d -m 0755 "$INSTALL_DIR"
    root python3 -c "$PY_HELPER" lock "$INSTALL_DIR/.install-lock" "$$" || fail '安装目录被锁定，请确认没有其他安装进程。'
    LOCK=$INSTALL_DIR/.install-lock
    root install -d -m 0700 "$INSTALL_DIR/.tmp"
    STAGING=$(root mktemp -d "$INSTALL_DIR/.tmp/install.XXXXXXXX")
    py unit "$BINARY" "$CONFIG" "$DATA" | "${PRIV[@]}" tee "$STAGING/porter.service" >/dev/null
    fragment=$(root systemctl show --property=FragmentPath --value porter.service) || fail '无法查询 systemd 服务。'
    [[ -z $fragment || $fragment == "$UNIT" ]] || fail '发现其他来源的 porter.service，拒绝覆盖。'
    if root test -e "$UNIT"; then
        root cmp --silent "$STAGING/porter.service" "$UNIT" || fail '现有 unit 与本安装器不符，拒绝覆盖。'
    fi
    if ! root test -e "$BINARY"; then
        local endpoint archive
        endpoint=https://api.github.com/repos/lwl0820/porter/releases/latest
        if (( managed )); then REQUESTED_VERSION=$VERSION; fi
        [[ -z $REQUESTED_VERSION ]] || endpoint=https://api.github.com/repos/lwl0820/porter/releases/tags/$REQUESTED_VERSION
        fetch "$endpoint" "$STAGING/release.json" || fail '无法获取公开 GitHub Release；请先确认正式版本已成功发布（尚无版本时 API 返回 404）。'
        VERSION=$(root cat "$STAGING/release.json" | py release "$ARCH" "$REQUESTED_VERSION") || fail 'Release 缺少匹配架构的发行包或 SHA256SUMS。'
        archive=porter-$VERSION-linux-$ARCH.tar.gz
        fetch "https://github.com/lwl0820/porter/releases/download/$VERSION/$archive" "$STAGING/$archive" || fail '发行包下载失败。'
        fetch "https://github.com/lwl0820/porter/releases/download/$VERSION/SHA256SUMS" "$STAGING/SHA256SUMS" || fail '校验和下载失败。'
        root python3 -c "$PY_HELPER" unpack "$STAGING/$archive" "$STAGING/SHA256SUMS" "$STAGING" "$LAYOUT" || fail '发行包摘要不符或不含唯一的 porter 普通文件。'
        for name in porter $([[ $LAYOUT != split ]] || printf porterd); do
            root chmod 0755 "$STAGING/$name"
            [[ $(root "$STAGING/$name" --version) == "$VERSION" ]] || fail '程序版本与发布标签不符。'
        done
    else root test -x "$BINARY" || fail '已有程序不可执行，拒绝覆盖。'; fi
    root install -d -m 0755 /etc/porter
    root python3 -c "$PY_HELPER" lock /etc/porter/.install-lock "$$" || fail '另一个安装进程正在写入全局服务配置。'
    GLOBAL_LOCK=/etc/porter/.install-lock
    if (( managed )); then
        # 前置校验到取得两把锁之间可能已完成卸载，不能继续使用旧记录。
        local current_record
        current_record=$(root cat "$STATE" | py state-read "$INSTALL_DIR" "$ARCH") || fail '安装状态已变化，请重新运行；未继续修改托管资源。'
        [[ $current_record == "$state_record" ]] || fail '安装状态已变化，请重新运行；未继续修改托管资源。'
    fi
    if [[ $STATE_SCHEMA == 3 ]]; then
        if root test -f "$CONFIG"; then write_candidate "$CONFIG"; else write_candidate; fi
        local candidate=$BINARY
        if root test -f "$STAGING/porterd"; then candidate=$STAGING/porterd; fi
        preflight "$candidate"
    fi
    root python3 -c "$PY_HELPER" cli-check "$CLI_CONFIG" || fail 'CLI 配置类型、属主或权限无效。'
    if [[ $LAYOUT == split && $CLI_ENABLED == true ]]; then check_command; fi
    if (( ! managed )); then
        root test ! -e "$STATE" || fail '安装期间出现已有安装状态，请重新运行。'
        for path in "$CONFIG" "$UNIT" "$DATA" "$BINARY" "$SERVER"; do
            root test ! -e "$path" || fail '安装期间出现已有文件：%s。' "$path"
        done
        if getent passwd porter >/dev/null || getent group porter >/dev/null; then fail '安装期间出现同名账号或组，拒绝接管。'; fi
        if [[ $CLI_ENABLED == true ]]; then
            root test ! -e "$INSTALL_DIR/porter" && root test ! -L "$INSTALL_DIR/porter" || fail '安装期间出现已有文件：%s。' "$INSTALL_DIR/porter"
        fi
        [[ $LAYOUT != split || $CLI_ENABLED != true || $INSTALL_DIR/porter == "$COMMAND" ]] || LINK_MANAGED=yes
        write_state initializing
    fi
    if [[ $status == uninstalled ]]; then write_state initializing; fi
    if ! root test -e "$CONFIG"; then
        root install -m 0600 "$STAGING/porter.env" "$CONFIG.new"
        root mv -- "$CONFIG.new" "$CONFIG"
        unset USERNAME PASSWORD LISTEN
    fi
    for name in $(program_names); do
        if root test -e "$INSTALL_DIR/$name"; then
            [[ $(root "$INSTALL_DIR/$name" --version) == "$VERSION" ]] || fail '程序版本与发布标签不符。'
        fi
    done
    root chmod 0600 "$CONFIG" "$STATE"
    check_account
    root install -d -o porter -g porter -m 0700 "$DATA"
    for name in $(program_names); do
        if ! root test -e "$INSTALL_DIR/$name"; then
            root install -m 0755 "$STAGING/$name" "$INSTALL_DIR/.$name.new"
            root mv -- "$INSTALL_DIR/.$name.new" "$INSTALL_DIR/$name"
        fi
    done
    if [[ $LAYOUT == split && $CLI_ENABLED == true ]]; then ensure_command; fi
    if ! root test -e "$UNIT"; then
        root install -m 0644 "$STAGING/porter.service" "$UNIT.new"
        root mv -- "$UNIT.new" "$UNIT"
    fi
    if [[ $status != complete ]]; then
        root systemctl daemon-reload
        root systemctl enable --now porter.service || fail 'systemd 启动失败，请检查 journalctl -u porter.service。'
    elif ! root systemctl is-active --quiet porter.service; then
        check_inactive
        show_layout
        message '安装完整，服务保持停止状态。\n'
        return
    fi
    for attempt in {1..30}; do
        if root systemctl is-active --quiet porter.service && probe_ready &&
            root systemctl is-active --quiet porter.service &&
            { [[ $status == complete ]] || root systemctl is-enabled --quiet porter.service; }; then ready=1; break; fi
        sleep 2
    done
    (( ready )) || fail '服务未就绪，请检查 journalctl -u porter.service 和监听设置。'
    write_state complete
    show_ready
    message '配置：%s（权限 600）；数据：%s；程序：%s\n' "$CONFIG" "$DATA" "$BINARY"
    local prefix=
    if (( ${#PRIV[@]} )); then prefix='sudo '; fi
    message '日志：%sjournalctl -u porter.service -f\n停止：%ssystemctl stop porter.service\n启动：%ssystemctl start porter.service\n' "$prefix" "$prefix" "$prefix"
}
main "$@"
