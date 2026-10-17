#!/usr/bin/env python3
"""检查客户端、服务和 Docker 构建的真实 Go 依赖边界。"""

import json
import os
from pathlib import Path
import subprocess


ROOT = Path(__file__).resolve().parents[1]
ENV = {**os.environ, "GOOS": "linux", "CGO_ENABLED": "0"}


def go(*arguments):
    return subprocess.run(
        ["go", *arguments], cwd=ROOT, env=ENV, check=True,
        text=True, capture_output=True,
    ).stdout


def reject_dependencies(dependencies, forbidden, description):
    found = sorted(
        dependency for dependency in dependencies
        if any(dependency == path or dependency.startswith(path + "/") for path in forbidden)
    )
    if found:
        raise SystemExit(description + ": " + ", ".join(found))


def main():
    module = go("list", "-m").strip()
    client = go("list", "-deps", "./cmd/porter").splitlines()
    reject_dependencies(
        client,
        [module + "/internal/" + package for package in
         ("config", "store", "rules", "forward", "httpapi", "localapi")]
        + [module + "/web", "modernc.org/sqlite"],
        "Client contains service dependencies",
    )
    for tags in ([], ["-tags", "porter_docker"]):
        server = go("list", "-deps", *tags, "./cmd/porterd").splitlines()
        reject_dependencies(server, [module + "/internal/cli"], "Service contains CLI dependencies")
    control = json.loads(go("list", "-json", "-tags", "porter_docker", "./internal/localapi"))
    if "server_linux.go" in control["GoFiles"] or "server_other.go" not in control["GoFiles"]:
        raise SystemExit("Docker must compile the local-control stub, not the native listener")
    print("Client, service and Docker dependency boundaries verified")


if __name__ == "__main__":
    main()
