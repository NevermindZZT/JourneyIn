$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

pnpm --dir web install --frozen-lockfile
pnpm --dir web typecheck
pnpm --dir web build

$env:CGO_ENABLED = "0"
go test ./...
go vet ./...
New-Item -ItemType Directory -Force dist | Out-Null
go build -trimpath -ldflags "-s -w -X main.version=0.2.2" -o dist/journeyin.exe ./cmd/journeyin
Write-Host "Built dist/journeyin.exe"
