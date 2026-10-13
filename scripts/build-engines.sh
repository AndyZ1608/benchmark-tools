#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
python3 scripts/fetch-engines.py --offline
mkdir -p .build/engines vendor-engines/LICENSES
for name in fio stress-ng iperf3; do
  mkdir -p ".build/engines/$name"
  tar --extract --gzip --file="third-party/$name.tar.gz" --directory=".build/engines/$name" --strip-components=1
 done
(
 cd .build/engines/fio
 ./configure --build-static --disable-native
 grep -q 'CONFIG_LIBAIO=y' config-host.mak || { echo 'Static libaio development dependency missing' >&2; exit 1; }
 make -j"${BUILD_JOBS:-2}"
)
(
 cd .build/engines/stress-ng
 make -j"${BUILD_JOBS:-2}" STATIC=1
)
(
 cd .build/engines/iperf3
 ./configure --enable-static-bin --disable-shared --without-openssl
 make -j"${BUILD_JOBS:-2}"
)
cp .build/engines/fio/fio vendor-engines/fio
cp .build/engines/stress-ng/stress-ng vendor-engines/stress-ng
cp .build/engines/iperf3/src/iperf3 vendor-engines/iperf3
cp -R LICENSES/. vendor-engines/LICENSES/
# Preserve source and build settings to meet upstream source redistribution obligations.
cp scripts/engines.lock.json vendor-engines/LICENSES/
cp third-party/*.tar.gz vendor-engines/LICENSES/
