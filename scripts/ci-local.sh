#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> Formatting Go code"
find . -name '*.go' -print0 | xargs -0 gofmt -w

echo "==> Running all tests"
go test ./...

echo "==> Building all packages"
go build ./...

echo "==> Local CI checks passed"
