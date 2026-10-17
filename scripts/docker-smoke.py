#!/usr/bin/env python3
"""通过真实 TCP、UDP 和管理接口验证 Linux host 网络容器。"""

import http.cookiejar
import json
import random
import re
import socket
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
import uuid


def docker(*args, check=True):
    return subprocess.run(
        ["docker", *args], check=check, text=True, capture_output=True
    )


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


class EchoServers:
    def __init__(self):
        for _ in range(100):
            self.tcp = socket.socket()
            self.udp = socket.socket(type=socket.SOCK_DGRAM)
            self.tcp.bind(("127.0.0.1", 0))
            try:
                self.udp.bind(self.tcp.getsockname())
                break
            except OSError:
                self.tcp.close()
                self.udp.close()
        else:
            raise RuntimeError("Could not allocate the same numeric port for TCP/UDP echo servers")
        self.tcp.listen()
        self.tcp.settimeout(0.2)
        self.udp.settimeout(0.2)
        self.closed = threading.Event()
        self.threads = [
            threading.Thread(target=self.accept_tcp, daemon=True),
            threading.Thread(target=self.echo_udp, daemon=True),
        ]
        for thread in self.threads:
            thread.start()

    @staticmethod
    def echo_connection(connection):
        with connection:
            connection.settimeout(5)
            while True:
                data = connection.recv(65536)
                if not data:
                    return
                connection.sendall(data)

    def accept_tcp(self):
        while not self.closed.is_set():
            try:
                connection, _ = self.tcp.accept()
            except socket.timeout:
                continue
            except OSError:
                return
            threading.Thread(
                target=self.echo_connection, args=(connection,), daemon=True
            ).start()

    def echo_udp(self):
        while not self.closed.is_set():
            try:
                data, address = self.udp.recvfrom(65536)
                self.udp.sendto(data, address)
            except socket.timeout:
                continue
            except OSError:
                return

    def close(self):
        self.closed.set()
        self.tcp.close()
        self.udp.close()
        for thread in self.threads:
            thread.join(timeout=1)


class Panel:
    def __init__(self, port, portal="admin"):
        self.portal = portal
        self.owner_id = ""
        self.origin = f"http://127.0.0.1:{port}"
        self.client = urllib.request.build_opener(
            urllib.request.ProxyHandler({}),
            urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()),
        )
        self.csrf = ""

    def request(self, method, path, payload=None, revision=None, status=200):
        headers = {"Origin": self.origin}
        data = None
        if payload is not None:
            if method == "POST" and path == "/api/admin/rules" and "owner_id" not in payload:
                payload = {**payload, "owner_id": self.owner_id}
            headers["Content-Type"] = "application/json"
            data = json.dumps(payload).encode()
        if self.csrf:
            headers["X-CSRF-Token"] = self.csrf
        if revision is not None:
            headers["If-Match"] = f'"{revision}"'
        request = urllib.request.Request(
            self.origin + path, data=data, headers=headers, method=method
        )
        try:
            response = self.client.open(request, timeout=5)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            body = response.read()
            expected = (status,) if isinstance(status, int) else status
            if response.status not in expected:
                raise AssertionError(
                    f"{method} {path}: expected {status}, got {response.status}: "
                    + body.decode(errors="replace")
                )
            if not body:
                return None
            if "application/json" in response.headers.get("Content-Type", ""):
                return json.loads(body)
            return body.decode()

    def ready(self):
        last_error = None
        for _ in range(100):
            try:
                self.request("GET", "/healthz")
                return
            except (OSError, AssertionError, urllib.error.URLError) as error:
                last_error = error
                time.sleep(0.1)
        raise RuntimeError(f"Container did not become ready: {last_error}")

    def login(self, username="smoke-admin", password="smoke-only-long-password"):
        session = self.request(
            "POST", f"/api/{self.portal}/auth/login",
            {"username": username, "password": password},
        )
        self.csrf = session["csrf_token"]
        self.owner_id = session["user_id"]
        assert session["username"] == username
        assert session["role"] == ("admin" if self.portal == "admin" else "user")


