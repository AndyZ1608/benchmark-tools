#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
go test ./...
go vet ./...
bench="$root/dist/vmbench/vmbench"
"$bench" version
"$bench" system info
"$bench" cpu test --preset quick --workers 2 --duration 1 --warmup 0
"$bench" ram test --preset quick --threads 2 --size 16MiB --duration 1 --warmup 0
"$bench" ram stress --preset quick --threads 2 --size 16MiB --duration 1 --warmup 0
"$bench" cpu stress --workers 2 --duration 1
scratch="$(mktemp -d)"
trap 'rmdir "$scratch"' EXIT
"$bench" disk test --path "$scratch" --preset quick --file-size 16MiB --duration 1 --warmup 0 --confirm-writes
"$bench" network server --bind 127.0.0.1 --port 55201 &
server=$!
trap 'kill "$server" 2>/dev/null || true; wait "$server" 2>/dev/null || true; rmdir "$scratch"' EXIT
sleep 1
"$bench" network test --target 127.0.0.1 --port 55201 --preset quick --duration 1 --warmup 0
