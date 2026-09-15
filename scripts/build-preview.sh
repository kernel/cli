#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
sha=$(git rev-parse HEAD)
version="0.0.0-preview.g${sha:0:12}"
date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
mkdir -p dist/preview
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

for os in linux darwin windows; do
  for arch in amd64 arm64; do
    binary=kernel
    if [ "$os" = windows ]; then binary=kernel.exe; fi
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
      -ldflags "-s -w -X main.version=$version -X main.commit=$sha -X main.date=$date" \
      -o "$work/$binary" ./cmd/kernel
    tar -czf "dist/preview/kernel_${version}_${os}_${arch}.tar.gz" -C "$work" "$binary"
  done
done
(cd dist/preview && sha256sum kernel_*.tar.gz > SHA256SUMS)
echo "Preview $version ($sha) built in dist/preview"
