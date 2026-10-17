#!/usr/bin/env bash
# 管道仅提供脚本，交互始终读取控制终端。
set +x
set -Eeuo pipefail
umask 077

LANGUAGE=zh-CN
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
       bash install.sh --uninstall [--purge] [--install-dir /opt/porter] [--lang zh-CN|en]
Install the latest stable GitHub Release; enter an admin username, hidden password and Web IP:port.
Supports Ubuntu 22.04/24.04, Debian 12/13, RHEL/Rocky/AlmaLinux 9/10, amd64/arm64.
Requires systemd as PID 1 and root or sudo; no Docker or compiler needed.
Repeating installation keeps the program, configuration and data; it does not upgrade.
Uninstall keeps configuration, all database files and the service account by default.
--purge additionally deletes configuration, data and the service account after a second confirmation.
Reinstall after uninstall restores the recorded version and retained configuration/data.
Paths cannot contain quotes or backslashes. --version cannot be used with --uninstall.
HELP
    else
        cat <<'HELP'
Porter 原生交互安装与卸载器
用法：bash install.sh [--install-dir /opt/porter] [--version v1.0.0] [--lang zh-CN|en]
      bash install.sh --uninstall [--purge] [--install-dir /opt/porter] [--lang zh-CN|en]
默认下载最新正式 GitHub Release；首次输入用户名、隐藏密码及完整 Web IP:端口。
支持 Ubuntu 22.04/24.04、Debian 12/13、RHEL/Rocky/AlmaLinux 9/10，amd64/arm64。
要求 systemd 为 PID 1，以及 root 或 sudo 权限；无需 Docker 或编译工具。
重复运行保留程序、配置和数据，不自动升级；目录不支持引号或反斜线。
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
        printf '语言 / Language [1 中文, 2 English] (1): ' >&3
        local answer
        IFS= read -r answer <&3 || fail '终端输入已结束。'
        case "$answer" in
            ''|1|zh-CN) LANGUAGE=zh-CN ;;
            2|en) LANGUAGE=en ;;
            *) fail '语言必须是 zh-CN 或 en。' ;;
        esac
    fi
}

# 使用标准库校验地址、产物与安装状态；凭据只经标准输入传递。
PY_HELPER='
import hashlib, ipaddress, json, os, re, sys, tarfile
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
    if not {"PORTER_ADMIN_USERNAME", "PORTER_ADMIN_PASSWORD", "PORTER_HTTP_ADDR", "PORTER_DB_PATH"} <= set(values):
        raise ValueError()
    if not values["PORTER_ADMIN_USERNAME"].strip() or not values["PORTER_ADMIN_PASSWORD"].strip():
        raise ValueError()
    return values

try:
    mode, *args = sys.argv[1:]
    if mode == "trim":
        print(sys.stdin.read().strip())
    elif mode == "address":
        print(address(args[0]))
    elif mode == "env-write":
        username, password, listen, _ = sys.stdin.read().split("\0")
        for key, value in zip(("PORTER_ADMIN_USERNAME", "PORTER_ADMIN_PASSWORD", "PORTER_HTTP_ADDR", "PORTER_DB_PATH"), (username, password, listen, args[0])):
            print(key + "=" + quote(value))
    elif mode == "env-read":
        values = read_environment(sys.stdin.read())
        if values["PORTER_DB_PATH"] != args[0]:
            raise ValueError()
        print(address(values["PORTER_HTTP_ADDR"]))
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
        archive, checksums, output = args
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
            members = [member for member in bundle.getmembers() if member.name == "porter"]
            if len(members) != 1 or not members[0].isreg() or not 0 < members[0].size <= 268435456:
                raise ValueError()
            # 只复制精确普通文件，不解包路径、链接或其他成员。
            with bundle.extractfile(members[0]) as source, open(output, "xb") as destination:
                while True:
                    block = source.read(1048576)
                    if not block:
                        break
                    destination.write(block)
    elif mode == "state-write":
        json.dump({"schema": 1, "directory": args[0], "arch": args[1], "version": args[2], "status": args[3]}, sys.stdout)
    elif mode == "state-read":
        state = json.load(sys.stdin)
        if type(state.get("schema")) is not int or state["schema"] != 1 or state["directory"] != args[0] or state["arch"] != args[1] or not re.fullmatch(VERSION, state["version"]) or state["status"] not in ("initializing", "complete", "uninstalling", "uninstalled"):
            raise ValueError()
        print(state["version"], state["status"])
    elif mode == "state-directory":
        state = json.load(sys.stdin)
        if type(state.get("schema")) is not int or state.get("schema") != 1 or not isinstance(state.get("directory"), str) or "\n" in state["directory"]:
            raise ValueError()
        print(state["directory"])
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
except (ValueError, KeyError, TypeError, OSError, tarfile.TarError):
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
    if [[ -n ${GLOBAL_LOCK:-} ]]; then root rmdir -- "$GLOBAL_LOCK" || true; fi
    if [[ -n ${LOCK:-} ]]; then root rmdir -- "$LOCK" || true; fi
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

