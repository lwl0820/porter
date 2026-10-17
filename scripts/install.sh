#!/usr/bin/env bash
# 管道仅提供脚本，交互始终读取控制终端。
set +x
set -Eeuo pipefail
umask 077

fail() { printf '错误：%s\n' "$*" >&2; exit 1; }
help() {
    cat <<'HELP'
Porter 原生交互安装器
用法：bash install.sh [--install-dir /opt/porter] [--version v1.0.0]
默认下载最新正式 GitHub Release；首次输入用户名、隐藏密码及完整 Web IP:端口。
支持 Ubuntu 22.04/24.04、Debian 12/13、RHEL/Rocky/AlmaLinux 9/10，amd64/arm64。
要求 systemd 为 PID 1，以及 root 或 sudo 权限；无需 Docker 或编译工具。
重复运行保留程序、配置和数据，不自动升级；目录不支持引号或反斜线。
HELP
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
        if state["schema"] != 1 or state["directory"] != args[0] or state["arch"] != args[1] or not re.fullmatch(VERSION, state["version"]) or state["status"] not in ("initializing", "complete"):
            raise ValueError()
        print(state["version"], state["status"])
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
        *) fail "不支持的发行版或版本：$ID $VERSION_ID。" ;;
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
    unset USERNAME PASSWORD CONFIRM LISTEN
    while true; do
        printf '管理员用户名：' >&3
        IFS= read -r USERNAME <&3 || fail '终端输入已结束。'
        USERNAME=$(printf '%s' "$USERNAME" | py trim)
        [[ -n $USERNAME ]] && break
        printf '用户名不能为空，请重新输入。\n' >&3
    done
    TTY_STATE=$(stty -g <&3); TTY_HIDDEN=1; stty -echo <&3
    while true; do
        printf '管理员密码：' >&3
        IFS= read -rs PASSWORD <&3 || fail '终端输入已结束。'
        printf '\n确认密码：' >&3
        IFS= read -rs CONFIRM <&3 || fail '终端输入已结束。'
        printf '\n' >&3
        if [[ -z $(printf '%s' "$PASSWORD" | py trim) ]]; then
            printf '密码不能全部为空白，请重新输入。\n' >&3
        elif [[ $PASSWORD != "$CONFIRM" ]]; then
            printf '两次密码不一致，请重新输入。\n' >&3
        else break; fi
    done
    unset CONFIRM; restore_terminal
    while true; do
        printf 'Web 监听地址（如 0.0.0.0:8080 或 [::]:8080）：' >&3
        IFS= read -r LISTEN <&3 || fail '终端输入已结束。'
        if HEALTH_URL=$(py address "$LISTEN"); then break; fi
        printf '请输入有效的 IPv4:端口 或 [IPv6]:端口，端口范围 1–65535。\n' >&3
    done
    exec 3>&-
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
main() {
    trap cleanup EXIT; trap 'exit 130' INT; trap 'exit 143' TERM
    INSTALL_DIR=/opt/porter; REQUESTED_VERSION=
    local managed=0 status=initializing saved_version fragment ready=0 attempt response
    while (( $# )); do
        case "$1" in
            --help|-h) help; return ;;
            --install-dir|--version)
                [[ $# -ge 2 && -n $2 ]] || fail "$1 缺少值。"
                if [[ $1 == --install-dir ]]; then INSTALL_DIR=$2; else REQUESTED_VERSION=$2; fi
                shift 2 ;;
            *) fail "未知参数：$1。" ;;
        esac
    done
    [[ $INSTALL_DIR == /* && ! $INSTALL_DIR =~ [[:cntrl:]] ]] || fail '安装目录必须是无控制字符的绝对路径。'
    case "$INSTALL_DIR" in *\\*|*\"*|*\'*) fail '安装目录不能包含引号或反斜线（systemd 路径限制）。' ;; esac
    [[ -z $REQUESTED_VERSION || $REQUESTED_VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || fail '版本格式应为 v1.0.0 或明确的预发布标签。'
    check_system
    PRIV=()
    if [[ $(id -u) != 0 ]]; then
        command -v sudo >/dev/null 2>&1 || fail '请使用 root 或 sudo 账号。'
        PRIV=(sudo); sudo -v </dev/null || fail '无法取得 sudo 权限。'
    fi
    INSTALL_DIR=$(root realpath -m -- "$INSTALL_DIR")
    [[ $INSTALL_DIR != / ]] || fail '不能安装到根目录。'
    CONFIG=/etc/porter/porter.env; STATE=/etc/porter/install.json
    UNIT=/etc/systemd/system/porter.service; DATA=/var/lib/porter; BINARY=$INSTALL_DIR/porter
    [[ $INSTALL_DIR != "$DATA" && $INSTALL_DIR != "$DATA/"* && $INSTALL_DIR != /etc/porter && $INSTALL_DIR != /etc/porter/* ]] || fail '程序目录不能放在 Porter 数据或配置目录内。'
    for path in /etc/porter "$CONFIG" "$STATE" "$UNIT" "$DATA" "$DATA/porter.db" "$BINARY"; do
        root test ! -L "$path" || fail "拒绝使用符号链接路径：$path。"
    done
    if ! root test -e "$CONFIG"; then
        { exec 3<>/dev/tty; } 2>/dev/null || fail '首次配置需要可交互终端，请在 SSH 终端运行。'
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
            root test ! -e "$path" || fail "发现未托管的已有文件：$path。"
        done
        if getent passwd porter >/dev/null || getent group porter >/dev/null; then fail '发现未知的 porter 账号或组，拒绝接管。'; fi
    fi
    if [[ $status == complete ]]; then
        root test -x "$BINARY" && root test -f "$CONFIG" && root test -f "$UNIT" &&
            root test -d "$DATA" && root test -f "$DATA/porter.db" ||
            fail '完整安装的文件、数据目录或数据库缺失，请恢复备份；安装器不会重置配置或数据。'
    fi
    if root test -e "$CONFIG"; then
        HEALTH_URL=$(root cat "$CONFIG" | py env-read "$DATA/porter.db") || fail '现有环境配置无效；未重置任何凭据。'
        exec 3>&-
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
    if (( ! managed )); then
        root test ! -e "$STATE" || fail '安装期间出现已有安装状态，请重新运行。'
        for path in "$CONFIG" "$UNIT" "$DATA" "$BINARY"; do
            root test ! -e "$path" || fail "安装期间出现已有文件：$path。"
        done
        if getent passwd porter >/dev/null || getent group porter >/dev/null; then fail '安装期间出现同名账号或组，拒绝接管。'; fi
        write_state initializing
    fi
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
    printf '\nPorter 已就绪：%s/admin/\n通配监听时，远程访问请改用服务器 IP。\n' "$HEALTH_URL"
    printf '配置：%s（权限 600）；数据：%s；程序：%s\n' "$CONFIG" "$DATA" "$BINARY"
    local prefix=
    if (( ${#PRIV[@]} )); then prefix='sudo '; fi
    printf '日志：%sjournalctl -u porter.service -f\n停止：%ssystemctl stop porter.service\n启动：%ssystemctl start porter.service\n' "$prefix" "$prefix" "$prefix"
}
main "$@"