def verify_tcp(port):
    payload = b"porter-container-tcp\x00"
    with socket.create_connection(("127.0.0.1", port), timeout=5) as client:
        client.sendall(payload)
        client.shutdown(socket.SHUT_WR)
        result = bytearray()
        while data := client.recv(65536):
            result.extend(data)
        assert result == payload, "Incomplete TCP response after half-close"


def verify_udp(port):
    with socket.socket(type=socket.SOCK_DGRAM) as first, socket.socket(
        type=socket.SOCK_DGRAM
    ) as second:
        first.settimeout(5)
        second.settimeout(5)
        first.sendto(b"first", ("127.0.0.1", port))
        second.sendto(b"second", ("127.0.0.1", port))
        assert first.recv(65536) == b"first", "UDP client response mismatch"
        assert second.recv(65536) == b"second", "UDP client response mismatch"
        first.sendto(b"", ("127.0.0.1", port))
        assert first.recv(65536) == b"", "Zero-length UDP datagram was lost"


def verify_traffic(port):
    verify_tcp(port)
    verify_udp(port)


def verify_stopped(port, protocols=("tcp", "udp")):
    if "tcp" in protocols:
        try:
            client = socket.create_connection(("127.0.0.1", port), timeout=1)
        except OSError:
            pass
        else:
            client.close()
            raise AssertionError("TCP still accepts connections after the rule stopped")
    if "udp" in protocols:
        with socket.socket(type=socket.SOCK_DGRAM) as client:
            client.settimeout(0.3)
            client.sendto(b"should-not-echo", ("127.0.0.1", port))
            try:
                client.recv(65536)
            except socket.timeout:
                pass
            else:
                raise AssertionError("UDP still forwards traffic after the rule stopped")


def rule_input(rule, **changes):
    fields = ("name", "protocol", "listen_host", "listen_port", "target_host", "target_port", "enabled")
    return {**{key: rule[key] for key in fields}, **changes}


def verify_native_control_disabled(name):
    # 显式语言确保检查的是容器限制，而不是无终端时的语言选择失败。
    result = docker(
        "exec", name, "/porter", "rules", "list", "--lang", "en", "--json",
        "--socket", "/data/smoke-control.sock", check=False,
    )
    assert result.returncode == 1, "Container rule CLI must reject native management"
    assert result.stdout == "", "Rejected rule CLI wrote to standard output"
    assert json.loads(result.stderr)["error"]["code"] == "native_only", result.stderr
    # scratch 镜像没有 shell；Docker 归档接口能直接检查容器及数据卷路径。
    for path in ("/data/porter-control.sock", "/data/smoke-control.sock"):
        missing = subprocess.run(
            ["docker", "cp", f"{name}:{path}", "-"], capture_output=True, timeout=10,
        )
        assert missing.returncode != 0 and b"Could not find the file" in missing.stderr, (
            f"Container unexpectedly exposes a control node at {path}: "
            + missing.stderr.decode(errors="replace")
        )


