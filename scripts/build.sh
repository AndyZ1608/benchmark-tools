#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd -P)"
cd "$ROOT"
source "$ROOT/scripts/bootstrap-common.sh"
mkdir -p dist/vmbench/engines dist/vmbench/config dist/vmbench/LICENSES
version_flags=$(build_version_ldflags)
GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$version_flags" -o dist/vmbench/vmbench ./cmd/vmbench
compile_native_kernel "$ROOT/dist/vmbench/engines/compute-bench" static cc
cp config/default.yaml dist/vmbench/config/
cp README.md VERSION LICENSE init.sh start.sh dist/vmbench/
mkdir -p dist/vmbench/scripts dist/vmbench/docs
cp docs/BUILD.md docs/BOOTSTRAP.md docs/PRESENTATION.md docs/RESULT_EXAMPLES.md docs/DISK.md docs/DISK_VALIDATION.md dist/vmbench/docs/
cp scripts/bootstrap-common.sh scripts/go-toolchain.lock dist/vmbench/scripts/
chmod +x dist/vmbench/init.sh dist/vmbench/start.sh
# Third-party static binaries must be built from the pinned sources recorded below.
for engine in fio stress-ng iperf3; do
  test -x "vendor-engines/$engine" || { echo "Missing vendor-engines/$engine; see docs/BUILD.md" >&2; exit 1; }
  cp "vendor-engines/$engine" dist/vmbench/engines/
done
cp -R vendor-engines/LICENSES/. dist/vmbench/LICENSES/
python3 scripts/verify-release.py dist/vmbench
(cd dist && tar -czf vmbench-linux-amd64.tar.gz vmbench && sha256sum vmbench-linux-amd64.tar.gz > vmbench-linux-amd64.tar.gz.sha256)
