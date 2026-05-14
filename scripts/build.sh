#!/usr/bin/env sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
OUT="${1:-dist}"
mkdir -p "$ROOT/$OUT"

cd "$ROOT"
export CGO_ENABLED=0

GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "$OUT/lunarbridge-windows-amd64.exe" ./cmd/lunarbridge
GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "$OUT/lunarbridge-darwin-amd64" ./cmd/lunarbridge
GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o "$OUT/lunarbridge-darwin-arm64" ./cmd/lunarbridge