def main():
    image = sys.argv[1] if len(sys.argv) > 1 else "porter:local"
    name = "porter-smoke-" + uuid.uuid4().hex[:12]
    volume = name + "-data"
    http_port = free_port()
    echoes = EchoServers()

    def start():
        docker(
            "run", "-d", "--name", name, "--network", "host",
            "--cap-drop", "ALL", "--cap-add", "NET_BIND_SERVICE",
            "-e", "PORTER_ADMIN_USERNAME=smoke-admin",
            "-e", "PORTER_ADMIN_PASSWORD=smoke-only-long-password",
            "-e", f"PORTER_HTTP_ADDR=127.0.0.1:{http_port}",
            "-e", "PORTER_TRAFFIC_FLUSH_INTERVAL=100ms",
            "-e", "PORTER_CONTROL_SOCKET=/data/smoke-control.sock",
            "-v", f"{volume}:/data", image,
        )

    try:
        docker("volume", "create", volume)
        start()
        panel = Panel(http_port)
        panel.ready()
        verify_native_control_disabled(name)
        html = panel.request("GET", "/")
        assert "<html" in html.lower(), "Embedded frontend page was not returned"
        asset = re.search(r'src="([^\"]+\.js)"', html)
        assert asset, "Frontend script is missing from the page"
        assert panel.request("GET", asset.group(1)), "Frontend script is empty"
        panel.request("GET", "/api/admin/rules", status=401)
        panel.login()
        # 宿主测试用户可能无低端口权限，由拥有 capability 的容器尝试实际绑定。
        candidates = list(range(900, 1024))
        random.shuffle(candidates)
        for listen_port in candidates:
            created = []
            for protocol, target in [
                ("tcp", echoes.tcp.getsockname()[1]),
                ("udp", echoes.udp.getsockname()[1]),
            ]:
                rule = panel.request(
                    "POST", "/api/admin/rules",
                    {
                        "name": f"Container smoke test {protocol}", "protocol": protocol,
                        "listen_host": "127.0.0.1", "listen_port": listen_port,
                        "target_host": "127.0.0.1", "target_port": target,
                        "enabled": True,
                    }, status=(201, 409),
                )
                if "error" in rule:
                    break
                created.append(rule)
            if len(created) == 2:
                break
            for rule in created:
                panel.request(
                    "DELETE", f'/api/admin/rules/{rule["id"]}',
                    revision=rule["revision"], status=204,
                )
        else:
            raise RuntimeError("No free TCP/UDP low port is available for the container smoke test")
        verify_traffic(listen_port)
        docker("stop", "--time", "10", name)
        docker("rm", name)
        start()
        panel = Panel(http_port)
        panel.ready()
        verify_native_control_disabled(name)
        panel.login()
        rules = panel.request("GET", "/api/admin/rules")["rules"]
        assert len(rules) == 2 and all(
            rule["status"] == "running" for rule in rules
        ), "Rules were not restored after container recreation"
        verify_traffic(listen_port)
        for rule in rules:
            panel.request(
                "DELETE", f'/api/admin/rules/{rule["id"]}',
                revision=rule["revision"], status=204,
            )
        assert panel.request("GET", "/api/admin/rules")["rules"] == []
        verify_stopped(listen_port)

        combined = panel.request(
            "POST", "/api/admin/rules",
            {
                "name": "Container combined smoke test", "protocol": "tcp+udp",
                "listen_host": "127.0.0.1", "listen_port": listen_port,
                "target_host": "127.0.0.1", "target_port": echoes.tcp.getsockname()[1],
                "enabled": True,
            }, status=201,
        )
        assert panel.request("GET", "/api/admin/rules")["rules"] == [combined], "A combined rule must be stored as one record"
        verify_traffic(listen_port)
        docker("stop", "--time", "10", name)
        docker("rm", name)
        start()
        panel = Panel(http_port)
        panel.ready()
        verify_native_control_disabled(name)
        panel.login()
        rules = panel.request("GET", "/api/admin/rules")["rules"]
        assert len(rules) == 1, "Combined rule was split into multiple records after recreation"
        restored = rules[0]
        assert restored["id"] == combined["id"] and restored["revision"] == combined["revision"]
        assert restored["protocol"] == "tcp+udp" and restored["status"] == "running"
        verify_traffic(listen_port)
        for enabled in (False, True):
            restored = panel.request(
                "PUT", f'/api/admin/rules/{restored["id"]}',
                rule_input(restored, enabled=enabled), revision=restored["revision"],
            )
            if enabled:
                verify_traffic(listen_port)
            else:
                assert restored["status"] == "disabled"
                verify_stopped(listen_port)
        restored = panel.request(
            "PUT", f'/api/admin/rules/{restored["id"]}',
            rule_input(restored, protocol="tcp"), revision=restored["revision"],
        )
        verify_tcp(listen_port)
        verify_stopped(listen_port, ("udp",))
        restored = panel.request(
            "PUT", f'/api/admin/rules/{restored["id"]}',
            rule_input(restored, protocol="tcp+udp"), revision=restored["revision"],
        )
        verify_traffic(listen_port)
        panel.request("DELETE", f'/api/admin/rules/{restored["id"]}', revision=restored["revision"], status=204)
        assert panel.request("GET", "/api/admin/rules")["rules"] == []
        verify_stopped(listen_port)
        # 删除组合规则后重新绑定两类单协议，确认两个低端口监听均已释放。
        for protocol in ("tcp", "udp"):
            rebound = panel.request(
                "POST", "/api/admin/rules", rule_input(restored, protocol=protocol), status=201,
            )
            if protocol == "tcp":
                verify_tcp(listen_port)
            else:
                verify_udp(listen_port)
            panel.request("DELETE", f'/api/admin/rules/{rebound["id"]}', revision=rebound["revision"], status=204)
        # 两端独立会话、用户归属、端口限制和额度耗尽均通过真实容器验证。
        for _ in range(100):
            pool_start = random.randint(20000, 60000)
            held = []
            try:
                for port in range(pool_start, pool_start + 6):
                    for kind in (socket.SOCK_STREAM, socket.SOCK_DGRAM):
                        sock = socket.socket(type=kind)
                        held.append(sock)
                        sock.bind(("127.0.0.1", port))
                break
            except OSError:
                continue
            finally:
                for sock in held:
                    sock.close()
        else:
            raise RuntimeError("No contiguous port segment is available")
        settings = panel.request("GET", "/api/admin/settings")
        panel.request("PUT", "/api/admin/settings", {
            "port_start": pool_start, "port_end": pool_start + 5,
        }, revision=settings["revision"])
        plan = panel.request("POST", "/api/admin/plans", {
            "name": "Smoke monthly max", "port_count": 2,
            "traffic_limit_bytes": "1048576", "accounting_mode": "max",
            "reset_mode": "fixed_day", "reset_day": 31,
        }, status=201)
        accounts = []
        for username in ("smoke-alice", "smoke-bob"):
            accounts.append(panel.request("POST", "/api/admin/users", {
                "username": username, "password": "smoke-user-long-password",
                "plan_id": plan["id"],
            }, status=201))
        alice, bob = [account["user"] for account in accounts]
        assert alice["port_end"] < bob["port_start"], "User allocations overlap"
        user = Panel(http_port, "user")
        user.client = panel.client
        user.login("smoke-alice", "smoke-user-long-password")
        isolated_user = Panel(http_port, "user")
        isolated_user.login("smoke-alice", "smoke-user-long-password")
        isolated_user.request("GET", "/api/admin/users", status=(401, 403))
        user.request("GET", "/api/rules", status=404)
        managed = []
        for owner in (alice, bob):
            managed.append(panel.request("POST", "/api/admin/rules", {
                "owner_id": owner["id"], "name": owner["username"],
                "protocol": "tcp+udp", "listen_host": "127.0.0.1",
                "listen_port": owner["port_start"], "target_host": "127.0.0.1",
                "target_port": echoes.tcp.getsockname()[1], "enabled": True,
                "traffic_enabled": False,
            }, status=201))
            verify_traffic(owner["port_start"])
        all_rules = panel.request("GET", "/api/admin/rules")["rules"]
        assert {rule["owner"]["id"] for rule in all_rules} == {alice["id"], bob["id"]}
        filtered = panel.request("GET", f'/api/admin/rules?owner_id={alice["id"]}')["rules"]
        assert len(filtered) == 1 and filtered[0]["owner"]["username"] == "smoke-alice"
        own_rules = user.request("GET", "/api/user/rules")["rules"]
        assert [rule["id"] for rule in own_rules] == [managed[0]["id"]]
        user.request("PUT", f'/api/user/rules/{managed[1]["id"]}',
                     rule_input(managed[1]), revision=managed[1]["revision"], status=404)
        user.request("POST", "/api/user/rules", rule_input(managed[0],
                     listen_port=bob["port_start"]), status=(400, 403, 409))
        stopped = panel.request("PUT", f'/api/admin/rules/{managed[0]["id"]}',
                                rule_input(managed[0], enabled=False), revision=managed[0]["revision"])
        verify_stopped(alice["port_start"])
        verify_traffic(bob["port_start"])
        managed[0] = user.request("PUT", f'/api/user/rules/{stopped["id"]}',
                                 rule_input(stopped, enabled=True), revision=stopped["revision"])
        verify_traffic(alice["port_start"])
        panel.request("POST", "/api/admin/auth/logout", status=204)
        user.request("GET", "/api/user/auth/session")
        panel.request("GET", "/api/admin/rules", status=401)
        panel.login()
        user.request("POST", "/api/user/auth/logout", status=204)
        panel.request("GET", "/api/admin/rules")
        user.request("GET", "/api/user/rules", status=401)
        user.login("smoke-alice", "smoke-user-long-password")
        usage = user.request("GET", "/api/user/account")["usage"]
        assert int(usage["used_bytes"]) > 0, "Quota accounting was disabled by display statistics"
        docker("stop", "--time", "10", name)
        docker("rm", name)
        start()
        panel = Panel(http_port)
        panel.ready()
        verify_native_control_disabled(name)
        panel.login()
        user.request("GET", "/api/user/auth/session", status=401)
        user.login("smoke-alice", "smoke-user-long-password")
        recovered = user.request("GET", "/api/user/account")["usage"]
        assert int(recovered["used_bytes"]) >= int(usage["used_bytes"]), "User quota usage was lost on normal restart"
        verify_traffic(alice["port_start"])
        verify_traffic(bob["port_start"])
        tiny = panel.request("POST", "/api/admin/plans", {
            "name": "Smoke tiny quota", "port_count": 1, "traffic_limit_bytes": "16",
            "accounting_mode": "both", "reset_mode": "calendar_month", "reset_day": None,
        }, status=201)
        limited = panel.request("POST", "/api/admin/users", {
            "username": "smoke-limited", "password": "smoke-user-long-password", "plan_id": tiny["id"],
        }, status=201)["user"]
        small_rule = panel.request("POST", "/api/admin/rules", {
            "owner_id": limited["id"], "name": "Smoke exhaust", "protocol": "tcp",
            "listen_host": "127.0.0.1", "listen_port": limited["port_start"],
            "target_host": "127.0.0.1", "target_port": echoes.tcp.getsockname()[1], "enabled": True,
        }, status=201)
        with socket.create_connection(("127.0.0.1", limited["port_start"]), timeout=5) as client:
            client.sendall(b"x" * 64)
            try:
                client.recv(64)
            except OSError:
                pass
        for _ in range(50):
            current = panel.request("GET", f'/api/admin/rules?owner_id={limited["id"]}')["rules"][0]
            if current["status"] == "quota_exceeded":
                break
            time.sleep(0.05)
        else:
            raise AssertionError("Quota exhaustion did not stop the user rule")
        for attempt in range(50):
            try:
                verify_stopped(limited["port_start"], ("tcp",))
                break
            except AssertionError:
                if attempt == 49:
                    raise
                time.sleep(0.05)
        panel.request("POST", f'/api/admin/rules/{small_rule["id"]}/retry',
                      revision=small_rule["revision"], status=409)
        verify_traffic(bob["port_start"])
        print("Container smoke test passed: native CLI and control socket disabled, authentication, low ports, real single-protocol and combined TCP+UDP traffic, recreation recovery, combined enable/disable and protocol changes, listener release after deletion, independent portals, owner filtering, user isolation, persisted usage, and real quota exhaustion")
    except Exception:
        logs = docker("logs", name, check=False)
        print(logs.stdout + logs.stderr, file=sys.stderr)
        raise
    finally:
        docker("rm", "-f", name, check=False)
        docker("volume", "rm", volume, check=False)
        echoes.close()


if __name__ == "__main__":
    main()
