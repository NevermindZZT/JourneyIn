$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
$version = (Get-Content -Raw -LiteralPath (Join-Path $root "VERSION")).Trim()
if ([string]::IsNullOrWhiteSpace($version)) { throw "VERSION must not be empty" }
if ($version -notmatch '^\d+\.\d+\.\d+$') { throw "VERSION must be semantic version (for example 1.2.3)" }

pnpm --dir web install --frozen-lockfile
if ($LASTEXITCODE -ne 0) { throw "Web dependency installation failed" }
pnpm --dir web typecheck
if ($LASTEXITCODE -ne 0) { throw "Web typecheck failed" }
pnpm --dir web build
if ($LASTEXITCODE -ne 0) { throw "Web build failed" }

$env:CGO_ENABLED = "0"
go test ./...
if ($LASTEXITCODE -ne 0) { throw "Go tests failed" }
go vet ./...
if ($LASTEXITCODE -ne 0) { throw "Go vet failed" }
New-Item -ItemType Directory -Force dist | Out-Null
go build -trimpath -ldflags "-s -w -X main.version=$version" -o dist/journeyin.exe ./cmd/journeyin
if ($LASTEXITCODE -ne 0) { throw "Go build failed" }
Write-Host "Built dist/journeyin.exe"
