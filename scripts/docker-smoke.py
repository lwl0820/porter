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
            raise RuntimeError("无法为 TCP/UDP 回显服务分配相同数字端口")
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
    def __init__(self, port):
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
                    f"{method} {path}: 期望 {status}，实际 {response.status}: "
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
        raise RuntimeError(f"容器未就绪：{last_error}")

    def login(self):
        session = self.request(
            "POST", "/api/auth/login",
            {"username": "smoke-admin", "password": "smoke-only-long-password"},
        )
        self.csrf = session["csrf_token"]
        assert session["username"] == "smoke-admin"


def verify_tcp(port):
    payload = b"porter-container-tcp\x00"
    with socket.create_connection(("127.0.0.1", port), timeout=5) as client:
        client.sendall(payload)
        client.shutdown(socket.SHUT_WR)
        result = bytearray()
        while data := client.recv(65536):
            result.extend(data)
        assert result == payload, "TCP 半关闭后响应不完整"


def verify_udp(port):
    with socket.socket(type=socket.SOCK_DGRAM) as first, socket.socket(
        type=socket.SOCK_DGRAM
    ) as second:
        first.settimeout(5)
        second.settimeout(5)
        first.sendto(b"first", ("127.0.0.1", port))
        second.sendto(b"second", ("127.0.0.1", port))
        assert first.recv(65536) == b"first", "UDP 客户端响应错配"
        assert second.recv(65536) == b"second", "UDP 客户端响应错配"
        first.sendto(b"", ("127.0.0.1", port))
        assert first.recv(65536) == b"", "UDP 零长度报文丢失"


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
            raise AssertionError("规则停止后 TCP 仍可连接")
    if "udp" in protocols:
        with socket.socket(type=socket.SOCK_DGRAM) as client:
            client.settimeout(0.3)
            client.sendto(b"should-not-echo", ("127.0.0.1", port))
            try:
                client.recv(65536)
            except socket.timeout:
                pass
            else:
                raise AssertionError("规则停止后 UDP 仍在转发")


def rule_input(rule, **changes):
    fields = ("name", "protocol", "listen_host", "listen_port", "target_host", "target_port", "enabled")
    return {**{key: rule[key] for key in fields}, **changes}


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
            "-v", f"{volume}:/data", image,
        )

    try:
        docker("volume", "create", volume)
        start()
        panel = Panel(http_port)
        panel.ready()
        html = panel.request("GET", "/")
        assert "<html" in html.lower(), "未返回内嵌前端页面"
        asset = re.search(r'src="([^\"]+\.js)"', html)
        assert asset, "页面缺少前端脚本"
        assert panel.request("GET", asset.group(1)), "前端脚本为空"
        panel.request("GET", "/api/rules", status=401)
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
                    "POST", "/api/rules",
                    {
                        "name": f"容器验证 {protocol}", "protocol": protocol,
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
                    "DELETE", f'/api/rules/{rule["id"]}',
                    revision=rule["revision"], status=204,
                )
        else:
            raise RuntimeError("容器没有可用于低端口验证的空闲 TCP/UDP 端口")
        verify_traffic(listen_port)
        docker("stop", "--time", "10", name)
        docker("rm", name)
        start()
        panel = Panel(http_port)
        panel.ready()
        panel.login()
        rules = panel.request("GET", "/api/rules")["rules"]
        assert len(rules) == 2 and all(
            rule["status"] == "running" for rule in rules
        ), "容器重建后规则未恢复"
        verify_traffic(listen_port)
        for rule in rules:
            panel.request(
                "DELETE", f'/api/rules/{rule["id"]}',
                revision=rule["revision"], status=204,
            )
        assert panel.request("GET", "/api/rules")["rules"] == []
        verify_stopped(listen_port)

        combined = panel.request(
            "POST", "/api/rules",
            {
                "name": "容器组合验证", "protocol": "tcp+udp",
                "listen_host": "127.0.0.1", "listen_port": listen_port,
                "target_host": "127.0.0.1", "target_port": echoes.tcp.getsockname()[1],
                "enabled": True,
            }, status=201,
        )
        assert panel.request("GET", "/api/rules")["rules"] == [combined], "组合规则必须只保存一条"
        verify_traffic(listen_port)
        docker("stop", "--time", "10", name)
        docker("rm", name)
        start()
        panel = Panel(http_port)
        panel.ready()
        panel.login()
        rules = panel.request("GET", "/api/rules")["rules"]
        assert len(rules) == 1, "组合规则重建后被拆成多条"
        restored = rules[0]
        assert restored["id"] == combined["id"] and restored["revision"] == combined["revision"]
        assert restored["protocol"] == "tcp+udp" and restored["status"] == "running"
        verify_traffic(listen_port)
        for enabled in (False, True):
            restored = panel.request(
                "PUT", f'/api/rules/{restored["id"]}',
                rule_input(restored, enabled=enabled), revision=restored["revision"],
            )
            if enabled:
                verify_traffic(listen_port)
            else:
                assert restored["status"] == "disabled"
                verify_stopped(listen_port)
        restored = panel.request(
            "PUT", f'/api/rules/{restored["id"]}',
            rule_input(restored, protocol="tcp"), revision=restored["revision"],
        )
        verify_tcp(listen_port)
        verify_stopped(listen_port, ("udp",))
        restored = panel.request(
            "PUT", f'/api/rules/{restored["id"]}',
            rule_input(restored, protocol="tcp+udp"), revision=restored["revision"],
        )
        verify_traffic(listen_port)
        panel.request("DELETE", f'/api/rules/{restored["id"]}', revision=restored["revision"], status=204)
        assert panel.request("GET", "/api/rules")["rules"] == []
        verify_stopped(listen_port)
        # 删除组合规则后重新绑定两类单协议，确认两个低端口监听均已释放。
        for protocol in ("tcp", "udp"):
            rebound = panel.request(
                "POST", "/api/rules", rule_input(restored, protocol=protocol), status=201,
            )
            if protocol == "tcp":
                verify_tcp(listen_port)
            else:
                verify_udp(listen_port)
            panel.request("DELETE", f'/api/rules/{rebound["id"]}', revision=rebound["revision"], status=204)
        print("容器验证通过：认证、低端口、单协议与单条 TCP+UDP 真实收发、重建恢复、组合启停/切换及删除释放")
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
