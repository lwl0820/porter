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
import sqlite3
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
import json, os, pathlib, shutil, subprocess, sys, signal, sqlite3
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
        print("porter:x:999:999::" + str(base / "data") + (":/bin/bash" if os.environ.get("FOREIGN_ACCOUNT") else ":/usr/sbin/nologin"))
        if len(args) == 1 and os.environ.get("SHARED_GROUP"):
            print("other:x:1000:999::/home/other:/bin/bash")
    elif args == ["passwd"]:
        print("root:x:0:0::/root:/bin/bash")
    elif args[0] == "group" and flag("group"):
        print("porter:x:999:")
    else:
        sys.exit(2)
elif name == "userdel":
    if os.environ.get("USERDEL_FAIL"):
        sys.exit(1)
    (base / "user").unlink(missing_ok=True)
elif name == "groupdel":
    if os.environ.get("GROUPDEL_FAIL"):
        sys.exit(1)
    (base / "group").unlink(missing_ok=True)
elif name == "rm":
    if os.environ.get("REMOVE_FAIL") and any(arg.endswith(os.environ["REMOVE_FAIL"]) for arg in args):
        sys.exit(1)
    if os.environ.get("CACHE_FRAGMENT") and any(arg.endswith("porter.service") for arg in args):
        enable("cached_fragment")
    code = subprocess.run(["/usr/bin/rm", *args]).returncode
    if os.environ.get("KILL_UNINSTALL") == "unit" and any(arg.endswith("porter.service") for arg in args):
        os.kill(os.getppid(), signal.SIGKILL); os.kill(os.getpid(), signal.SIGKILL)
    sys.exit(code)
elif name == "ln":
    if os.environ.get("LINK_RACE"):
        pathlib.Path(args[-1]).write_text("racing unknown file")
    sys.exit(subprocess.run(["/usr/bin/ln", *args]).returncode)
elif name == "groupadd":
    enable("group")
elif name == "useradd":
    if os.environ.get("USERADD_FAIL"):
        sys.exit(1)
    enable("user")
elif name == "install":
    race = os.environ.get("MANAGED_STATE_RACE")
    state = base / "etc/porter/install.json"
    if race and "-d" in args and args[-1] == str(base / "program with spaces") and not flag("state_race_done"):
        # 模拟另一进程在前置校验后、目录加锁前完成卸载或改写记录。
        enable("state_race_done")
        record = json.loads(state.read_text())
        if race in ("uninstalled", "purged"):
            (base / "program with spaces/porter").unlink()
            (base / "program with spaces/porterd").unlink(missing_ok=True)
            (base / "usr/bin/porter").unlink(missing_ok=True)
            (base / "etc/systemd/system/porter.service").unlink()
            (base / "enabled").unlink()
            enable("stopped")
        if race == "purged":
            state.unlink()
            (base / "etc/porter/porter.env").unlink()
            shutil.rmtree(base / "data")
            (base / "user").unlink()
            (base / "group").unlink()
        else:
            if race == "uninstalled":
                record["status"] = "uninstalled"
            elif race == "version":
                record["version"] = "v1.2.4"
            elif race == "arch":
                record["arch"] = "arm64"
            elif race == "directory":
                record["directory"] = str(base / "other-program")
            state.write_text(json.dumps(record))
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
    elif os.environ.get("HEALTH_FAIL") or os.environ.get("UPGRADE_HEALTH_FAIL_ONCE"):
        if os.environ.get("UPGRADE_HEALTH_FAIL_ONCE"):
            (base / "data/new-sidecar").write_bytes(b"new version sidecar")
            (base / "data/porter.db").write_bytes(b"new migrated db")
        sys.exit(1)
    else:
        if os.environ.get("EXIT_DURING_HEALTH"):
            enable("exited")
        print("unexpected" if os.environ.get("WRONG_HEALTH_BODY") else (os.environ.get("CONTROL_RULES", '{"rules":[]}') if "--unix-socket" in args else '{"status":"ok"}'))
