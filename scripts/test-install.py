#!/usr/bin/env python3
"""使用发行包、命令替身和伪终端验证原生安装，不修改宿主服务或账号。"""
import fcntl
import hashlib
import io
import json
import os
from pathlib import Path
import pty
import select
import shutil
import subprocess
import tarfile
import tempfile
import termios
import time
import unittest

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = (ROOT / "scripts/install.sh").read_text()
REAL_PYTHON = shutil.which("python3")

STUB = r'''#!/usr/bin/python3
import json, os, pathlib, shutil, subprocess, sys
name = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
base = pathlib.Path(os.environ["CASE_DIR"])
with (base / "calls").open("a") as file:
    file.write(json.dumps([name] + args) + "\n")
def flag(key):
    return (base / key).exists()
def enable(key):
    (base / key).touch()
if name == "uname":
    print("Linux" if args == ["-s"] else os.environ.get("ARCH", "x86_64"))
elif name == "id":
    print(os.environ.get("FAKE_UID", "0"))
elif name == "sudo":
    if args != ["-v"]:
        os.execvp(args[0], args)
elif name in ("apt-get", "dnf"):
    if os.environ.get("PACKAGE_FAIL"):
        sys.exit(1)
    for command in ("python3", "curl"):
        if command in args and not (base / "bin" / command).exists():
            (base / "bin" / command).symlink_to(os.environ["REAL_PYTHON"] if command == "python3" else base / "stub")
elif name == "getent":
    if args[0] == "passwd" and flag("user"):
        print("porter:x:999:999::" + str(base / "data") + ":/usr/sbin/nologin")
    elif args[0] == "group" and flag("group"):
        print("porter:x:999:")
    else:
        sys.exit(2)
elif name == "groupadd":
    enable("group")
elif name == "useradd":
    if os.environ.get("USERADD_FAIL"):
        sys.exit(1)
    enable("user")
elif name == "install":
    failure = os.environ.get("INSTALL_FAIL")
    if failure and args[-1].endswith((failure, failure + ".new")):
        sys.exit(1)
    partial = os.environ.get("PARTIAL_INSTALL_FAIL")
    if partial and args[-1].endswith((partial, partial + ".new")):
        pathlib.Path(args[-1]).write_text("不完整文件")
        sys.exit(1)
    filtered = []
    index = 0
    while index < len(args):
        if args[index] in ("-o", "-g"):
            index += 2
        else:
            filtered.append(args[index])
            index += 1
    sys.exit(subprocess.run(["/usr/bin/install", *filtered]).returncode)
elif name == "sleep":
    pass
elif name == "curl":
    url = next(arg for arg in args if arg.startswith("https://") or arg.startswith("http://"))
    if "-o" in args:
        if os.environ.get("DOWNLOAD_FAIL") or ("api.github.com" in url and os.environ.get("RELEASE_404")):
            sys.exit(22)
        target = pathlib.Path(args[args.index("-o") + 1])
        filename = "release.json" if "api.github.com" in url else url.rsplit("/", 1)[1]
        source = base / "fixtures" / filename
        if not source.exists():
            sys.exit(22)
        shutil.copyfile(source, target)
        if filename == "SHA256SUMS" and os.environ.get("BAD_CHECKSUM"):
            target.write_text("0" * 64 + "  porter-v1.2.3-linux-amd64.tar.gz\n")
        if filename == "SHA256SUMS" and os.environ.get("DUPLICATE_SUM"):
            target.write_bytes(target.read_bytes() * 2)
    elif os.environ.get("HEALTH_FAIL"):
        sys.exit(1)
    else:
        if os.environ.get("EXIT_DURING_HEALTH"):
            enable("exited")
        print("unexpected" if os.environ.get("WRONG_HEALTH_BODY") else '{"status":"ok"}')
elif name == "systemctl":
    if args[0] == "show":
        if os.environ.get("VENDOR_UNIT"):
            print("/usr/lib/systemd/system/porter.service")
        elif (base / "etc/systemd/system/porter.service").exists():
            print(base / "etc/systemd/system/porter.service")
    elif args[0] == "enable":
        if os.environ.get("START_FAIL"):
            sys.exit(1)
        enable("enabled")
        (base / "data/porter.db").touch()
    elif args[0] == "is-active":
        sys.exit(1 if os.environ.get("ACTIVE_FAIL") or flag("exited") else 0)
    elif args[0] == "is-enabled":
        sys.exit(1 if os.environ.get("ENABLED_FAIL") or not flag("enabled") else 0)
    elif args[0] == "daemon-reload" and os.environ.get("RELOAD_FAIL"):
        sys.exit(1)
else:
    raise AssertionError(name)
'''


