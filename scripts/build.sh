#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
# Source the same toolchain selector used by init.sh; main is guarded.
source "$ROOT/init.sh"
detect_linux
TARGET_ARCH=${TARGET_ARCH:-$GO_ARCH}; TARGET_LIBC=${TARGET_LIBC:-$LIBC}
[[ $TARGET_ARCH == "$GO_ARCH" && $TARGET_LIBC == "$LIBC" ]] || { fail 'Release build must run on its target architecture/libc. Cross builds are not runtime qualification.'; exit 1; }
[[ $TARGET_LIBC == musl || $TARGET_LIBC == glibc ]] || { fail 'Cannot identify target libc'; exit 1; }
OFFLINE=1 BUILD_CLI=1
ensure_go_toolchain
check_compiler_target "$(find_compiler)"
output="$ROOT/dist/$TARGET_ARCH-$TARGET_LIBC/vmbench"
mkdir -p "$output/engines" "$output/config" "$output/LICENSES" "$output/scripts/install" "$output/scripts/go-telemetry" "$output/docs"
version_flags=$(build_version_ldflags)
(cd "$ROOT"; CGO_ENABLED=0 GOOS=linux GOARCH="$TARGET_ARCH" GOAMD64=v1 GOARM64=v8.0 go_local build -buildvcs=false -trimpath -ldflags "$version_flags" -o "$output/vmbench" ./cmd/vmbench)
compile_native_kernel "$output/engines/compute-bench" static "$(find_compiler)"
cp "$ROOT"/config/*.yaml "$output/config/"
cp "$ROOT"/README.md "$ROOT"/VERSION "$ROOT"/LICENSE "$ROOT"/init.sh "$ROOT"/start.sh "$output/"
cp "$ROOT"/docs/*.md "$ROOT"/docs/compatibility.json "$output/docs/"
if [[ -d $ROOT/docs/validation ]]; then cp -R "$ROOT/docs/validation" "$output/docs/"; fi
cp "$ROOT"/scripts/bootstrap-common.sh "$ROOT"/scripts/go-toolchain.lock "$output/scripts/"
cp "$ROOT"/scripts/install/*.sh "$output/scripts/install/"
cp "$ROOT/scripts/go-telemetry/mode" "$output/scripts/go-telemetry/"
chmod +x "$output/init.sh" "$output/start.sh"
for engine in fio stress-ng iperf3; do
    test -x "$ROOT/vendor-engines/$engine" || { fail "Missing vendor-engines/$engine; run scripts/build-engines.sh on this target first"; exit 1; }
    cp "$ROOT/vendor-engines/$engine" "$output/engines/"
done
cp -R "$ROOT/vendor-engines/LICENSES/." "$output/LICENSES/"
# Preserve the recorded build's runtime sources/notices. This never downloads.
if [[ -f $ROOT/scripts/runtime-sources.lock.json && -d $ROOT/third-party/runtime ]]; then
    python3 "$ROOT/scripts/fetch-runtime-sources.py" --offline
    mkdir -p "$output/LICENSES/runtime"
    cp -R "$ROOT/.vmbench/cache/runtime-sources/." "$output/LICENSES/runtime/"
fi
mkdir -p "$output/LICENSES/rebuild/scripts/install"
cp "$ROOT/scripts/build-engines.sh" "$ROOT/scripts/fetch-engines.py" "$ROOT/scripts/engines.lock.json" "$ROOT/scripts/bootstrap-common.sh" "$output/LICENSES/rebuild/scripts/"
cp "$ROOT/scripts/install/"*.sh "$output/LICENSES/rebuild/scripts/install/"
cp "$ROOT/scripts/runtime-sources.lock.json" "$ROOT/scripts/fetch-runtime-sources.py" "$output/LICENSES/rebuild/scripts/"
python3 - "$output/LICENSES/iperf3.tar.gz" "$output/LICENSES/iperf3/cJSON-LICENSE" <<'PY'
import sys,tarfile,pathlib
with tarfile.open(sys.argv[1]) as archive:
    member=next(m for m in archive.getmembers() if m.isfile() and m.name.endswith('/src/cjson.c'))
    header=archive.extractfile(member).read().split(b'*/',1)[0]+b'*/\n'
    pathlib.Path(sys.argv[2]).write_bytes(header)
PY
# Retain notices for the statically linked Go standard library and vendored code.
go_root=$(go_local env GOROOT)
mkdir -p "$output/LICENSES/Go"
cp "$go_root/LICENSE" "$go_root/PATENTS" "$output/LICENSES/Go/"
if [[ -d $go_root/src ]]; then
    while IFS= read -r -d '' notice; do
        relative=${notice#"$go_root/src/"}; mkdir -p "$output/LICENSES/Go/$(dirname "$relative")"
        cp "$notice" "$output/LICENSES/Go/$relative"
    done < <(find "$go_root/src" -type f \( -name LICENSE -o -name COPYING -o -name PATENTS -o -name NOTICE \) -print0)
fi
python3 "$ROOT/scripts/release.py" "$output" --arch "$TARGET_ARCH" --libc "$TARGET_LIBC" "$@"