fetch() {
    root curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
        --connect-timeout 10 --max-time 180 "$1" -o "$2"
}
write_state() {
    py state-write "$INSTALL_DIR" "$ARCH" "$VERSION" "$1" | "${PRIV[@]}" tee "$STAGING/install.json" >/dev/null
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
    root python3 -c "$PY_HELPER" paths "$INSTALL_DIR" /etc/porter "$CONFIG" "$STATE" "$UNIT" "$DATA" "$DATA/porter.db" "$BINARY" "$STATE.new" "$CONFIG.new" "$UNIT.new" "$INSTALL_DIR/.porter.new" "$INSTALL_DIR/.tmp" || fail '拒绝使用符号链接路径。'
    if ! root test -e "$STATE"; then
        fragment=$(root systemctl show --property=FragmentPath --value porter.service) || fail '无法查询 systemd 服务。'
        [[ -z $fragment ]] || fail '发现已有 porter.service，拒绝覆盖未知服务。'
        for path in "$CONFIG" "$UNIT" "$DATA" "$BINARY" "$CONFIG.new" "$STATE.new" "$UNIT.new" "$INSTALL_DIR/.porter.new"; do
            root test ! -e "$path" || fail '发现未托管的已有文件：%s。' "$path"
        done
        if getent passwd porter >/dev/null || getent group porter >/dev/null; then fail '发现未知的 porter 账号或组，拒绝接管。'; fi
        check_inactive
        message 'Porter 已卸载，无需重复操作。\n'
        return
    fi
    # 与安装共用锁；失败清理仅释放本次成功获取的锁。
    root mkdir /etc/porter/.install-lock || fail '另一个安装进程正在写入全局服务配置。'
    GLOBAL_LOCK=/etc/porter/.install-lock
    if root test -d "$INSTALL_DIR"; then
        root mkdir "$INSTALL_DIR/.install-lock" || fail '安装目录被锁定，请确认没有其他安装进程。'
        LOCK=$INSTALL_DIR/.install-lock
    elif root test -e "$INSTALL_DIR"; then fail '程序目录不是目录。'; fi
    local record
    record=$(root cat "$STATE" | py state-read "$INSTALL_DIR" "$ARCH") || fail '安装状态无效或目录/架构不匹配，未覆盖已有文件。'
    read -r saved_version status <<<"$record"
    VERSION=$saved_version
    for path in "$CONFIG" "$DATA/porter.db" "$CONFIG.new" "$STATE.new" "$UNIT.new" "$INSTALL_DIR/.porter.new"; do
        if root test -e "$path"; then
            root test -f "$path" || fail '托管配置或数据的文件类型不符，拒绝卸载。'
        fi
    done
    if root test -e "$DATA"; then
        root test -d "$DATA" || fail '托管配置或数据的文件类型不符，拒绝卸载。'
    fi
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
    check_uninstall_account
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
    STAGING=$GLOBAL_LOCK/work
    root mkdir "$STAGING"
    write_state uninstalling
    if root test -f "$UNIT"; then
        root systemctl stop porter.service || fail '停止服务失败，请重试卸载；尚未删除文件。'
        root systemctl disable porter.service || fail '禁用开机启动失败，请重试卸载；尚未删除文件。'
    fi
    check_inactive
    root rm -f -- "$UNIT" "$BINARY" "$CONFIG.new" "$STATE.new" "$UNIT.new" "$INSTALL_DIR/.porter.new"
    root systemctl daemon-reload || fail 'systemd 重新加载失败，请重试卸载。'
    if (( PURGE )); then
        root rm -rf -- "$DATA"
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
main() {
    trap cleanup EXIT; trap 'fail "命令执行失败，请检查上方诊断后重试。"' ERR; trap 'exit 130' INT; trap 'exit 143' TERM
    INSTALL_DIR=/opt/porter; REQUESTED_VERSION=; UNINSTALL=0; PURGE=0; DIRECTORY_EXPLICIT=0
    LANGUAGE_EXPLICIT=0
    local managed=0 status=initializing saved_version fragment ready=0 attempt response arg
    local arguments=("$@")
    # 先确定语言，确保帮助和参数错误不受参数顺序影响。
    for (( arg=0; arg<${#arguments[@]}; arg++ )); do
        if [[ ${arguments[arg]} == --lang ]]; then
            (( arg + 1 < ${#arguments[@]} )) || fail '%s 缺少值。' --lang
            LANGUAGE=${arguments[arg+1]}; LANGUAGE_EXPLICIT=1
            [[ $LANGUAGE == zh-CN || $LANGUAGE == en ]] || { LANGUAGE=zh-CN; fail '语言必须是 zh-CN 或 en。'; }
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
            *) fail '未知参数：%s。' "$1" ;;
        esac
    done
    if (( show_help )); then help; return; fi
    (( ! PURGE || UNINSTALL )) || fail '--purge 仅可与 --uninstall 一起使用。'
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
    if (( UNINSTALL )); then
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
    UNIT=/etc/systemd/system/porter.service; DATA=/var/lib/porter; BINARY=$INSTALL_DIR/porter
    [[ $INSTALL_DIR != "$DATA" && $INSTALL_DIR != "$DATA/"* && $INSTALL_DIR != /etc/porter && $INSTALL_DIR != /etc/porter/* ]] || fail '程序目录不能放在 Porter 数据或配置目录内。'
    for path in /etc/porter "$CONFIG" "$STATE" "$UNIT" "$DATA" "$DATA/porter.db" "$BINARY"; do
        root test ! -L "$path" || fail '拒绝使用符号链接路径：%s。' "$path"
    done
    if (( UNINSTALL )); then uninstall; return; fi
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
    root python3 -c "$PY_HELPER" paths "$INSTALL_DIR" /etc/porter "$CONFIG" "$STATE" "$UNIT" "$DATA" "$DATA/porter.db" "$BINARY" "$STATE.new" "$CONFIG.new" "$UNIT.new" "$INSTALL_DIR/.porter.new" "$INSTALL_DIR/.tmp" || fail '拒绝使用符号链接路径。'
    if root test -e "$STATE"; then
        local state_record
        state_record=$(root cat "$STATE" | py state-read "$INSTALL_DIR" "$ARCH") || fail '安装状态无效或目录/架构不匹配，未覆盖已有文件。'
        read -r saved_version status <<<"$state_record"
        VERSION=$saved_version; managed=1
        [[ -z $REQUESTED_VERSION || $REQUESTED_VERSION == "$VERSION" ]] || fail '已有安装不能通过 --version 升级，请按 README 显式升级。'
    else
        fragment=$(root systemctl show --property=FragmentPath --value porter.service) || fail '无法查询 systemd 服务。'
        [[ -z $fragment ]] || fail '发现已有 porter.service，拒绝覆盖未知服务。'
        for path in "$CONFIG" "$UNIT" "$DATA" "$BINARY"; do
            root test ! -e "$path" || fail '发现未托管的已有文件：%s。' "$path"
        done
        if getent passwd porter >/dev/null || getent group porter >/dev/null; then fail '发现未知的 porter 账号或组，拒绝接管。'; fi
    fi
    [[ $status != uninstalling ]] || fail '卸载尚未完成，请先重新运行 --uninstall。'
    if [[ $status == complete ]]; then
        root test -x "$BINARY" && root test -f "$CONFIG" && root test -f "$UNIT" &&
            root test -d "$DATA" && root test -f "$DATA/porter.db" ||
            fail '完整安装的文件、数据目录或数据库缺失，请恢复备份；安装器不会重置配置或数据。'
    fi
    if [[ $status == uninstalled ]]; then
        root test -f "$CONFIG" && root test -d "$DATA" && root test -f "$DATA/porter.db" || fail '保留的配置或数据库缺失，请恢复备份。'
        root test ! -e "$BINARY" && root test ! -e "$UNIT" || fail '已卸载状态存在意外程序或服务，请先检查。'
    fi
    if root test -e "$CONFIG"; then
        HEALTH_URL=$(root cat "$CONFIG" | py env-read "$DATA/porter.db") || fail '现有环境配置无效；未重置任何凭据。'
        if [[ ${TTY_OPEN:-0} == 1 ]]; then exec 3>&-; TTY_OPEN=0; fi
    else collect_input; fi
    root install -d -m 0755 "$INSTALL_DIR"
    root mkdir "$INSTALL_DIR/.install-lock" || fail '安装目录被锁定，请确认没有其他安装进程。'
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
        root python3 -c "$PY_HELPER" unpack "$STAGING/$archive" "$STAGING/SHA256SUMS" "$STAGING/porter" || fail '发行包摘要不符或不含唯一的 porter 普通文件。'
        root chmod 0755 "$STAGING/porter"
        [[ $(root "$STAGING/porter" --version) == "$VERSION" ]] || fail '程序版本与发布标签不符。'
    else root test -x "$BINARY" || fail '已有程序不可执行，拒绝覆盖。'; fi
    root install -d -m 0755 /etc/porter
    root mkdir /etc/porter/.install-lock || fail '另一个安装进程正在写入全局服务配置。'
    GLOBAL_LOCK=/etc/porter/.install-lock
    if (( managed )); then
        # 前置校验到取得两把锁之间可能已完成卸载，不能继续使用旧记录。
        local current_record
        current_record=$(root cat "$STATE" | py state-read "$INSTALL_DIR" "$ARCH") || fail '安装状态已变化，请重新运行；未继续修改托管资源。'
        [[ $current_record == "$state_record" ]] || fail '安装状态已变化，请重新运行；未继续修改托管资源。'
    fi
    if (( ! managed )); then
        root test ! -e "$STATE" || fail '安装期间出现已有安装状态，请重新运行。'
        for path in "$CONFIG" "$UNIT" "$DATA" "$BINARY"; do
            root test ! -e "$path" || fail '安装期间出现已有文件：%s。' "$path"
        done
        if getent passwd porter >/dev/null || getent group porter >/dev/null; then fail '安装期间出现同名账号或组，拒绝接管。'; fi
        write_state initializing
    fi
    if [[ $status == uninstalled ]]; then write_state initializing; fi
    if ! root test -e "$CONFIG"; then
        printf '%s\0%s\0%s\0' "$USERNAME" "$PASSWORD" "$LISTEN" | py env-write "$DATA/porter.db" | "${PRIV[@]}" tee "$STAGING/porter.env" >/dev/null
        root install -m 0600 "$STAGING/porter.env" "$CONFIG.new"
        root mv -- "$CONFIG.new" "$CONFIG"
        unset USERNAME PASSWORD LISTEN
    fi
    root chmod 0600 "$CONFIG" "$STATE"
    check_account
    root install -d -o porter -g porter -m 0700 "$DATA"
    if ! root test -e "$BINARY"; then
        root install -m 0755 "$STAGING/porter" "$INSTALL_DIR/.porter.new"
        root mv -- "$INSTALL_DIR/.porter.new" "$BINARY"
    fi
    if ! root test -e "$UNIT"; then
        root install -m 0644 "$STAGING/porter.service" "$UNIT.new"
        root mv -- "$UNIT.new" "$UNIT"
    fi
    root systemctl daemon-reload
    root systemctl enable --now porter.service || fail 'systemd 启动失败，请检查 journalctl -u porter.service。'
    for attempt in {1..30}; do
        if root systemctl is-active --quiet porter.service &&
            response=$(curl --fail --silent --show-error --noproxy '*' --connect-timeout 2 --max-time 2 "$HEALTH_URL/healthz" 2>/dev/null) &&
            [[ $response == '{"status":"ok"}' ]] && root systemctl is-active --quiet porter.service &&
            root systemctl is-enabled --quiet porter.service; then ready=1; break; fi
        sleep 2
    done
    (( ready )) || fail '服务未就绪，请检查 journalctl -u porter.service 和监听设置。'
    write_state complete
    message '\nPorter 已就绪\n用户入口：%s/user/\n管理员入口：%s/admin/\n通配监听时，远程访问请改用服务器 IP。\n' "$HEALTH_URL" "$HEALTH_URL"
    message '配置：%s（权限 600）；数据：%s；程序：%s\n' "$CONFIG" "$DATA" "$BINARY"
    local prefix=
    if (( ${#PRIV[@]} )); then prefix='sudo '; fi
    message '日志：%sjournalctl -u porter.service -f\n停止：%ssystemctl stop porter.service\n启动：%ssystemctl start porter.service\n' "$prefix" "$prefix" "$prefix"
}
main "$@"
