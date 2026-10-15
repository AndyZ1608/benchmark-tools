# Offline builds and deployment

There are two separate models: a source checkout with locally supplied build dependencies (`bash init.sh --offline`), and a complete binary archive that requires no build tools (`./vmbench doctor; ./vmbench quick`). A development snapshot missing engines is not the second model.

## Explicit static targets

Release targets are linux-{amd64,arm64}-{musl,glibc}-static. Build each on matching Linux architecture/libc. ARM64 is experimental until real architecture execution is recorded. Static linkage avoids assuming a glibc minimum or mixing glibc/musl shared loaders; it does not remove kernel/ISA/filesystem limitations. No global LD_LIBRARY_PATH is set. Dynamic host-installed engines are valid for source development only, not accepted by portable packaging.

Build prerequisites: compatible Go selected by the common bootstrap, GCC/Clang, make, Python3, binutils, Linux headers, libc/static runtime and static libaio. Additional optional stress-ng libraries depend on feature probes. Alpine 3.22 libaio-dev contains libaio.a; there is no separate libaio-static package in the tested repositories. Minimal hosts need Bash/coreutils/findutils. Keep compiler output/build configuration for the transitive audit.

```bash
python3 scripts/fetch-engines.py                 # approved networked build machine
python3 scripts/fetch-engines.py --offline       # verifies cached sources
python3 scripts/fetch-runtime-sources.py --offline # recorded Alpine runtime notices/sources
bash scripts/build-engines.sh                    # offline compilation
bash scripts/build.sh --candidate                # local qualification only
```

Engine inputs are pinned fio 3.39, stress-ng 0.18.10 and iperf3 3.17.1. Fetching verifies SHA-256 and stops on mismatch. It does not establish upstream signature provenance. fio is built static without native CPU tuning and requires libaio support; stress-ng is static with detected features and baseline target flags (upstream may append optimization flags); iperf3 is static without OpenSSL authentication. Use trusted benchmark networks. Baseline native targets are x86-64 or ARMv8-A, never march=native. Native floating-point contraction is disabled for consistent calculation semantics.

Build output is staged under `dist/ARCH-LIBC/vmbench`. `scripts/release.py` verifies ELF64 machine, rejects PT_INTERP/DT_NEEDED, executes version/doctor checks, verifies source archives/licenses and exact engine build hashes, and writes release-manifest.json plus archive SHA-256. `LICENSES/engine-build.json` records architecture/libc/compiler/flags/hashes; feature/configuration files accompany engine sources. No packaging step downloads dependencies.

## Redistribution gate

Candidate packages are unqualified local test artifacts, not redistribution-ready. Producing an ordinary archive requires:

```bash
bash scripts/build.sh --audit /absolute/path/approved-audit.json
```

The audit is a documented source/license/build review record, not an assertion automatically inferred from static ELF. It must approve the exact target and contain components for fio, stress-ng, iperf3, libc, compiler-runtime, go-runtime, libaio and every additional linked library. Each entry records `license`, `source`, `reviewer`, and its bundled `artifacts` (relative files beside the audit). Example schema (replace each field with reviewed real evidence):

```json
{"target":"linux-amd64-musl-static","approved":true,"components":{"libc":{"license":"actual license","source":"corresponding source location","reviewer":"responsible reviewer","artifacts":["actual-source-archive","actual-notice-file"]}}}
```

This short schema example deliberately lacks other required components and will fail the gate. Preserve legal notices, corresponding source/build instructions, applicable relinking requirements and compiler-runtime exceptions; auditing optional static dependencies remains the builder's responsibility. Do not fabricate approval. The gate checks structure/artifacts, not the legal correctness of a reviewer decision. Recorded Alpine build runtime inputs include exact package revisions, immutable Alpine build recipes/patches, verified libaio/musl/zlib source archives, and GCC runtime notices/source references. The records do not automatically approve another host or compiler/library revision. A source/license review must match the actual linked closure and binary hashes. The required `binary_sha256` map must contain exactly the five packaged executable paths and their SHA-256 values; a mismatching record is rejected. The [recorded Alpine review](RELEASE-AUDIT.md) documents this session's link-map inventory and included source/notices; it does not cover other targets.

## Offline target validation

Transfer a matching archive and checksum. Verify its checksum, extract, then run `./vmbench doctor` and bounded workloads in disposable storage. Direct binary execution needs no Go/GCC/git/Python/package manager. Shell startup is optional and still needs shell base utilities.

A proper qualification test disables network access and hides/removes development tools before invoking the bundle. Distinguish safe runtime inspection, real CPU/RAM workloads, isolated fio, and remote-network testing. A static build plus version queries on the build host do not qualify all distributions. See [evidence](COMPATIBILITY.md).

The recorded 2026-10-14 Alpine amd64 musl archive passed all three bounded modes in a fresh diskless qemu64 guest without development packages or external networking. CPU/RAM/Disk all completed; private dataset cleanup and sentinel preservation passed. Loopback iperf and a verified one-second stress-ng worker also passed. See [the phase evidence](validation/alpine-3.22.6-amd64-2026-10-14.json); other distributions/architectures remain unqualified.
