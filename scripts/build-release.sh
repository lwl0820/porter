#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
version="${VERSION:-dev}"
if [[ ! "$version" =~ ^[a-zA-Z0-9._-]+$ ]]; then
  echo 'Version must contain only letters, digits, dots, underscores, and hyphens' >&2
  exit 1
fi
export TMPDIR="$PWD/.tmp/tmp" GOTMPDIR="$PWD/.tmp/tmp"
export GOCACHE="$PWD/.tmp/go-cache" GOPATH="$PWD/.tmp/go-path" GOMODCACHE="$PWD/.tmp/go-mod"
export npm_config_cache="$PWD/.tmp/npm-cache"
mkdir -p "$TMPDIR" .tmp/release
stage=""
trap 'if [[ -n "$stage" ]]; then rm -rf -- "$stage"; fi' EXIT
npm --prefix web ci
npm --prefix web run build
for arch in amd64 arm64; do
  stage="$(mktemp -d "$PWD/.tmp/package-$arch.XXXXXXXX")"
  for program in porter porterd; do
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath \
      -ldflags "-s -w -X main.version=$version" -o "$stage/$program" "./cmd/$program"
  done
  cp README.md README.zh-CN.md "$stage/"
  tar -czf ".tmp/release/porter-${version}-linux-${arch}.tar.gz" -C "$stage" porter porterd README.md README.zh-CN.md
  rm -rf -- "$stage"
  stage=""
done
cd .tmp/release
sha256sum "porter-${version}-linux-amd64.tar.gz" "porter-${version}-linux-arm64.tar.gz" > SHA256SUMS
