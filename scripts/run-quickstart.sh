#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
CONFIG_PATH="${1:-$ROOT_DIR/examples/quickstart-rtsp.yaml}"

cd "$ROOT_DIR"

echo "==> Using config: $CONFIG_PATH"
echo "==> Starting RTSP quickstart"
echo "==> Open in VLC: rtsp://127.0.0.1:8554/test"

go run ./cmd/stream-source-tester -config "$CONFIG_PATH"