class InstallCase(unittest.TestCase):
    def setUp(self):
        (ROOT / ".tmp").mkdir(exist_ok=True)
        self.temp = tempfile.TemporaryDirectory(prefix="native-install-test-", dir=ROOT / ".tmp")
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.bin = self.base / "bin"
        self.bin.mkdir()
        self.etc = self.base / "etc"
        for path in ("systemd/system", "ssl/certs"):
            (self.etc / path).mkdir(parents=True, exist_ok=True)
        (self.etc / "ssl/certs/ca-certificates.crt").write_text("测试 CA")
        (self.base / "proc1").write_text("systemd\n")
        (self.base / "run").mkdir()
        (self.base / "stub").write_text(STUB)
        (self.base / "stub").chmod(0o755)
        for command in ("uname", "id", "sudo", "apt-get", "dnf", "systemctl", "sleep", "curl", "getent", "useradd", "groupadd", "install"):
            (self.bin / command).symlink_to(self.base / "stub")
        for command in ("env", "cat", "mktemp", "tee", "chmod", "mv", "rm", "rmdir", "mkdir", "realpath", "python3", "stty", "cmp", "cut", "test"):
            (self.bin / command).symlink_to(shutil.which(command))
        self.directory = self.base / "program with spaces"
        self.config = self.etc / "porter/porter.env"
        self.state = self.etc / "porter/install.json"
        self.unit = self.etc / "systemd/system/porter.service"
        self.env = dict(os.environ, PATH=str(self.bin), CASE_DIR=str(self.base), REAL_PYTHON=REAL_PYTHON)
        self.system("debian", "13")
        self.fixtures()

    def system(self, name, version):
        (self.etc / "os-release").write_text(f'ID={name}\nVERSION_ID="{version}"\n')

    def fixtures(self, kind="normal", version="v1.2.3"):
        folder = self.base / "fixtures"
        folder.mkdir(exist_ok=True)
        sums = []
        assets = []
        for arch in ("amd64", "arm64"):
            name = f"porter-{version}-linux-{arch}.tar.gz"
            program = f'#!/bin/sh\nif [ "$1" = --version ]; then echo {version}; exit 0; fi\nexit 0\n'.encode()
            with tarfile.open(folder / name, "w:gz") as bundle:
                info = tarfile.TarInfo("../porter" if kind == "traversal" else "porter")
                if kind == "symlink":
                    info.type = tarfile.SYMTYPE
                    info.linkname = "/etc/passwd"
                else:
                    info.size = len(program)
                info.mode = 0o755
                bundle.addfile(info, io.BytesIO(program) if info.isfile() else None)
                if kind == "duplicate":
                    bundle.addfile(info, io.BytesIO(program))
            sums.append(hashlib.sha256((folder / name).read_bytes()).hexdigest() + "  " + name)
            assets.append({"name": name})
        (folder / "SHA256SUMS").write_text("\n".join(sums) + "\n")
        assets.append({"name": "SHA256SUMS"})
        (folder / "release.json").write_text(json.dumps({"tag_name": version, "draft": False, "prerelease": False, "assets": assets}))

    def calls(self):
        path = self.base / "calls"
        return [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []

    def actions(self, command):
        return [call[1:] for call in self.calls() if call[0] == command]

    def dialogue(self, username="admin", password="test-secret", listen="0.0.0.0:18080"):
        return [("管理员用户名：", username), ("管理员密码：", password), ("确认密码：", password), ("Web 监听地址", listen)]
    def run_install(self, dialogue=None, args=(), terminal=True):
        # 仅改写测试副本中的宿主路径，生产脚本不提供绕过系统检测的环境变量。
        script = SCRIPT.replace("/etc/", str(self.etc) + "/").replace("/var/lib/porter", str(self.base / "data")).replace("/proc/1/comm", str(self.base / "proc1")).replace("/run/systemd/system", str(self.base / "run"))
        argv = ["/bin/bash", "-s", "--", "--install-dir", str(self.directory), *args]
        if not terminal:
            result = subprocess.run(argv, input=script, text=True, capture_output=True,
                                    env=self.env, start_new_session=True, timeout=10)
            return result.returncode, result.stdout + result.stderr
        master, slave = pty.openpty()

        def controlling_terminal():
            os.setsid()
            fcntl.ioctl(slave, termios.TIOCSCTTY, 0)

        process = subprocess.Popen(argv, stdin=subprocess.PIPE, stdout=slave, stderr=slave,
                                   env=self.env, preexec_fn=controlling_terminal)
        os.close(slave)
        try:
            process.stdin.write(script.encode())
            process.stdin.close()
            output = b""
            pending = list(self.dialogue() if dialogue is None else dialogue)
            offset = 0
            deadline = time.monotonic() + 12
            while time.monotonic() < deadline:
                if select.select([master], [], [], 0.05)[0]:
                    try:
                        chunk = os.read(master, 65536)
                    except OSError:
                        break
                    if not chunk:
                        break
                    output += chunk
                    if pending:
                        prompt, answer = pending[0]
                        found = output.find(prompt.encode(), offset)
                        if found >= 0:
                            offset = found + len(prompt.encode())
                            pending.pop(0)
                            os.write(master, answer.encode() + b"\n")
                if process.poll() is not None:
                    # 仍读取终端中残留的输出。
                    if not select.select([master], [], [], 0)[0]:
                        break
            else:
                process.kill()
                self.fail("交互超时：" + output.decode(errors="replace"))
            code = process.wait(timeout=2)
            self.terminal_echo_restored = bool(termios.tcgetattr(master)[3] & termios.ECHO)
            return code, output.decode(errors="replace")
        finally:
            if process.poll() is None:
                process.kill()
                process.wait()
            os.close(master)

    def assert_success(self, *args, **kwargs):
        code, output = self.run_install(*args, **kwargs)
        self.assertEqual(code, 0, output)
        self.assertIn("Porter 已就绪", output)
        return output

    def environment(self):
        helper = SCRIPT.split("PY_HELPER='", 1)[1].split("\n'\npy()", 1)[0]
        namespace = {"__name__": "native_install_helper"}
        # 仅加载函数定义；不执行生产辅助器的命令入口。
        exec(helper.split("\ntry:\n", 1)[0], namespace)
        return namespace["read_environment"](self.config.read_text())

    def test_pipe_interaction_environment_and_unit(self):
        password = "  密码$${HOME}'\"\\`#尾  "
        output = self.assert_success(self.dialogue("  admin$name  ", password, "[::]:18080"))
        self.assertNotIn(password, output)
        self.assertNotIn("sudo systemctl", output)
        self.assertNotIn(password, (self.base / "calls").read_text())
        self.assertTrue(self.terminal_echo_restored)
        self.assertEqual(self.config.stat().st_mode & 0o777, 0o600)
        self.assertEqual(self.environment()["PORTER_ADMIN_PASSWORD"], password)
        self.assertEqual(self.environment()["PORTER_ADMIN_USERNAME"], "admin$name")
        text = self.unit.read_text()
        for required in ("User=porter", "Group=porter", "AmbientCapabilities=CAP_NET_BIND_SERVICE", "CapabilityBoundingSet=CAP_NET_BIND_SERVICE", "Restart=on-failure", "WantedBy=multi-user.target"):
            self.assertIn(required, text)
        self.assertTrue(any("http://[::1]:18080/healthz" in call for call in self.actions("curl")))
        self.assertFalse(any(call[0] == "docker" for call in self.calls()))
        analyze = shutil.which("systemd-analyze")
        self.assertIsNotNone(analyze, "验证环境需提供 systemd-analyze")
        result = subprocess.run([analyze, "verify", "--man=no", str(self.unit)], capture_output=True, text=True, timeout=15)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stderr, "")

    def test_unit_paths_preserve_special_characters(self):
        self.directory = self.base / 'program $d%q with spaces'
        self.assert_success()
        result = subprocess.run([shutil.which("systemd-analyze"), "verify", "--man=no", str(self.unit)],
                                capture_output=True, text=True, timeout=15)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stderr, "")

    def test_supported_systems_and_architectures(self):
        systems = (("ubuntu", "22.04"), ("ubuntu", "24.04"), ("debian", "12"), ("debian", "13"),
                   ("rhel", "9.4"), ("rhel", "10.0"), ("rocky", "9"), ("rocky", "10"), ("almalinux", "9"), ("almalinux", "10"))
        for index, (name, version) in enumerate(systems):
            with self.subTest(system=name, version=version):
                self.system(name, version)
                self.env["ARCH"] = "aarch64" if index % 2 else "x86_64"
                self.assert_success(args=("--version", "v1.2.3"))
                expected = "arm64" if index % 2 else "amd64"
                self.assertTrue(any(f"porter-v1.2.3-linux-{expected}.tar.gz" in str(call) for call in self.actions("curl")))
                self.assertTrue(any("/releases/tags/v1.2.3" in str(call) for call in self.actions("curl")))
                self.assertFalse(self.actions("apt-get") or self.actions("dnf"))
                self.reset_installation()

    def reset_installation(self):
        for path in (self.directory, self.etc / "porter", self.base / "data"):
            if path.exists():
                shutil.rmtree(path)
        for path in (self.unit, self.base / "calls", self.base / "user", self.base / "group", self.base / "enabled", self.base / "exited"):
            path.unlink(missing_ok=True)

    def test_repeat_keeps_binary_credentials_and_data_without_network(self):
        self.assert_success()
        with self.config.open("a") as file:
            file.write('PORTER_COOKIE_SECURE="true"\n')
        before = self.config.read_bytes()
        binary = self.directory / "porter"
        original_binary = binary.read_bytes()
        database = self.base / "data/porter.db"
        database.write_text("保留原有数据")
        (self.base / "calls").unlink()
        self.env["DOWNLOAD_FAIL"] = "1"
        code, output = self.run_install(terminal=False)
        self.assertEqual(code, 0, output)
        self.assertEqual(self.config.read_bytes(), before)
        self.assertEqual(binary.read_bytes(), original_binary)
        self.assertEqual(database.read_text(), "保留原有数据")
        self.assertFalse(any("-o" in call for call in self.actions("curl")))
        self.assertFalse(self.actions("useradd"))

    def test_partial_installation_retries_preserving_credentials(self):
        for failure in ("USERADD_FAIL", "START_FAIL", "RELOAD_FAIL", "HEALTH_FAIL"):
            with self.subTest(failure=failure):
                self.env[failure] = "1"
                code, output = self.run_install()
                self.assertNotEqual(code, 0, output)
                self.assertNotIn("Porter 已就绪", output)
                self.assertEqual(json.loads(self.state.read_text())["status"], "initializing")
                saved = self.config.read_bytes()
                del self.env[failure]
                code, output = self.run_install(terminal=False)
                self.assertEqual(code, 0, output)
                self.assertEqual(self.config.read_bytes(), saved)
                self.assertEqual(json.loads(self.state.read_text())["status"], "complete")
                self.assertFalse((self.etc / "porter/.install-lock").exists())
                self.reset_installation()

    def test_download_checksum_and_archive_fail_before_account_or_service(self):
        for failure in ("RELEASE_404", "DOWNLOAD_FAIL", "BAD_CHECKSUM", "DUPLICATE_SUM"):
            with self.subTest(failure=failure):
                self.env[failure] = "1"
                code, output = self.run_install()
                self.assertNotEqual(code, 0, output)
                self.assertFalse(self.actions("useradd"))
                self.assertFalse(self.config.exists() or self.unit.exists() or self.state.exists())
                self.assertFalse(any("enable" in call for call in self.actions("systemctl")))
                self.assertFalse((self.directory / ".install-lock").exists())
                self.assertFalse(list((self.directory / ".tmp").glob("install.*")))
                del self.env[failure]
                self.reset_installation()
        for kind in ("symlink", "traversal", "duplicate"):
            with self.subTest(archive=kind):
                self.fixtures(kind)
                code, output = self.run_install()
                self.assertNotEqual(code, 0, output)
                self.assertFalse(self.config.exists() or self.state.exists())
                self.reset_installation()

    def test_partial_configuration_and_unit_writes_can_retry(self):
        for name, target in (("porter.env", self.config), ("porter.service", self.unit)):
            with self.subTest(target=name):
                self.env["PARTIAL_INSTALL_FAIL"] = name
                code, output = self.run_install()
                self.assertNotEqual(code, 0, output)
                self.assertNotIn("Porter 已就绪", output)
                self.assertFalse(target.exists())
                self.assertEqual(json.loads(self.state.read_text())["status"], "initializing")
                saved = self.config.read_bytes() if self.config.exists() else None
                del self.env["PARTIAL_INSTALL_FAIL"]
                code, output = self.run_install(terminal=saved is None)
                self.assertEqual(code, 0, output)
                if saved is not None:
                    self.assertEqual(self.config.read_bytes(), saved)
                self.assertTrue(target.is_file())
                self.assertFalse(Path(str(target) + ".new").exists())
                self.assertEqual(json.loads(self.state.read_text())["status"], "complete")
                self.reset_installation()

    def test_missing_release_assets_and_latest_prerelease_fail(self):
        path = self.base / "fixtures/release.json"
        original = path.read_text()
        for change in ("assets", "prerelease", "draft"):
            with self.subTest(change=change):
                release = json.loads(original)
                release[change] = [] if change == "assets" else True
                path.write_text(json.dumps(release))
                code, output = self.run_install()
                self.assertNotEqual(code, 0, output)
                self.assertFalse(self.state.exists())
                self.reset_installation()

    def test_unknown_existing_files_accounts_and_vendor_units_are_preserved(self):
        for existing in ("config", "unit", "binary", "data", "user", "group", "vendor"):
            with self.subTest(existing=existing):
                if existing == "config":
                    self.config.parent.mkdir()
                    self.config.write_text("未知凭据")
                elif existing == "unit":
                    self.unit.write_text("未知服务")
                elif existing == "binary":
                    self.directory.mkdir()
                    (self.directory / "porter").write_text("未知程序")
                elif existing == "data":
                    (self.base / "data").mkdir()
                elif existing == "vendor":
                    self.env["VENDOR_UNIT"] = "1"
                else:
                    (self.base / existing).touch()
                code, output = self.run_install(terminal=False)
                self.assertNotEqual(code, 0, output)
                self.assertFalse(self.actions("useradd"))
                self.assertFalse(any("enable" in call for call in self.actions("systemctl")))
                self.assertFalse(self.state.exists())
                self.env.pop("VENDOR_UNIT", None)
                self.reset_installation()

    def test_no_terminal_and_unsupported_hosts_do_not_install(self):
        (self.bin / "python3").unlink()
        code, output = self.run_install(terminal=False)
        self.assertNotEqual(code, 0, output)
        self.assertIn("需要可交互终端", output)
        self.assertFalse(self.actions("apt-get") or self.directory.exists())
        (self.bin / "python3").symlink_to(REAL_PYTHON)
        for name, version, arch, systemd in (("fedora", "42", "x86_64", True), ("rhel", "91", "x86_64", True),
                                            ("debian", "13", "riscv64", True), ("ubuntu", "24.04", "x86_64", False)):
            with self.subTest(name=name, arch=arch, systemd=systemd):
                self.system(name, version)
                self.env["ARCH"] = arch
                (self.base / "proc1").write_text("systemd" if systemd else "codex")
                code, output = self.run_install(terminal=False)
                self.assertNotEqual(code, 0, output)
                self.assertFalse(self.directory.exists())

    def test_invalid_inputs_retry_and_preserve_terminal(self):
        dialogue = [("管理员用户名：", " "), ("管理员用户名：", "user"),
                    ("管理员密码：", "  "), ("确认密码：", "  "),
                    ("管理员密码：", "one"), ("确认密码：", "two"),
                    ("管理员密码：", "final-secret"), ("确认密码：", "final-secret")]
        for address in ("localhost:80", "127.0.0.1:0", "127.0.0.1:65536", "::1:80", "[fe80::1%eth0]:80", "[::]:8080"):
            dialogue.append(("Web 监听地址", address))
        output = self.assert_success(dialogue)
        self.assertNotIn("final-secret", output)
        self.assertIn("两次密码不一致", output)
        self.assertTrue(self.terminal_echo_restored)

    def test_password_eof_or_interrupt_restores_terminal(self):
        for answer in ("\x04", "\x03"):
            with self.subTest(answer=repr(answer)):
                code, output = self.run_install([("管理员用户名：", "admin"), ("管理员密码：", answer)])
                self.assertNotEqual(code, 0, output)
                self.assertTrue(self.terminal_echo_restored)
                self.assertFalse(self.state.exists() or self.actions("useradd"))

    def test_system_dependencies_and_sudo_pipeline(self):
        for name, version, manager in (("debian", "13", "apt-get"), ("rhel", "10", "dnf")):
            with self.subTest(name=name):
                self.system(name, version)
                (self.bin / "python3").unlink()
                self.env["FAKE_UID"] = "1000"
                output = self.assert_success()
                self.assertIn("sudo systemctl", output)
                self.assertTrue(self.actions("sudo"))
                installs = [call for call in self.actions(manager) if "install" in call]
                self.assertTrue(any("python3" in call and "ca-certificates" in call for call in installs))
                self.assertFalse(any("docker" in str(call) for call in installs))
                self.reset_installation()

    def test_permission_start_and_health_failures_do_not_report_success(self):
        for failure, value in (("INSTALL_FAIL", "porter.env"), ("ACTIVE_FAIL", "1"), ("ENABLED_FAIL", "1"), ("WRONG_HEALTH_BODY", "1"), ("EXIT_DURING_HEALTH", "1")):
            with self.subTest(failure=failure):
                self.env[failure] = value
                code, output = self.run_install()
                self.assertNotEqual(code, 0, output)
                self.assertNotIn("Porter 已就绪", output)
                self.assertFalse((self.directory / ".install-lock").exists())
                self.assertFalse((self.etc / "porter/.install-lock").exists())
                del self.env[failure]
                self.reset_installation()

    def test_invalid_arguments_fail_before_writing(self):
        for args in (("--image", "unused"), ("--version", "bad/version"), ("--version",),
                     ("--install-dir", "/opt/.."), ("--install-dir", str(self.base / "bad'path")),
                     ("--install-dir", str(self.base / "data/program")),
                     ("--install-dir", str(self.etc / "porter/program"))):
            with self.subTest(args=args):
                code, output = self.run_install(args=args, terminal=False)
                self.assertNotEqual(code, 0, output)
                self.assertFalse(self.state.exists() or self.actions("useradd") or self.directory.exists())

    def test_complete_installation_and_version_override_are_protected(self):
        self.assert_success()
        saved = self.config.read_bytes()
        code, output = self.run_install(args=("--version", "v2.0.0"), terminal=False)
        self.assertNotEqual(code, 0, output)
        self.assertEqual(self.config.read_bytes(), saved)
        self.config.unlink()
        code, output = self.run_install(terminal=False)
        self.assertNotEqual(code, 0, output)
        self.assertFalse(self.config.exists())

    def test_complete_installation_missing_data_is_not_reinitialized(self):
        for missing in ("directory", "database"):
            with self.subTest(missing=missing):
                self.assert_success()
                saved = self.config.read_bytes()
                if missing == "directory":
                    shutil.rmtree(self.base / "data")
                else:
                    (self.base / "data/porter.db").unlink()
                (self.base / "calls").unlink()
                code, output = self.run_install(terminal=False)
                self.assertNotEqual(code, 0, output)
                self.assertIn("数据库缺失", output)
                self.assertFalse((self.base / "data/porter.db").exists())
                self.assertEqual(self.config.read_bytes(), saved)
                self.assertFalse(any("enable" in call for call in self.actions("systemctl")))
                self.reset_installation()

    def test_existing_installation_locks_block_without_removing_owner_lock(self):
        for directory in (self.directory, self.config.parent):
            with self.subTest(directory=directory.name):
                directory.mkdir(parents=True)
                lock = directory / ".install-lock"
                lock.mkdir()
                code, output = self.run_install()
                self.assertNotEqual(code, 0, output)
                self.assertTrue(lock.is_dir())
                self.assertFalse(self.state.exists() or self.config.exists() or self.unit.exists())
                self.assertFalse(self.actions("useradd"))
                self.assertFalse(any("enable" in call for call in self.actions("systemctl")))
                self.reset_installation()


if __name__ == "__main__":
    unittest.main(verbosity=2)
