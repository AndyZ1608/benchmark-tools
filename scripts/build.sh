#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p dist/vmbench/engines dist/vmbench/config dist/vmbench/LICENSES
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o dist/vmbench/vmbench ./cmd/vmbench
cc -O2 -fno-fast-math -pthread -static native/compute-bench.c -lm -o dist/vmbench/engines/compute-bench
cp config/default.yaml dist/vmbench/config/
cp README.md VERSION LICENSE dist/vmbench/
# Third-party static binaries must be built from the pinned sources recorded below.
for engine in fio stress-ng iperf3; do
  test -x "vendor-engines/$engine" || { echo "Missing vendor-engines/$engine; see docs/BUILD.md" >&2; exit 1; }
  cp "vendor-engines/$engine" dist/vmbench/engines/
done
cp -R vendor-engines/LICENSES/. dist/vmbench/LICENSES/
python3 scripts/verify-release.py dist/vmbench
(cd dist && tar -czf vmbench-linux-amd64.tar.gz vmbench && sha256sum vmbench-linux-amd64.tar.gz > vmbench-linux-amd64.tar.gz.sha256)