elif name == "systemctl":
    if args[0] == "show":
        if "--property=DropInPaths" in args:
            if os.environ.get("DROP_IN_QUERY_FAIL"):
                sys.exit(1)
            print("/etc/systemd/system/porter.service.d/custom.conf" if os.environ.get("DROP_IN") else "")
        elif os.environ.get("VENDOR_UNIT"):
            print("/usr/lib/systemd/system/porter.service")
        elif (base / "etc/systemd/system/porter.service").exists() or flag("cached_fragment"):
            print(base / "etc/systemd/system/porter.service")
    elif args[0] in ("enable", "start"):
        if os.environ.get("START_FAIL"):
            sys.exit(1)
        if args[0] == "enable":
            enable("enabled")
        enable("active")
        (base / "stopped").unlink(missing_ok=True)
        database = base / "data/porter.db"
        if not database.exists():
            connection = sqlite3.connect(database)
            connection.execute("CREATE TABLE rules (enabled INTEGER, protocol TEXT, listen_host TEXT, listen_port INTEGER)")
            connection.commit(); connection.close()
    elif args[0] == "stop":
        if os.environ.get("STOP_FAIL"):
            sys.exit(1)
        if os.environ.get("INSERT_RULE_ON_STOP") and not flag("stop_rule_inserted"):
            connection = sqlite3.connect(base / "data/porter.db")
            connection.execute("INSERT INTO rules VALUES (1, \"tcp+udp\", \"127.0.0.1\", 18080)")
            connection.commit(); connection.close()
            enable("stop_rule_inserted")
        if not os.environ.get("STILL_ACTIVE"):
            enable("stopped")
            (base / "active").unlink(missing_ok=True)
        if os.environ.get("KILL_UNINSTALL") == "stop":
            os.kill(os.getppid(), signal.SIGKILL); os.kill(os.getpid(), signal.SIGKILL)
    elif args[0] == "disable":
        if os.environ.get("DISABLE_FAIL"):
            sys.exit(1)
        (base / "enabled").unlink(missing_ok=True)
    elif args[0] == "is-active":
        sys.exit(3 if os.environ.get("ACTIVE_FAIL") or flag("exited") or flag("stopped") or (not flag("active") and not os.environ.get("STILL_ACTIVE")) else 0)
    elif args[0] == "is-enabled":
        sys.exit(1 if os.environ.get("ENABLED_FAIL") or not flag("enabled") else 0)
    elif args[0] == "daemon-reload":
        if os.environ.get("RELOAD_FAIL"):
            sys.exit(1)
        (base / "cached_fragment").unlink(missing_ok=True)
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
        for command in ("uname", "id", "sudo", "apt-get", "dnf", "systemctl", "sleep", "curl", "getent", "useradd", "groupadd", "userdel", "groupdel", "rm", "install", "ln"):
            (self.bin / command).symlink_to(self.base / "stub")
        for command in ("env", "cat", "mktemp", "tee", "chmod", "mv", "rmdir", "mkdir", "realpath", "python3", "stty", "cmp", "cut", "test"):
            (self.bin / command).symlink_to(shutil.which(command))
        self.command = self.base / "usr/bin/porter"
        self.command.parent.mkdir(parents=True)
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
            help_redirect = " >&2" if kind == "help-stderr" else ""
            program = f'#!/bin/sh\nif [ "$1" = --version ]; then echo {version}; exit 0; fi\nif [ "$1" = --help ]; then echo --check-config{help_redirect}; exit 0; fi\nif [ "$1" = --check-config ]; then [ -z "$PREFLIGHT_FAIL" ]; exit $?; fi\nexit 0\n'.encode()
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
                if kind not in ("legacy", "missing-server"):
                    server = tarfile.TarInfo("porterd")
                    server.size = len(program)
                    server.mode = 0o755
                    if kind == "server-link":
                        server.type = tarfile.SYMTYPE
                        server.linkname = "porter"
                    bundle.addfile(server, io.BytesIO(program) if server.isfile() else None)
                    if kind == "duplicate-server":
                        bundle.addfile(server, io.BytesIO(program))
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
    def run_install(self, dialogue=None, args=(), terminal=True, language="zh-CN", directory=True):
        # 仅改写测试副本中的宿主路径，生产脚本不提供绕过系统检测的环境变量。
        script = SCRIPT.replace("/usr/bin/porter", str(self.command)).replace("/etc/", str(self.etc) + "/").replace("/var/lib/porter", str(self.base / "data")).replace("/proc/1/comm", str(self.base / "proc1")).replace("/run/systemd/system", str(self.base / "run"))
        script = script.replace(".st_uid != 0", ".st_uid != os.getuid()")
        script = script.replace('while current != "/":', 'while current not in ("/", ' + json.dumps(str(self.base)) + '):')
        if self.env.get("KILL_PHASE"):
            script = script.replace('transaction["phase"] = phase', 'transaction["phase"] = phase').replace('atomic_json(os.path.join(folder, "transaction.json"), transaction)\n    def service_intent', 'atomic_json(os.path.join(folder, "transaction.json"), transaction)\n        if phase == os.environ.get("KILL_PHASE"):\n            os.kill(os.getppid(), signal.SIGKILL)\n            os.kill(os.getpid(), signal.SIGKILL)\n    def service_intent')
        if self.env.get("FAULT_BOUNDARY"):
            script = script.replace("    def mark(phase):", "    fault_seen = False\n    def mark(phase):")
            script = script.replace("        os.replace(temporary, target)", "        os.replace(temporary, target)\n        nonlocal fault_seen\n        boundary = os.environ.get(\"FAULT_BOUNDARY\")\n        if not fault_seen and ((boundary == \"porter\" and target.endswith(\"/porter\")) or (boundary == \"porterd\" and target.endswith(\"/porterd\")) or (boundary == \"unit\" and target.endswith(\"/porter.service\"))):\n            fault_seen = True\n            raise OSError(\"injected replacement failure\")")
            script = script.replace('copy_data(data, os.path.join(folder, "data"))', 'copy_data(data, os.path.join(folder, "data"))\n        if os.environ.get("FAULT_BOUNDARY") == "backup":\n            raise OSError("injected backup failure")')
            script = script.replace("        atomic_json(statepath, new)", '        if os.environ.get("FAULT_BOUNDARY") == "state":\n            raise OSError("injected state failure")\n        atomic_json(statepath, new)')
        if self.env.get("SIGNAL_PHASE"):
            script = script.replace('atomic_json(os.path.join(folder, "transaction.json"), transaction)\n    def service_intent', 'atomic_json(os.path.join(folder, "transaction.json"), transaction)\n        if phase == os.environ.get("SIGNAL_PHASE"):\n            os.kill(os.getppid(), signal.SIGTERM)\n            time.sleep(1)\n    def service_intent')
        if self.env.get("MOUNT_FIXTURE"):
            script = script.replace('"/proc/self/mountinfo"', json.dumps(str(self.base / "mountinfo")))
            if self.env.get("MOUNT_AFTER_LOCK"):
                insertion = 'json.dump({"pid": int(pid), "start": start}, file)\n        if path == ' + json.dumps(str(self.config.parent / ".install-lock")) + ":\n            with open(" + json.dumps(str(self.base / "mountinfo")) + ", \"a\") as mounted:\n                mounted.write(" + json.dumps(self.env["MOUNT_AFTER_LOCK"]) + ")"
                script = script.replace('json.dump({"pid": int(pid), "start": start}, file)', insertion)
        if self.env.get("COMPONENT_CUT"):
            checkpoint = "    def checkpoint(point):\n        if mode == \"reconfigure\" and point == os.environ.get(\"COMPONENT_CUT\"):\n            if os.environ.get(\"COMPONENT_CUT_KIND\") == \"kill\":\n                os.kill(os.getppid(), signal.SIGKILL)\n                os.kill(os.getpid(), signal.SIGKILL)\n            raise OSError(\"injected component boundary failure\")\n"
            script = script.replace("    def mark(phase):", checkpoint + "    def mark(phase):")
            boundaries = {
                '    atomic_json(statepath, state)\n    def interrupted': '    atomic_json(statepath, state)\n    checkpoint("prepared")\n    def interrupted',
                '        ctl("stop", "porter.service"); inactive(); mark("stopped")': '        ctl("stop", "porter.service"); inactive(); checkpoint("stopped"); mark("stopped")',
                '        copy_owned(config, os.path.join(folder, "porter.env"))': '        checkpoint("backup-partial")\n        copy_owned(config, os.path.join(folder, "porter.env"))',
                '        mark("backup_complete")': '        mark("backup_complete"); checkpoint("backup-complete")',
                '        install_file(os.path.join(staging, "porter.env"), config, "0600")': '        install_file(os.path.join(staging, "porter.env"), config, "0600"); checkpoint("environment")',
                '                os.unlink(command)\n            regular(os.path.join(directory, "porter"))': '                os.unlink(command); checkpoint("link-remove")\n            regular(os.path.join(directory, "porter"))',
                '            regular(os.path.join(directory, "porter")); os.unlink(os.path.join(directory, "porter"))': '            regular(os.path.join(directory, "porter")); os.unlink(os.path.join(directory, "porter")); checkpoint("client-remove")',
                '            os.symlink(directory + "/porter", command)\n        ctl("daemon-reload")': '            os.symlink(directory + "/porter", command); checkpoint("link-create")\n        ctl("daemon-reload")',
                '        service_intent()\n        new = ': '        checkpoint("ready"); service_intent()\n        new = ',
                '        atomic_json(statepath, new)': '        checkpoint("state"); atomic_json(statepath, new)',
                '            mark("restored")': '            mark("restored"); checkpoint("restored")',
            }
            for original, replacement in boundaries.items():
                self.assertIn(original, script)
                script = script.replace(original, replacement)
        argv = ["/bin/bash", "-s", "--"]
        if language is not None:
            argv.extend(("--lang", language))
        if directory:
            argv.extend(("--install-dir", str(self.directory)))
        argv.extend(args)
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
            english = language == "en" or (language is None and (not pending or pending[0][1] != "2"))
            web_prompt = "Enable Web service?" if english else "是否启用 Web 服务？"
            cli_prompt = "Install command-line tool?" if english else "是否安装命令行工具？"
            change_prompt = "Change management components?" if english else "是否修改管理组件？"
            if "--uninstall" not in args and "--upgrade" not in args:
                if not self.config.exists() and not any(web_prompt in prompt for prompt, _ in pending):
                    index = 1 if pending and pending[0][0] == "Language" else 0
                    pending[index:index] = [(web_prompt, ""), (cli_prompt, "")]
                elif self.state.exists() and json.loads(self.state.read_text())["status"] == "complete" and not any(change_prompt in prompt for prompt, _ in pending):
                    pending.insert(0, (change_prompt, ""))
            offset = 0
            deadline = time.monotonic() + 25
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

    def uninstall(self, purge=False, answers=None, **kwargs):
        english = kwargs.get("language") == "en"
        prompts = [("Confirm uninstall?" if english else "确认卸载？", "yes")]
        if purge:
            prompts.append(("Type DELETE" if english else "输入 DELETE", "DELETE"))
        return self.run_install(dialogue=prompts if answers is None else answers,
                                args=("--uninstall", *( ("--purge",) if purge else () )), **kwargs)

    def snapshot(self):
        return {str(path.relative_to(self.base)): path.read_bytes()
                for folder in (self.config.parent, self.base / "data")
                for path in folder.rglob("*") if path.is_file() and ".install-lock" not in path.parts}


    def mode_dialogue(self, web, cli, change=False, credentials=False, confirm="yes", english=False):
        choices = [("Enable Web service?" if english else "是否启用 Web 服务？", "yes" if web else "no"),
                   ("Install command-line tool?" if english else "是否安装命令行工具？", "yes" if cli else "no")]
        if change:
            choices.insert(0, ("Change management components?" if english else "是否修改管理组件？", "yes"))
        if credentials or (web and not change):
            choices += [("Admin username:", "admin"), ("Admin password:", "test-secret"), ("Confirm password:", "test-secret"), ("Web listen address", "0.0.0.0:18080")] if english else self.dialogue()
        if change:
            choices.append(("Type yes to confirm" if english else "输入 yes 确认，", confirm))
        return choices

    def install_mode(self, web, cli, english=False):
        return self.run_install(self.mode_dialogue(web, cli, english=english), language="en" if english else "zh-CN")

    def reconfigure(self, web, cli, **kwargs):
        return self.run_install(self.mode_dialogue(web, cli, change=True, **kwargs))

    def assert_mode(self, web, cli):
        state = json.loads(self.state.read_text())
        self.assertEqual(state["schema"], 3)
        self.assertEqual(state["features"], {"web": web, "cli": cli})
        self.assertEqual(self.environment()["PORTER_WEB_ENABLED"], str(web).lower())
        self.assertEqual(self.environment()["PORTER_CLI_ENABLED"], str(cli).lower())
        self.assertTrue((self.directory / "porterd").exists())
        self.assertEqual((self.directory / "porter").exists(), cli)
        self.assertEqual(self.command.is_symlink(), cli and self.directory != self.command.parent)
        self.assertEqual("command_link" in state, cli and self.directory != self.command.parent)

    def test_three_component_install_modes_skip_unneeded_inputs_and_probes(self):
        for english in (False, True):
            for web, cli in ((True, True), (True, False), (False, True)):
                with self.subTest(english=english, web=web, cli=cli):
                    code, output = self.install_mode(web, cli, english)
                    self.assertEqual(code, 0, output)
                    self.assert_mode(web, cli)
                    calls = self.actions("curl")
                    self.assertEqual(any("/healthz" in str(call) for call in calls), web)
                    self.assertEqual(any("--unix-socket" in call for call in calls), cli)
                    self.assertFalse((self.config.parent / "cli.json").exists())
                    if not web:
                        self.assertNotIn("Admin username", output)
                        self.assertNotIn("管理员用户名", output)
                        self.assertNotIn("PORTER_ADMIN_USERNAME", self.environment())
                        self.assertNotIn("PORTER_ADMIN_PASSWORD", self.environment())
                        self.assertNotIn("PORTER_HTTP_ADDR", self.environment())
                    if not cli:
                        self.assertNotIn(" rules", output)
                    self.reset_installation()

    def test_all_disabled_reprompts_without_committing(self):
        dialogue = self.mode_dialogue(False, False) + self.mode_dialogue(False, True)
        code, output = self.run_install(dialogue)
        self.assertEqual(code, 0, output)
        self.assertIn("至少启用一种", output)
        self.assert_mode(False, True)
        self.reset_installation()
        code, output = self.run_install(self.mode_dialogue(False, False) + [("是否启用 Web 服务？", "\x04")])
        self.assertNotEqual(code, 0, output)
        self.assertFalse(self.state.exists() or self.config.exists() or self.unit.exists())
        self.assertFalse(any(call[0] in ("stop", "start", "enable") for call in self.actions("systemctl")))

    def test_all_six_component_switches_preserve_data_identity_preferences_and_version(self):
        modes = ((True, True), (True, False), (False, True))
        for source in modes:
            for target in modes:
                if source == target:
                    continue
                with self.subTest(source=source, target=target):
                    self.assertEqual(self.install_mode(*source)[0], 0)
                    database = self.base / "data/porter.db"
                    if source[0]:
                        database.write_bytes(b"same rule ID / owner / traffic / accounts / plans")
                    data_before = database.read_bytes()
                    preference = self.config.parent / "cli.json"
                    preference.write_text('{"schema":1,"language":"zh-CN"}')
                    preference.chmod(0o600)
                    config = self.environment()
                    (self.base / "calls").unlink()
                    code, output = self.reconfigure(*target, credentials=target[0] and not source[0])
                    self.assertEqual(code, 0, output)
                    self.assert_mode(*target)
                    self.assertEqual(database.read_bytes(), data_before)
                    self.assertEqual(preference.read_text(), '{"schema":1,"language":"zh-CN"}')
                    self.assertEqual(json.loads(self.state.read_text())["version"], "v1.2.3")
                    downloads = [str(call) for call in self.actions("curl") if "-o" in call]
                    self.assertFalse(any("latest" in call for call in downloads))
                    self.assertEqual(bool(downloads), target[1] and not source[1])
                    self.assertTrue(any(call[0] == "stop" for call in self.actions("systemctl")))
                    for key, value in config.items():
                        if key not in ("PORTER_WEB_ENABLED", "PORTER_CLI_ENABLED"):
                            self.assertEqual(self.environment()[key], value)
                    backup = next((self.directory / ".tmp").glob("upgrade.*"))
                    transaction = json.loads((backup / "transaction.json").read_text())
                    self.assertEqual(transaction["operation"], "reconfigure")
                    self.assertEqual(transaction["old_features"], {"web": source[0], "cli": source[1]})
                    self.assertEqual(transaction["target_features"], {"web": target[0], "cli": target[1]})
                    self.reset_installation()

    def test_candidate_preflight_accepts_help_on_stderr_and_still_checks_config(self):
        # Go flag的真实帮助写入stderr；不能把该程序误判为旧版本。
        for invalid in (True, False):
            self.fixtures("help-stderr")
            if invalid:
                self.env["PREFLIGHT_FAIL"] = "1"
            else:
                self.env.pop("PREFLIGHT_FAIL", None)
            code, output = self.install_mode(False, True)
            self.assertEqual(code, 1 if invalid else 0, output)
            if invalid:
                self.assertFalse(self.state.exists() or self.config.exists() or self.unit.exists())
                self.assertFalse(any(call[0] in ("stop", "start", "enable") for call in self.actions("systemctl")))
            else:
                self.assert_mode(False, True)
                code, output = self.reconfigure(True, True, credentials=True)
                self.assertEqual(code, 0, output)
                self.assert_mode(True, True)
                self.fixtures("help-stderr", version="v1.3.0")
                code, output = self.run_install([("输入 yes 确认升级", "yes")], args=("--upgrade",))
                self.assertEqual(code, 0, output)
                self.assertEqual(json.loads(self.state.read_text())["version"], "v1.3.0")
            self.reset_installation()

    def test_component_switch_cancel_preflight_and_noop_do_not_stop_or_commit(self):
        self.assertEqual(self.install_mode(True, True)[0], 0)
        saved = self.snapshot()
        for failure in ("cancel", "preflight", "noop"):
            (self.base / "calls").unlink()
            if failure == "preflight":
                self.env["PREFLIGHT_FAIL"] = "1"
            code, output = self.reconfigure(True, failure == "noop", confirm="" if failure == "cancel" else "yes")
            self.assertEqual(code, 1 if failure == "preflight" else 0, output)
            self.assertEqual(self.snapshot(), saved)
            self.assertFalse(any(call[0] in ("stop", "start", "enable", "disable") for call in self.actions("systemctl")))
            self.env.pop("PREFLIGHT_FAIL", None)

    def test_unchanged_rerun_keeps_stopped_disabled_service_and_saved_configuration(self):
        self.assert_success()
        state = self.state.read_bytes()
        config = self.config.read_bytes()
        (self.base / "stopped").touch()
        (self.base / "active").unlink()
        (self.base / "enabled").unlink()
        (self.base / "calls").unlink()
        code, output = self.run_install(terminal=False)
        self.assertEqual(code, 0, output)
        self.assertIn("服务保持停止状态", output)
        self.assertNotIn("已就绪", output)
        self.assertEqual(self.state.read_bytes(), state)
        self.assertEqual(self.config.read_bytes(), config)
        self.assertFalse((self.base / "active").exists() or (self.base / "enabled").exists())
        self.assertFalse(any(call[0] in ("enable", "start", "stop", "daemon-reload") for call in self.actions("systemctl")))

    def test_web_only_ignores_and_preserves_unrelated_system_command(self):
        self.command.write_text("foreign command")
        code, output = self.install_mode(True, False)
        self.assertEqual(code, 0, output)
        self.assertEqual(self.command.read_text(), "foreign command")
        (self.base / "calls").unlink()
        code, output = self.reconfigure(True, True)
        self.assertNotEqual(code, 0, output)
        self.assertEqual(self.command.read_text(), "foreign command")
        self.assertFalse(any(call[0] == "stop" for call in self.actions("systemctl")))
        code, output = self.uninstall(True)
        self.assertEqual(code, 0, output)
        self.assertEqual(self.command.read_text(), "foreign command")

    def test_modes_reinstall_upgrade_and_purge_preserve_selected_components(self):
        for mode in ((True, False), (False, True)):
            self.fixtures()
            self.assertEqual(self.install_mode(*mode)[0], 0)
            data = self.base / "data/porter.db"
            data.write_bytes(b"retained data")
            config = self.config.read_bytes()
            self.assertEqual(self.uninstall()[0], 0)
            self.assert_success(terminal=False)
            self.assert_mode(*mode)
            self.assertEqual(self.config.read_bytes(), config)
            self.assertEqual(data.read_bytes(), b"retained data")
            code, output = self.upgrade()
            self.assertEqual(code, 0, output)
            self.assert_mode(*mode)
            self.assertEqual(data.read_bytes(), b"retained data")
            code, output = self.uninstall(True)
            self.assertEqual(code, 0, output)
            self.assertFalse(self.config.exists() or self.state.exists() or data.exists() or self.command.exists())
            self.reset_installation()

    def test_schema3_invalid_features_and_environment_mismatch_refuse_changes(self):
        self.assert_success()
        original = self.state.read_bytes()
        for value in ({"web": False, "cli": False}, {"web": 1, "cli": True}, {"web": True}, {"web": True, "cli": True, "other": False}):
            state = json.loads(original)
            state["features"] = value
            self.state.write_text(json.dumps(state))
            code, output = self.run_install(terminal=False)
            self.assertNotEqual(code, 0, output)
        self.state.write_bytes(original)
        self.config.write_text(self.config.read_text().replace('PORTER_WEB_ENABLED="true"', 'PORTER_WEB_ENABLED="false"'))
        saved = self.snapshot()
        (self.base / "calls").unlink()
        for action in (lambda: self.run_install(terminal=False), lambda: self.uninstall(answers=[])):
            code, output = action()
            self.assertNotEqual(code, 0, output)
            self.assertEqual(self.snapshot(), saved)
        self.assertFalse(any(call[0] == "stop" for call in self.actions("systemctl")))

    def test_schema2_upgrade_retains_both_and_old_program_preflight_refuses_before_stop(self):
        self.assert_success()
        state = json.loads(self.state.read_text())
        state["schema"] = 2
        del state["features"]
        self.state.write_text(json.dumps(state))
        self.config.write_text("\n".join(line for line in self.config.read_text().splitlines() if "_ENABLED=" not in line) + "\n")
        self.assert_success(terminal=False)
        (self.base / "calls").unlink()
        code, output = self.reconfigure(False, True)
        self.assertNotEqual(code, 0, output)
        self.assertIn("请先使用 --upgrade", output)
        self.assertFalse(any(call[0] == "stop" for call in self.actions("systemctl")))
        code, output = self.upgrade()
        self.assertEqual(code, 0, output)
        self.assert_mode(True, True)
        self.reset_installation()
        self.fixtures()
        self.assert_success()
        self.fixtures(version="v1.3.0")
        self.env["PREFLIGHT_FAIL"] = "1"
        (self.base / "calls").unlink()
        code, output = self.run_install([], args=("--upgrade",))
        self.assertNotEqual(code, 0, output)
        self.assertFalse(any(call[0] == "stop" for call in self.actions("systemctl")))

    def test_enabling_web_rejects_overlapping_rules_online_and_stopped_without_database_side_effects(self):
        for stopped in (False, True):
            for host, protocol, enabled, reject in (("0.0.0.0", "tcp", True, True), ("127.0.0.1", "tcp+udp", True, True), ("::ffff:127.0.0.1", "tcp", True, True), ("127.0.0.1", "udp", True, False), ("::", "tcp", True, False), ("127.0.0.1", "tcp", False, False)):
                with self.subTest(stopped=stopped, host=host, protocol=protocol, enabled=enabled):
                    self.fixtures()
                    self.assertEqual(self.install_mode(False, True)[0], 0)
                    database = self.base / "data/porter.db"
                    connection = sqlite3.connect(database)
                    connection.execute("INSERT INTO rules VALUES (?, ?, ?, ?)", (enabled, protocol, host, 18080))
                    connection.commit(); connection.close()
                    self.env["CONTROL_RULES"] = json.dumps({"rules": [{"enabled": enabled, "protocol": protocol, "listen_host": host, "listen_port": 18080}]})
                    if stopped:
                        (self.base / "stopped").touch()
                        (self.base / "active").unlink()
                    saved = self.recovery_snapshot()
                    (self.base / "calls").unlink()
                    code, output = self.reconfigure(True, True, credentials=True)
                    self.assertEqual(code, 1 if reject else 0, output)
                    self.assertFalse(Path(str(database) + "-journal").exists())
                    self.assertFalse(Path(str(database) + "-wal").exists())
                    self.assertFalse(Path(str(database) + "-shm").exists())
                    if reject:
                        self.assertIn("Web 监听与已有转发规则冲突", output)
                        self.assertEqual(self.recovery_snapshot(), saved)
                        self.assertFalse(any(call[0] == "stop" for call in self.actions("systemctl")))
                    else:
                        self.assertEqual(database.read_bytes(), saved[str(database)][3])
                        self.assertEqual(database.stat().st_uid, saved[str(database)][0])
                        self.assertEqual(database.stat().st_mode, saved[str(database)][2])
                    self.env.pop("CONTROL_RULES")
                    self.reset_installation()

    def test_rule_created_between_preflight_and_stop_is_preserved_and_web_change_rolls_back(self):
        self.assertEqual(self.install_mode(False, True)[0], 0)
        config = self.config.read_bytes()
        state = self.state.read_bytes()
        self.env["INSERT_RULE_ON_STOP"] = "1"
        (self.base / "calls").unlink()
        code, output = self.reconfigure(True, True, credentials=True)
        self.assertNotEqual(code, 0, output)
        self.assertIn("已恢复原版本", output)
        self.assertEqual(self.config.read_bytes(), config)
        self.assertEqual(self.state.read_bytes(), state)
        connection = sqlite3.connect(self.base / "data/porter.db")
        try:
            self.assertEqual(connection.execute("SELECT enabled, protocol, listen_host, listen_port FROM rules").fetchall(), [(1, "tcp+udp", "127.0.0.1", 18080)])
        finally:
            connection.close()
        self.assertTrue((self.base / "active").exists())
        self.assertFalse(Path(str(self.base / "data/porter.db") + "-journal").exists())
        self.env.pop("INSERT_RULE_ON_STOP")

    def recovery_snapshot(self):
        result = {}
        for path in (self.state, self.config, self.unit, self.directory / "porterd", self.directory / "porter", self.command, self.config.parent / "cli.json", self.base / "data", self.base / "data/porter.db"):
            if path.is_symlink():
                result[str(path)] = ("link", os.readlink(path))
            elif path.exists():
                info = path.stat()
                result[str(path)] = (info.st_uid, info.st_gid, info.st_mode, path.read_bytes() if path.is_file() else None)
        return result

    def test_component_transaction_fault_and_sigkill_boundaries_restore_exact_original(self):
        cases = (("prepared", True, True, True, False), ("stopped", True, True, True, False),
                 ("backup-partial", True, True, True, False), ("backup-complete", True, True, True, False),
                 ("environment", True, True, True, False), ("link-remove", True, True, True, False),
                 ("client-remove", True, True, True, False), ("link-create", True, False, True, True),
                 ("ready", False, True, True, True), ("state", True, True, True, False))
        for kind in ("error", "kill"):
            for index, (point, old_web, old_cli, new_web, new_cli) in enumerate(cases):
                with self.subTest(kind=kind, point=point):
                    self.fixtures()
                    self.assertEqual(self.install_mode(old_web, old_cli)[0], 0)
                    preference = self.config.parent / "cli.json"
                    preference.write_text('{"schema":1,"language":"zh-CN"}')
                    preference.chmod(0o600)
                    if old_web:
                        (self.base / "data/porter.db").write_bytes(b"original rule IDs and traffic")
                    inactive = index % 2 == 1
                    if inactive:
                        (self.base / "stopped").touch()
                        (self.base / "active").unlink()
                        (self.base / "enabled").unlink()
                    before = self.recovery_snapshot()
                    self.env.update(COMPONENT_CUT=point, COMPONENT_CUT_KIND=kind)
                    code, output = self.reconfigure(new_web, new_cli, credentials=new_web and not old_web)
                    self.assertNotEqual(code, 0, output)
                    self.env.pop("COMPONENT_CUT"); self.env.pop("COMPONENT_CUT_KIND")
                    record = json.loads(self.state.read_text())
                    if record["status"] == "upgrading":
                        code, output = self.run_install(terminal=False)
                        self.assertEqual(code, 0, output)
                        self.assertIn("已恢复稳定安装", output)
                    self.assertEqual(self.recovery_snapshot(), before)
                    self.assertEqual((self.base / "active").exists(), not inactive)
                    self.assertEqual((self.base / "enabled").exists(), not inactive)
                    self.assertFalse((self.config.parent / ".install-lock").exists())
                    self.assertFalse((self.directory / ".install-lock").exists())
                    self.reset_installation()

    def test_component_rollback_interruption_and_legacy_six_key_journal_recovery(self):
        self.assert_success()
        before = self.recovery_snapshot()
        self.env.update(COMPONENT_CUT="restored", COMPONENT_CUT_KIND="kill", HEALTH_FAIL="1")
        code, output = self.reconfigure(True, False)
        self.assertNotEqual(code, 0, output)
        for key in ("COMPONENT_CUT", "COMPONENT_CUT_KIND", "HEALTH_FAIL"):
            self.env.pop(key)
        code, output = self.run_install(terminal=False)
        self.assertEqual(code, 0, output)
        self.assertEqual(self.recovery_snapshot(), before)
        self.reset_installation()
        self.legacy()
        before = self.recovery_snapshot()
        self.env["KILL_PHASE"] = "switching"
        self.assertNotEqual(self.upgrade()[0], 0)
        self.env.pop("KILL_PHASE")
        state = json.loads(self.state.read_text())
        journal = self.directory / ".tmp" / state["upgrade_id"] / "transaction.json"
        record = json.loads(journal.read_text())
        journal.write_text(json.dumps({key: value for key, value in record.items() if key in ("id", "phase", "active", "enabled", "cli", "new_link")}))
        journal.chmod(0o600)
        code, output = self.run_install(terminal=False)
        self.assertEqual(code, 0, output)
        self.assertEqual(self.recovery_snapshot(), before)

    def test_languages_help_validation_and_selection(self):
        for language, marker in (("zh-CN", "原生交互安装"), ("en", "native interactive installer")):
            code, output = self.run_install(args=("--help", "--lang", language), terminal=False)
            self.assertEqual(code, 0, output)
            self.assertIn(marker, output)
            self.assertIn("--purge", output)
            self.assertFalse(self.calls())
        for args, text in ((("--purge",), "--purge requires --uninstall"),
                           (("--uninstall", "--version", "v1.2.3"), "--version cannot"),
                           (("--unknown",), "Unknown argument"),
                           (("--version",), "Missing value")):
            code, output = self.run_install(args=args + ("--lang", "en"), terminal=False)
            self.assertNotEqual(code, 0, output)
            self.assertIn(text, output)
            self.assertFalse(self.directory.exists())
        dialogue = [("Language", "1"), ("Admin username:", "admin"),
                    ("Admin password:", "one"), ("Confirm password:", "two"),
                    ("Admin password:", "english-secret"), ("Confirm password:", "english-secret"),
                    ("Web listen address", "bad"), ("Web listen address", "[::]:18080")]
        code, output = self.run_install(dialogue, language=None)
        self.assertEqual(code, 0, output)
        self.assertIn("Passwords do not match", output)
        self.assertIn("Enter a valid", output)
        self.assertIn("User portal: http://[::1]:18080/user/", output)
        self.assertIn("Admin portal: http://[::1]:18080/admin/", output)
        self.assertNotIn("english-secret", output)
        self.assertTrue(self.terminal_echo_restored)
        self.reset_installation()
        code, output = self.run_install([("Language", ""), ("Admin username:", "admin"), ("Admin password:", "secret"), ("Confirm password:", "secret"), ("Web listen address", "0.0.0.0:18080")], language=None)
        self.assertEqual(code, 0, output)
        self.assertIn("Porter is ready", output)

    def test_uninstall_retains_all_data_and_reinstalls_recorded_version(self):
        self.assert_success()
        database = self.base / "data/porter.db"
        database.write_bytes(b"original database\x00bytes")
        (self.base / "data/porter.db-wal").write_bytes(b"complete wal")
        (self.base / "data/porter.db-shm").write_bytes(b"complete shm")
        with self.config.open("a") as file:
            file.write('PORTER_COOKIE_SECURE="true"\n')
        before = self.snapshot()
        extra = self.directory / "user-file.txt"
        extra.write_text("额外文件")
        (self.base / "calls").unlink()
        code, output = self.uninstall(directory=False)
        self.assertEqual(code, 0, output)
        self.assertIn("已保留", output)
        self.assertFalse(self.unit.exists() or (self.directory / "porter").exists())
        self.assertFalse((self.base / "enabled").exists())
        self.assertTrue((self.base / "stopped").exists())
        self.assertTrue((self.base / "user").exists() and (self.base / "group").exists())
        self.assertEqual(extra.read_text(), "额外文件")
        self.assertEqual(json.loads(self.state.read_text())["status"], "uninstalled")
        after = self.snapshot()
        self.assertEqual({k:v for k,v in before.items() if not k.endswith("install.json")},
                         {k:v for k,v in after.items() if not k.endswith("install.json")})
        self.assertFalse(self.actions("curl") or self.actions("apt-get") or self.actions("dnf") or self.actions("useradd"))
        self.assertEqual([call[0] for call in self.actions("systemctl") if call[0] in ("stop", "disable", "daemon-reload")],
                         ["stop", "disable", "daemon-reload"])
        code, output = self.uninstall(terminal=False, directory=False)
        self.assertEqual(code, 0, output)
        self.assertIn("无需重复操作", output)
        (self.base / "calls").unlink()
        self.assert_success(terminal=False)
        self.assertEqual(self.config.read_bytes(), before[str(self.config.relative_to(self.base))])
        self.assertEqual(database.read_bytes(), b"original database\x00bytes")
        self.assertTrue(any("/releases/tags/v1.2.3" in str(call) for call in self.actions("curl")))
        self.assertFalse(any("/releases/latest" in str(call) for call in self.actions("curl")))
        self.assertFalse(self.actions("useradd"))

    def test_uninstall_cancel_and_missing_terminal_do_not_modify_installation(self):
        self.assert_success()
        before = self.snapshot()
        binary = (self.directory / "porter").read_bytes()
        for purge, answers, terminal in ((False, [("确认卸载？", "")], True),
                                          (True, [("确认卸载？", "yes"), ("输入 DELETE", "no")], True),
                                          (False, None, False)):
            (self.base / "calls").unlink(missing_ok=True)
            code, output = self.uninstall(purge, answers=answers, terminal=terminal)
            self.assertEqual(code, 0 if terminal else 1, output)
            self.assertEqual(self.snapshot(), before)
            self.assertEqual((self.directory / "porter").read_bytes(), binary)
            self.assertTrue(self.unit.exists())
            self.assertFalse(any(call[0] in ("stop", "disable", "daemon-reload") for call in self.actions("systemctl")))
            self.assertFalse((self.config.parent / ".install-lock").exists())
            self.assertFalse((self.directory / ".install-lock").exists())

    def test_purge_english_removes_managed_resources_and_preserves_extra_files(self):
        self.assert_success()
        (self.config.parent / "notes.txt").write_text("保留配置目录的额外文件")
        (self.directory / "notes.txt").write_text("保留程序目录的额外文件")
        code, output = self.uninstall(True, language="en", directory=False)
        self.assertEqual(code, 0, output)
        self.assertIn("Porter has been purged", output)
        self.assertIn("Historical upgrade backups may contain credentials", output)
        self.assertIn(str(self.directory / ".tmp"), output)
        self.assertIn(str(self.base) + "/.porter-failed-upgrade.<id>", output)
        self.assertIn("business data", output)
        for path in (self.config, self.state, self.unit, self.base / "data", self.base / "user", self.base / "group", self.directory / "porter"):
            self.assertFalse(path.exists(), str(path))
        self.assertTrue((self.directory / "notes.txt").exists())
        self.assertTrue((self.config.parent / "notes.txt").exists())
        code, output = self.uninstall(True, language="en", terminal=False)
        self.assertEqual(code, 0, output)
        self.assertIn("already uninstalled", output)

    def test_purge_after_retained_uninstall_and_clean_repeat(self):
        self.assert_success()
        self.assertEqual(self.uninstall()[0], 0)
        code, output = self.uninstall(True, directory=False)
        self.assertEqual(code, 0, output)
        self.assertFalse(self.config.parent.exists() or self.directory.exists())
        self.assertEqual(self.uninstall(True, terminal=False)[0], 0)

    def test_uninstall_failures_preserve_recovery_record_and_allow_retry(self):
        for failure, value in (("STOP_FAIL", "1"), ("DISABLE_FAIL", "1"), ("STILL_ACTIVE", "1"),
                               ("REMOVE_FAIL", "porter.service"), ("RELOAD_FAIL", "1"),
                               ("USERDEL_FAIL", "1"), ("GROUPDEL_FAIL", "1")):
            with self.subTest(failure=failure):
                self.assert_success()
                saved = self.config.read_bytes()
                self.env[failure] = value
                self.env["CACHE_FRAGMENT"] = "1"
                code, output = self.uninstall(True)
                self.assertNotEqual(code, 0, output)
                self.assertNotIn("已彻底删除", output)
                self.assertEqual(json.loads(self.state.read_text())["status"], "uninstalling")
                if failure in ("STOP_FAIL", "DISABLE_FAIL", "STILL_ACTIVE"):
                    self.assertTrue(self.unit.exists() and (self.directory / "porter").exists())
                    self.assertEqual(self.config.read_bytes(), saved)
                del self.env[failure]
                code, output = self.run_install(terminal=False)
                self.assertNotEqual(code, 0, output)
                code, output = self.uninstall(True)
                self.assertEqual(code, 0, output)
                self.assertFalse(self.state.exists() or (self.base / "data").exists())
                self.env.pop("CACHE_FRAGMENT", None)
                self.reset_installation()

    def test_uninstall_rejects_foreign_resources_bad_state_and_path_mismatch(self):
        self.assert_success()
        for failure in ("VENDOR_UNIT", "DROP_IN", "FOREIGN_ACCOUNT", "SHARED_GROUP"):
            with self.subTest(failure=failure):
                self.env[failure] = "1"
                saved = self.snapshot()
                code, output = self.uninstall(True, terminal=False)
                self.assertNotEqual(code, 0, output)
                self.assertEqual(self.snapshot(), saved)
                del self.env[failure]
        code, output = self.run_install(args=("--uninstall", "--install-dir", str(self.base / "wrong")), terminal=False)
        self.assertNotEqual(code, 0, output)
        original = self.state.read_bytes()
        self.state.write_text('{"schema": 1, "directory": "/", "status": "complete"}')
        code, output = self.uninstall(directory=False, terminal=False)
        self.assertNotEqual(code, 0, output)
        self.state.write_bytes(original)
        self.unit.write_text("未知服务")
        code, output = self.uninstall(terminal=False)
        self.assertNotEqual(code, 0, output)
        self.assertEqual(self.unit.read_text(), "未知服务")
        self.assertTrue((self.directory / "porter").exists())

    def test_uninstall_drop_in_query_failure_preserves_resources(self):
        self.assert_success()
        before = self.snapshot()
        binary = (self.directory / "porter").read_bytes()
        unit = self.unit.read_bytes()
        (self.base / "calls").unlink()
        self.env["DROP_IN_QUERY_FAIL"] = "1"
        code, output = self.uninstall(True, answers=[], language="en")
        self.assertNotEqual(code, 0, output)
        self.assertIn("Cannot query the systemd service", output)
        self.assertEqual(self.snapshot(), before)
        self.assertEqual((self.directory / "porter").read_bytes(), binary)
        self.assertEqual(self.unit.read_bytes(), unit)
        self.assertFalse(any(call[0] in ("stop", "disable", "daemon-reload") for call in self.actions("systemctl")))
        self.assertFalse(self.actions("rm") or self.actions("userdel") or self.actions("groupdel"))

    def test_uninstall_rejects_configuration_and_data_type_conflicts(self):
        self.assert_success()
        for path in (self.config, self.base / "data", self.base / "data/porter.db"):
            with self.subTest(path=str(path)):
                moved = Path(str(path) + ".saved")
                was_directory = path.is_dir()
                path.rename(moved)
                if was_directory:
                    path.write_text("未知文件")
                else:
                    path.mkdir()
                    (path / "notes.txt").write_text("未知目录内容")
                before = self.snapshot()
                (self.base / "calls").unlink()
                code, output = self.uninstall(True, answers=[], language="en")
                self.assertNotEqual(code, 0, output)
                self.assertIn("unexpected file type", output)
                self.assertEqual(self.snapshot(), before)
                self.assertEqual(json.loads(self.state.read_text())["status"], "complete")
                self.assertTrue(self.unit.exists() and (self.directory / "porter").exists())
                self.assertFalse(any(call[0] in ("stop", "disable", "daemon-reload") for call in self.actions("systemctl")))
                self.assertFalse(self.actions("rm") or self.actions("userdel") or self.actions("groupdel"))
                if was_directory:
                    self.assertEqual(path.read_text(), "未知文件")
                    path.unlink()
                else:
                    self.assertEqual((path / "notes.txt").read_text(), "未知目录内容")
                    shutil.rmtree(path)
                moved.rename(path)

    def test_managed_state_change_before_locks_aborts_installation(self):
        for race in ("uninstalled", "purged", "version", "arch", "directory"):
            with self.subTest(race=race):
                self.assert_success()
                saved = self.config.read_bytes()
                (self.base / "calls").unlink()
                self.env["MANAGED_STATE_RACE"] = race
                code, output = self.run_install(terminal=False, language="en")
                self.assertNotEqual(code, 0, output)
                self.assertIn("Installation state changed", output)
                self.assertNotIn("unbound variable", output)
                self.assertFalse(any(call[0] in ("enable", "daemon-reload") for call in self.actions("systemctl")))
                self.assertFalse(any("-d" not in call for call in self.actions("install")))
                self.assertFalse(self.actions("useradd") or self.actions("groupadd"))
                if race in ("uninstalled", "purged"):
                    self.assertFalse(self.unit.exists() or (self.directory / "porter").exists())
                if race == "purged":
                    self.assertFalse(self.state.exists() or self.config.exists() or (self.base / "data").exists())
                else:
                    self.assertEqual(self.config.read_bytes(), saved)
                    record = json.loads(self.state.read_text())
                    self.assertEqual(record["status"], "uninstalled" if race == "uninstalled" else "complete")
                    if race == "version":
                        self.assertEqual(record["version"], "v1.2.4")
                    elif race == "arch":
                        self.assertEqual(record["arch"], "arm64")
                    elif race == "directory":
                        self.assertEqual(record["directory"], str(self.base / "other-program"))
                self.assertFalse((self.config.parent / ".install-lock").exists())
                self.assertFalse((self.directory / ".install-lock").exists())
                del self.env["MANAGED_STATE_RACE"]
                (self.base / "state_race_done").unlink()
                self.reset_installation()

    def test_uninstall_partial_installation_removes_uncommitted_staging_files(self):
        for purge in (False, True):
            with self.subTest(purge=purge):
                self.env["PARTIAL_INSTALL_FAIL"] = "porter.env"
                code, output = self.run_install()
                self.assertNotEqual(code, 0, output)
                del self.env["PARTIAL_INSTALL_FAIL"]
                self.assertEqual(json.loads(self.state.read_text())["status"], "initializing")
                staged = (Path(str(self.config) + ".new"), Path(str(self.state) + ".new"),
                          Path(str(self.unit) + ".new"), self.directory / ".porter.new")
                for path in staged:
                    path.write_text('PORTER_ADMIN_PASSWORD="unfinished-secret"\n')
                code, output = self.uninstall(purge)
                self.assertEqual(code, 0, output)
                self.assertNotIn("unfinished-secret", output)
                for path in staged:
                    self.assertFalse(path.exists(), str(path))
                if not purge:
                    self.assertEqual(json.loads(self.state.read_text())["status"], "uninstalled")
                self.reset_installation()

    def test_uninstall_rejects_staging_directory_and_symbolic_link_conflicts(self):
        self.assert_success()
        staged = (Path(str(self.config) + ".new"), Path(str(self.state) + ".new"),
                  Path(str(self.unit) + ".new"), self.directory / ".porter.new")
        for path in staged:
            for kind in ("directory", "symlink"):
                with self.subTest(path=str(path), kind=kind):
                    if kind == "directory":
                        path.mkdir()
                        (path / "notes.txt").write_text("未知内容")
                    else:
                        path.symlink_to(self.config)
                    before = self.snapshot()
                    (self.base / "calls").unlink()
                    code, output = self.uninstall(True, answers=[])
                    self.assertNotEqual(code, 0, output)
                    self.assertEqual(self.snapshot(), before)
                    self.assertEqual(json.loads(self.state.read_text())["status"], "complete")
                    self.assertFalse(any(call[0] in ("stop", "disable", "daemon-reload") for call in self.actions("systemctl")))
                    self.assertFalse(self.actions("rm") or self.actions("userdel") or self.actions("groupdel"))
                    if kind == "directory":
                        self.assertEqual((path / "notes.txt").read_text(), "未知内容")
                        shutil.rmtree(path)
                    else:
                        self.assertTrue(path.is_symlink())
                        path.unlink()

    def test_uninstall_unknown_resources_and_locks_are_preserved(self):
        for unknown in ("config", "data", "binary", "user", "vendor"):
            with self.subTest(unknown=unknown):
                if unknown == "config":
                    self.config.parent.mkdir()
                    self.config.write_text("未知配置")
                elif unknown == "data":
                    (self.base / "data").mkdir()
                elif unknown == "binary":
                    self.directory.mkdir()
                    (self.directory / "porter").write_text("未知程序")
                elif unknown == "vendor":
                    self.env["VENDOR_UNIT"] = "1"
                else:
                    (self.base / "user").touch()
                code, output = self.uninstall(terminal=False)
                self.assertNotEqual(code, 0, output)
                self.assertFalse(self.actions("userdel") or self.actions("groupdel") or self.actions("rm"))
                self.env.pop("VENDOR_UNIT", None)
                self.reset_installation()
        self.assert_success()
        for folder in (self.directory, self.config.parent):
            lock = folder / ".install-lock"
            lock.mkdir()
            code, output = self.uninstall(terminal=False)
            self.assertNotEqual(code, 0, output)
            self.assertTrue(lock.exists())
            self.assertEqual(json.loads(self.state.read_text())["status"], "complete")
            lock.rmdir()

    def test_uninstall_symbolic_links_and_missing_python_are_rejected(self):
        self.assert_success()
        for path in (self.unit, self.state, self.base / "data", self.directory / "porter"):
            with self.subTest(path=str(path)):
                moved = Path(str(path) + ".saved")
                path.rename(moved)
                path.symlink_to(moved)
                code, output = self.uninstall(terminal=False)
                self.assertNotEqual(code, 0, output)
                self.assertTrue(path.is_symlink() and moved.exists())
                path.unlink()
                moved.rename(path)
        (self.bin / "python3").unlink()
        code, output = self.uninstall(terminal=False)
        self.assertNotEqual(code, 0, output)
        self.assertIn("需要 Python 3", output)
        self.assertFalse(self.actions("apt-get") or self.actions("dnf"))

    def test_retained_reinstall_failures_are_retryable_and_release_is_required(self):
        self.assert_success()
        saved = self.config.read_bytes()
        self.assertEqual(self.uninstall()[0], 0)
        self.env["DOWNLOAD_FAIL"] = "1"
        code, output = self.run_install(terminal=False)
        self.assertNotEqual(code, 0, output)
        self.assertEqual(self.config.read_bytes(), saved)
        self.assertEqual(json.loads(self.state.read_text())["status"], "uninstalled")
        del self.env["DOWNLOAD_FAIL"]
        self.env["START_FAIL"] = "1"
        code, output = self.run_install(terminal=False)
        self.assertNotEqual(code, 0, output)
        self.assertEqual(json.loads(self.state.read_text())["status"], "initializing")
        del self.env["START_FAIL"]
        self.assert_success(terminal=False)
        self.assertEqual(self.config.read_bytes(), saved)

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
        self.command.unlink(missing_ok=True)
        for path in (self.directory, self.etc / "porter", self.base / "data"):
            if path.exists():
                shutil.rmtree(path)
        for path in (self.unit, self.base / "calls", self.base / "user", self.base / "group", self.base / "enabled", self.base / "active", self.base / "exited", self.base / "stopped", self.base / "cached_fragment"):
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


    def upgrade(self, version="v1.3.0", answers=None, **kwargs):
        self.fixtures(version=version)
        return self.run_install(dialogue=[("输入 yes 确认升级", "yes")] if answers is None else answers,
                                args=("--upgrade", "--version", version), **kwargs)

    def legacy(self, version="v0.3.0"):
        self.assert_success()
        self.command.unlink()
        (self.directory / "porterd").unlink()
        program = self.directory / "porter"
        program.write_text(f'#!/bin/sh\nif [ "$1" = --version ]; then echo {version}; exit 0; fi\nexit 0\n')
        program.chmod(0o755)
        self.state.write_text(json.dumps({"schema": 1, "directory": str(self.directory), "arch": "amd64", "version": version, "status": "complete"}))
        self.state.chmod(0o600)
        self.config.write_text("\n".join(line for line in self.config.read_text().splitlines() if "_ENABLED=" not in line) + "\n")
        text = self.unit.read_text().replace('/porterd"', '/porter"')
        self.unit.write_text(text)

    def test_split_layout_path_and_link_ownership(self):
        self.assert_success()
        state = json.loads(self.state.read_text())
        self.assertEqual((state["schema"], state["layout"], state["command_link"]), (3, "split", str(self.command)))
        self.assertEqual(os.readlink(self.command), str(self.directory / "porter"))
        result = subprocess.run([str(self.command), "--version"], capture_output=True, text=True, env={"PATH": str(self.command.parent)})
        self.assertEqual((result.returncode, result.stdout.strip()), (0, "v1.2.3"))
        self.assertIn("/porterd", self.unit.read_text())
        self.command.unlink()
        self.env["DOWNLOAD_FAIL"] = "1"
        self.assert_success(terminal=False)
        self.assertTrue(self.command.is_symlink())
        for kind in ("file", "directory", "dangling", "different"):
            self.command.unlink()
            if kind == "file":
                self.command.write_text("unknown")
            elif kind == "directory":
                self.command.mkdir()
            else:
                self.command.symlink_to(self.base / kind)
            before = self.snapshot()
            (self.base / "calls").unlink()
            code, output = self.uninstall(True, answers=[])
            self.assertNotEqual(code, 0, output)
            self.assertEqual(before, self.snapshot())
            self.assertFalse(any(call[0] == "stop" for call in self.actions("systemctl")))
            if kind == "directory":
                self.command.rmdir()
            else:
                self.command.unlink()
            self.command.symlink_to(self.directory / "porter")

    def test_unknown_same_target_link_is_not_adopted(self):
        self.command.symlink_to(self.directory / "porter")
        code, output = self.run_install()
        self.assertNotEqual(code, 0, output)
        self.assertFalse(self.state.exists())
        self.assertTrue(self.command.is_symlink())

    def test_direct_system_directory_has_no_self_link(self):
        self.directory = self.command.parent
        self.assert_success()
        self.assertFalse(self.command.is_symlink())
        self.assertNotIn("command_link", json.loads(self.state.read_text()))
        code, output = self.uninstall()
        self.assertEqual(code, 0, output)
        self.assertFalse(self.command.exists() or (self.directory / "porterd").exists())

    def test_cli_preference_retained_and_purged_precisely(self):
        self.assert_success()
        preference = self.config.parent / "cli.json"
        preference.write_text('{"schema":1,"language":"zh-CN"}')
        preference.chmod(0o600)
        pending = preference.parent / (".cli-" + "a" * 24 + ".new")
        pending.write_text("partial")
        pending.chmod(0o600)
        unknown = preference.parent / "notes.txt"
        unknown.write_text("unknown")
        self.assertEqual(self.uninstall()[0], 0)
        self.assertTrue(preference.exists() and pending.exists())
        self.assert_success(terminal=False)
        code, output = self.uninstall(True)
        self.assertEqual(code, 0, output)
        self.assertIn("历史升级备份可能包含凭据", output)
        self.assertIn(str(self.directory / ".tmp"), output)
        self.assertIn(str(self.base) + "/.porter-failed-upgrade.<标识>", output)
        self.assertIn("业务数据", output)
        self.assertFalse(preference.exists() or pending.exists() or self.config.exists() or self.state.exists())
        self.assertEqual(unknown.read_text(), "unknown")

    def test_cli_config_conflicts_refuse_before_stop(self):
        self.assert_success()
        preference = self.config.parent / "cli.json"
        for kind in ("directory", "symlink", "permissions", "schema"):
            if kind == "directory":
                preference.mkdir()
            elif kind == "symlink":
                preference.symlink_to(self.config)
            else:
                preference.write_text('{"schema":7,"language":"en"}' if kind == "schema" else '{"schema":1,"language":"en"}')
                preference.chmod(0o644 if kind == "permissions" else 0o600)
            (self.base / "calls").unlink()
            code, output = self.uninstall(True, answers=[])
            self.assertNotEqual(code, 0, output)
            self.assertFalse(any(call[0] == "stop" for call in self.actions("systemctl")))
            if kind == "directory":
                preference.rmdir()
            else:
                preference.unlink()

    def test_legacy_repeat_restore_and_explicit_migration(self):
        self.legacy()
        database = self.base / "data/porter.db"
        database.write_bytes(b"accounts rules statistics")
        (self.base / "data/porter.db-wal").write_bytes(b"wal")
        before = self.config.read_bytes()
        self.env["DOWNLOAD_FAIL"] = "1"
        output = self.assert_success(terminal=False)
        self.assertIn("旧单程序", output)
        self.assertFalse(self.command.exists() or (self.directory / "porterd").exists())
        del self.env["DOWNLOAD_FAIL"]
        code, output = self.upgrade()
        self.assertEqual(code, 0, output)
        self.assertEqual(database.read_bytes(), b"accounts rules statistics")
        values = self.environment()
        self.assertEqual(values["PORTER_WEB_ENABLED"], "true")
        self.assertEqual(values["PORTER_CLI_ENABLED"], "false")
        self.assertEqual((self.base / "data/porter.db-wal").read_bytes(), b"wal")
        self.assertEqual(json.loads(self.state.read_text())["schema"], 3)
        self.assertFalse(self.command.exists() or (self.directory / "porter").exists())
        backups = list((self.directory / ".tmp").glob("upgrade.*"))
        self.assertEqual(len(backups), 1)
        self.assertEqual(json.loads((backups[0] / "install.json").read_text())["schema"], 1)
        self.assertEqual((backups[0] / "data/porter.db").read_bytes(), b"accounts rules statistics")
        self.assertEqual(backups[0].stat().st_mode & 0o777, 0o700)

    def test_legacy_retained_reinstall_uses_legacy_archive(self):
        self.legacy("v0.2.0")
        self.assertEqual(self.uninstall()[0], 0)
        self.fixtures(kind="legacy", version="v0.2.0")
        self.assert_success(terminal=False)
        self.assertEqual(json.loads(self.state.read_text())["schema"], 1)
        self.assertFalse(self.command.exists() or (self.directory / "porterd").exists())

    def test_upgrade_cancel_network_checksum_and_legacy_target_do_not_stop(self):
        self.assert_success()
        for failure in ("DOWNLOAD_FAIL", "BAD_CHECKSUM", "DUPLICATE_SUM", "legacy", "cancel"):
            self.fixtures(kind="legacy" if failure == "legacy" else "normal", version="v1.3.0")
            if failure in ("DOWNLOAD_FAIL", "BAD_CHECKSUM", "DUPLICATE_SUM"):
                self.env[failure] = "1"
            before = self.snapshot()
            (self.base / "calls").unlink()
            code, output = self.run_install(dialogue=[("输入 yes 确认升级", "")] if failure == "cancel" else [], args=("--upgrade",))
            self.assertEqual(code == 0, failure == "cancel", output)
            self.assertEqual(self.snapshot(), before)
            self.assertFalse(any(call[0] == "stop" for call in self.actions("systemctl")))
            self.env.pop(failure, None)

    def test_semver_comparison_and_upgrade_idempotence(self):
        helper = SCRIPT.split("PY_HELPER='", 1)[1].split("\n'\npy()", 1)[0]
        namespace = {}
        exec(helper.split("\ntry:\n", 1)[0], namespace)
        compare = namespace["compare_versions"]
        order = ["v1.0.0-alpha", "v1.0.0-alpha.1", "v1.0.0-alpha.beta", "v1.0.0-beta", "v1.0.0-beta.2", "v1.0.0-beta.11", "v1.0.0-rc.1", "v1.0.0", "v1.0.1", "v2.0.0"]
        for left, right in zip(order, order[1:]):
            self.assertLess(compare(left, right), 0)
            self.assertGreater(compare(right, left), 0)
        for value in ("v01.2.3", "v1.2.3-01", "v1.2.3-a..b"):
            with self.assertRaises(ValueError):
                compare(value, "v1.2.3")
        self.assert_success()
        (self.base / "calls").unlink()
        code, output = self.upgrade(version="v1.2.3", terminal=False)
        self.assertEqual(code, 0, output)
        self.assertFalse(any(call[0] in ("stop", "start", "enable") for call in self.actions("systemctl")))
        code, output = self.upgrade(version="v1.2.2", answers=[])
        self.assertNotEqual(code, 0, output)
        self.assertIn("拒绝降级", output)

    def test_upgrade_restore_complete_data_and_original_service_intent(self):
        for inactive in (False, True):
            self.fixtures()
            self.assert_success()
            if inactive:
                (self.base / "stopped").touch()
                (self.base / "enabled").unlink()
                (self.base / "active").unlink()
            (self.base / "data/porter.db").write_bytes(b"old-db")
            (self.base / "data/porter.db-wal").write_bytes(b"old-wal")
            before = self.snapshot()
            self.env["UPGRADE_HEALTH_FAIL_ONCE"] = "1"
            code, output = self.upgrade()
            self.assertNotEqual(code, 0, output)
            self.assertIn("已恢复原版本", output)
            self.assertEqual(self.snapshot(), before)
            self.assertEqual((self.base / "enabled").exists(), not inactive)
            self.assertEqual((self.base / "active").exists(), not inactive)
            self.assertEqual(json.loads(self.state.read_text())["version"], "v1.2.3")
            del self.env["UPGRADE_HEALTH_FAIL_ONCE"]
            self.reset_installation()

    def test_upgrade_strong_interrupt_recovery_all_persistent_phases(self):
        for phase in ("stopped", "backup_complete", "switching"):
            self.legacy()
            (self.base / "data/porter.db").write_bytes(b"old-db")
            before = self.snapshot()
            self.env["KILL_PHASE"] = phase
            code, output = self.upgrade()
            self.assertNotEqual(code, 0, output)
            self.assertEqual(json.loads(self.state.read_text())["status"], "upgrading")
            del self.env["KILL_PHASE"]
            code, output = self.run_install(terminal=False)
            self.assertEqual(code, 0, output)
            self.assertIn("已恢复稳定安装", output)
            self.assertEqual(self.snapshot(), before)
            self.assertFalse(self.command.exists() or (self.directory / "porterd").exists())
            self.assertTrue((self.base / "active").exists())
            self.reset_installation()

    def test_lock_metadata_invalid_and_active_owners_are_not_reclaimed(self):
        self.assert_success()
        lock = self.config.parent / ".install-lock"
        with open("/proc/" + str(os.getpid()) + "/stat") as file:
            current = file.read().rsplit(")", 1)[1].split()[19]
        records = [{"pid": 0}, {"pid": True, "start": None}, {"pid": -1, "start": "1"}, {"pid": 99999999, "start": ""}, {"pid": 99999999, "start": "1", "unknown": 1}, {"pid": os.getpid(), "start": current}]
        for record in records:
            lock.mkdir(mode=0o700)
            owner = lock / "owner.json"
            owner.write_text(json.dumps(record))
            owner.chmod(0o600)
            before = owner.read_bytes()
            code, output = self.run_install(terminal=False)
            self.assertNotEqual(code, 0, output)
            self.assertEqual(owner.read_bytes(), before)
            owner.unlink(); lock.rmdir()
        lock.mkdir(mode=0o700)
        (lock / "owner.json").write_text(json.dumps({"pid": 99999999, "start": "1"}))
        (lock / "owner.json").chmod(0o600)
        self.assert_success(terminal=False)
        self.assertFalse(lock.exists())

    def test_archive_both_members_required_unique_and_regular(self):
        for kind in ("missing-server", "server-link", "duplicate-server"):
            self.fixtures(kind=kind)
            code, output = self.run_install()
            self.assertNotEqual(code, 0, output)
            self.assertFalse(self.state.exists() or self.unit.exists())
            self.reset_installation()

    def test_upgrade_success_preserves_stopped_disabled_intent_and_cli(self):
        self.assert_success()
        (self.base / "stopped").touch()
        (self.base / "enabled").unlink()
        (self.base / "active").unlink()
        cli = self.config.parent / "cli.json"
        cli.write_text('{"schema":1,"language":"zh-CN"}')
        cli.chmod(0o600)
        code, output = self.upgrade()
        self.assertEqual(code, 0, output)
        self.assertFalse((self.base / "enabled").exists() or (self.base / "active").exists())
        self.assertEqual(cli.read_text(), '{"schema":1,"language":"zh-CN"}')
        backup = next((self.directory / ".tmp").glob("upgrade.*"))
        self.assertEqual((backup / "cli.json").read_bytes(), cli.read_bytes())

    def test_upgrade_failure_at_replacement_and_backup_boundaries_recovers(self):
        for legacy in (True, False):
            # 旧单程序升级保持仅Web，不再替换CLI；双入口升级覆盖客户端替换。
            boundaries = ("backup", "porterd", "unit", "state") if legacy else ("backup", "porter", "porterd", "unit", "state")
            for boundary in boundaries:
                with self.subTest(legacy=legacy, boundary=boundary):
                    self.fixtures()
                    if legacy:
                        self.legacy()
                    else:
                        self.assert_success()
                    (self.base / "data/porter.db").write_bytes(b"old database")
                    (self.directory / "porter").chmod(0o750)
                    self.unit.chmod(0o640)
                    before = self.recovery_snapshot()
                    self.env["FAULT_BOUNDARY"] = boundary
                    code, output = self.upgrade()
                    self.assertNotEqual(code, 0, output)
                    self.assertIn("已恢复原版本", output)
                    self.assertEqual(before, self.recovery_snapshot())
                    del self.env["FAULT_BOUNDARY"]
                    self.reset_installation()

    def test_upgrade_rollback_interruption_can_resume(self):
        for phase in ("restoring", "restored"):
            self.fixtures()
            self.legacy()
            (self.base / "data/porter.db").write_bytes(b"old-db")
            before = self.snapshot()
            self.env["UPGRADE_HEALTH_FAIL_ONCE"] = "1"
            self.env["KILL_PHASE"] = phase
            code, output = self.upgrade()
            self.assertNotEqual(code, 0, output)
            self.assertEqual(json.loads(self.state.read_text())["status"], "upgrading")
            del self.env["KILL_PHASE"]
            del self.env["UPGRADE_HEALTH_FAIL_ONCE"]
            code, output = self.run_install(terminal=False)
            self.assertEqual(code, 0, output)
            self.assertEqual(self.snapshot(), before)
            self.assertTrue((self.base / "active").exists())
            self.reset_installation()

    def test_upgrade_recovery_after_systemctl_failure(self):
        for failure in ("STOP_FAIL", "RELOAD_FAIL", "START_FAIL"):
            self.fixtures()
            self.legacy()
            before = self.snapshot()
            self.env[failure] = "1"
            code, output = self.upgrade()
            self.assertNotEqual(code, 0, output)
            self.assertEqual(json.loads(self.state.read_text())["status"], "upgrading")
            del self.env[failure]
            code, output = self.run_install(terminal=False)
            self.assertEqual(code, 0, output)
            self.assertEqual(self.snapshot(), before)
            self.assertTrue((self.base / "active").exists())
            self.reset_installation()

    def test_corrupted_upgrade_metadata_refuses_before_stop(self):
        self.legacy()
        self.env["KILL_PHASE"] = "switching"
        self.assertNotEqual(self.upgrade()[0], 0)
        del self.env["KILL_PHASE"]
        record = json.loads(self.state.read_text())
        journal = self.directory / ".tmp" / record["upgrade_id"] / "transaction.json"
        value = json.loads(journal.read_text())
        del value["cli"]
        journal.write_text(json.dumps(value))
        before = self.snapshot()
        (self.base / "calls").unlink()
        code, output = self.run_install(terminal=False)
        self.assertNotEqual(code, 0, output)
        self.assertEqual(self.snapshot(), before)
        self.assertFalse(any(call[0] == "stop" for call in self.actions("systemctl")))

    def test_atomic_link_race_does_not_overwrite_unknown_node(self):
        self.env["LINK_RACE"] = "1"
        code, output = self.run_install()
        self.assertNotEqual(code, 0, output)
        self.assertEqual(self.command.read_text(), "racing unknown file")
        self.assertFalse(self.command.is_symlink())
        self.assertEqual(json.loads(self.state.read_text())["status"], "initializing")

    def test_uninstall_sigkill_can_resume_without_abandoning_global_lock(self):
        for boundary in ("stop", "unit"):
            self.fixtures()
            self.assert_success()
            self.env["KILL_UNINSTALL"] = boundary
            code, output = self.uninstall()
            self.assertNotEqual(code, 0, output)
            self.assertEqual(json.loads(self.state.read_text())["status"], "uninstalling")
            del self.env["KILL_UNINSTALL"]
            code, output = self.uninstall()
            self.assertEqual(code, 0, output)
            self.assertEqual(json.loads(self.state.read_text())["status"], "uninstalled")
            self.assertFalse((self.config.parent / ".install-lock").exists())
            self.reset_installation()

    def test_upgrade_shell_sigterm_waits_for_rollback(self):
        self.legacy()
        before = self.snapshot()
        self.env["SIGNAL_PHASE"] = "stopped"
        code, output = self.upgrade()
        self.assertNotEqual(code, 0, output)
        self.assertEqual(json.loads(self.state.read_text())["status"], "complete")
        self.assertEqual(before, self.snapshot())
        self.assertFalse((self.config.parent / ".install-lock").exists())
        self.assertTrue((self.base / "active").exists())

    def test_upgrade_rejects_data_mount_root_before_stop_and_rechecks_under_lock(self):
        for operation in ("upgrade", "reconfigure"):
            self.fixtures()
            self.assert_success()
            before = self.snapshot()
            mountinfo = self.base / "mountinfo"
            data = str(self.base / "data")
            root_line = "1 0 0:1 / / rw - tmpfs tmpfs rw\n"
            mounted_line = "2 1 0:1 / " + data.replace(" ", "\\040") + " rw - tmpfs tmpfs rw\n"
            self.env["MOUNT_FIXTURE"] = "1"
            for locked in (False, True):
                with self.subTest(operation=operation, locked=locked):
                    mountinfo.write_text(root_line if locked else root_line + mounted_line)
                    if locked:
                        self.env["MOUNT_AFTER_LOCK"] = mounted_line
                    (self.base / "calls").unlink()
                    if operation == "upgrade":
                        code, output = self.upgrade(answers=[("输入 yes 确认升级", "yes")] if locked else [])
                    else:
                        code, output = self.reconfigure(True, False)
                    self.assertNotEqual(code, 0, output)
                    self.assertIn("挂载根", output)
                    self.assertEqual(self.snapshot(), before)
                    self.assertFalse(any(call[0] == "stop" for call in self.actions("systemctl")))
                    self.env.pop("MOUNT_AFTER_LOCK", None)
            self.env.pop("MOUNT_FIXTURE")
            self.reset_installation()

    def test_mountinfo_parent_filesystem_allowed_and_escaped_mountpoint_decoded(self):
        helper = SCRIPT.split("PY_HELPER='", 1)[1].split("\n'\npy()", 1)[0]
        mountinfo = self.base / "mountinfo"
        helper = helper.replace('"/proc/self/mountinfo"', json.dumps(str(mountinfo)))
        namespace = {}
        exec(helper.split("\ntry:\n", 1)[0], namespace)
        check = namespace["check_data_mount"]
        mountinfo.write_text("1 0 0:1 / / rw - tmpfs tmpfs rw\n2 1 0:2 / /var rw - ext4 /dev/disk rw\n")
        check("/var/lib/porter")
        mountinfo.write_text("1 0 0:1 / /var/lib/porter\\040data rw - tmpfs tmpfs rw\n")
        with self.assertRaises(ValueError):
            check("/var/lib/porter data")
        for text in ("", "malformed\n"):
            mountinfo.write_text(text)
            with self.assertRaises(ValueError):
                check("/var/lib/porter")

if __name__ == "__main__":
    unittest.main(verbosity=2)
