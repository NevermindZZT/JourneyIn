#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

pnpm --dir web install --frozen-lockfile
pnpm --dir web typecheck
pnpm --dir web build

export CGO_ENABLED=0
go test ./...
go vet ./...

VERSION="${VERSION:-dev}"
mkdir -p dist
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  IFS=/ read -r GOOS GOARCH <<< "$target"
  suffix=""
  [[ "$GOOS" == "windows" ]] && suffix=".exe"
  GOOS="$GOOS" GOARCH="$GOARCH" go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "dist/journeyin_${GOOS}_${GOARCH}${suffix}" ./cmd/journeyin
done
