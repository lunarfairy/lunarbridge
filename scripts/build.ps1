param(
  [string]$Output = "dist"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$out = Join-Path $root $Output
New-Item -ItemType Directory -Force -Path $out | Out-Null

Push-Location $root
try {
  $env:CGO_ENABLED = "0"

  $env:GOOS = "windows"
  $env:GOARCH = "amd64"
  go build -trimpath -ldflags="-s -w" -o (Join-Path $out "lunarbridge-windows-amd64.exe") ./cmd/lunarbridge

  $env:GOOS = "darwin"
  $env:GOARCH = "amd64"
  go build -trimpath -ldflags="-s -w" -o (Join-Path $out "lunarbridge-darwin-amd64") ./cmd/lunarbridge

  $env:GOOS = "darwin"
  $env:GOARCH = "arm64"
  go build -trimpath -ldflags="-s -w" -o (Join-Path $out "lunarbridge-darwin-arm64") ./cmd/lunarbridge
} finally {
  Pop-Location
}
