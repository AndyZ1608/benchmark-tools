#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT"
source "$ROOT/scripts/bootstrap-common.sh"
detect_linux
TARGET_ARCH=${TARGET_ARCH:-$GO_ARCH}; TARGET_LIBC=${TARGET_LIBC:-$LIBC}
[[ $TARGET_ARCH == "$GO_ARCH" && $TARGET_LIBC == "$LIBC" ]] || { fail 'Engine build requires its target architecture/libc; use a matching build VM'; exit 1; }
CC="$(find_compiler)"
export CC
check_compiler_target "$CC"
# Conservative baseline ISA; never -march=native.
case $GO_ARCH in amd64) export CFLAGS='-O2 -march=x86-64 -mtune=generic';; arm64) export CFLAGS='-O2 -march=armv8-a';; esac
export LC_ALL=C

python3 scripts/fetch-engines.py --offline
context=$(printf '%s\n' "$GO_ARCH" "$LIBC" "$LIBC_VERSION" "$CFLAGS" "$("$CC" --version)" "$(hash_file scripts/engines.lock.json)" "$(hash_file scripts/build-engines.sh)" | sha256sum | cut -c1-16)
BUILD_ROOT="$ROOT/.build/engines/$GO_ARCH-$LIBC-$context"
mkdir -p "$BUILD_ROOT" vendor-engines/LICENSES
for name in fio stress-ng iperf3; do
  mkdir -p "$BUILD_ROOT/$name"
  tar --extract --gzip --file="third-party/$name.tar.gz" --directory="$BUILD_ROOT/$name" --strip-components=1
 done
(
 cd "$BUILD_ROOT/fio"
 ./configure --build-static --disable-native
 grep -q 'CONFIG_LIBAIO=y' config-host.mak || { echo 'Static libaio development dependency missing' >&2; exit 1; }
 make -j"${BUILD_JOBS:-2}"
)
(
 cd "$BUILD_ROOT/stress-ng"
 make -j"${BUILD_JOBS:-2}" STATIC=1
)
(
 cd "$BUILD_ROOT/iperf3"
 ./configure --enable-static-bin --disable-shared --without-openssl
 make -j"${BUILD_JOBS:-2}"
)
cp "$BUILD_ROOT/fio/fio" vendor-engines/fio
cp "$BUILD_ROOT/stress-ng/stress-ng" vendor-engines/stress-ng
cp "$BUILD_ROOT/iperf3/src/iperf3" vendor-engines/iperf3
cp -R LICENSES/. vendor-engines/LICENSES/
# Preserve source and build settings to meet upstream source redistribution obligations.
cp scripts/engines.lock.json vendor-engines/LICENSES/
cp third-party/*.tar.gz vendor-engines/LICENSES/
# Capture exactly which binaries and build inputs were used, including feature detection.
cp "$BUILD_ROOT/fio/config-host.mak" vendor-engines/LICENSES/fio-build-config.txt
cp "$BUILD_ROOT/stress-ng/config" vendor-engines/LICENSES/stress-ng-build-config.txt
cp "$BUILD_ROOT/iperf3/config.log" vendor-engines/LICENSES/iperf3-config.log
export TARGET_ARCH TARGET_LIBC
python3 - <<'PY'
import hashlib,json,os,pathlib,subprocess
root=pathlib.Path('vendor-engines')
manifest={'arch':os.environ['TARGET_ARCH'],'libc':os.environ['TARGET_LIBC'],'cc':subprocess.check_output([os.environ['CC'],'--version'],text=True).splitlines()[0], 'cflags':os.environ['CFLAGS'],'linkage':'static', 'sha256':{name:hashlib.sha256((root/name).read_bytes()).hexdigest() for name in ['fio','stress-ng','iperf3']}}
(root/'LICENSES/engine-build.json').write_text(json.dumps(manifest,indent=2)+'\n')
PY
