# Build and qualification
Go 1.23+, Python 3, GCC/Clang, make, binutils, static libc/libaio development libraries and usual Linux headers are build-machine prerequisites. The target machine needs none of these.

Pinned fio 3.39, stress-ng 0.18.10 and iperf3 3.17.1 upstream archives were downloaded on this Windows development host. Their SHA-256 hashes are in `scripts/engines.lock.json`, and upstream license files in `LICENSES/` were extracted from those archives. Hashes record the fetched inputs; they are not upstream-signed provenance evidence.

On a networked build machine, run `python3 scripts/fetch-engines.py`. Offline build from cached archives:

```
python3 scripts/fetch-engines.py --offline
bash scripts/build-engines.sh
bash scripts/build.sh
bash scripts/smoke-linux.sh
```

These third-party build scripts have not been executed on Linux here. fio requires static libaio; io_uring is compiled by fio's native kernel interface where supported. Verify `fio --enghelp` and actual direct-I/O test success on each target. stress-ng enables features according to build dependencies; record its build log. iperf3 OpenSSL authentication is disabled; use only trusted benchmark networks. All transitive static library licenses must also be reviewed and included. The supplied top-level license bundle is not a completed redistribution audit.

The packaging gate rejects dynamically linked engines. The native kernel can alternatively be cross-built with Zig 0.13.0: `zig cc -target x86_64-linux-musl -O2 -fno-fast-math -pthread -static native/compute-bench.c -o dist/vmbench/engines/compute-bench`. This cross-compilation was executed successfully on Windows. The Go Linux executable was also cross-compiled and later executed with fio 3.39 in a disposable Alpine guest; see [disk validation](DISK_VALIDATION.md). Native benchmark execution and Ubuntu qualification remain untested.

Run the entire package on Ubuntu 22.04 and 24.04 x86_64 with networking disabled except loopback smoke tests. ELF validation does not establish kernel compatibility. A complete release archive is created only after all engine prerequisites are present and runnable. Current artifacts are development binaries/source, not a qualified offline release.

## Automated host setup

For Ubuntu source development, use `./start.sh` (or `./init.sh --yes` for explicitly authorized noninteractive apt installation). See [BOOTSTRAP.md](BOOTSTRAP.md). The bootstrap defaults to host-linked system engines; this differs from the static release process above. `./init.sh --engine-mode bundled` reuses these source-build scripts and enforces static binaries. Release archives now include init.sh/start.sh and the runtime bootstrap helper, so offline prebuilt validation needs no compiler.
