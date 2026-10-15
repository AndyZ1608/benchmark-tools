# VMBench

Portable Linux VM Performance Benchmark CLI

VMBench measures CPU, memory, disk and network performance with three commands:

```bash
./vmbench quick
./vmbench standard
./vmbench max
```

Benchmark execution is offline: it uses local engines and optional trusted network peers. Source initialization can install missing dependencies with approval. A complete binary bundle needs neither a compiler nor Go. Current distribution qualification and release limitations are stated below; recognition by the installer does not certify a platform.

## Features

- Automatic effective CPU, affinity/quota, available memory, cache and network detection.
- QUICK screening, repeated STANDARD measurements, and bounded adaptive MAX discovery.
- Integer/floating-point throughput, concurrency scaling, STREAM-style RAM bandwidth and pointer-chase latency.
- fio direct asynchronous I/O, random IOPS, sequential throughput and latency, with owned temporary datasets.
- iperf3 TCP throughput with remote peers and controlled parallel streams; advanced UDP testing remains available.
- English, ASCII terminal summaries and optional details. No mandatory result files, database, GUI, or daemon.
- Distribution-aware source initialization, incremental startup, read-only diagnostics and explicit offline operation.
- Architecture/libc-specific static release tooling, checksums, engine provenance and a redistribution audit gate.

## Operating systems and qualification

The installation adapters below are implemented. The CI workflow attempts representative container installations; **adding a workflow does not mean those jobs have run**. Container results also do not qualify a full VM. See [compatibility evidence](docs/COMPATIBILITY.md) and [machine-readable status](docs/compatibility.json).

| Distribution | Version | Architecture | Installation status | Runtime validation status | Offline release status | Notes |
|---|---|---|---|---|---|---|
| Ubuntu | 22.04, 24.04 | amd64 / arm64 | Adapter tested with fixtures; real installation unverified | Unverified | Unverified | apt |
| Debian | 12, 13 | amd64 / arm64 | Adapter tested with fixtures; real installation unverified | Unverified | Unverified | apt |
| Rocky Linux, AlmaLinux | 9, 10 | amd64 / arm64 | Adapter tested with fixtures; real installation unverified | Unverified | Unverified | dnf; enabled repositories only |
| RHEL, CentOS Stream | 9, 10 | amd64 / arm64 | Adapter tested with fixtures; real installation unverified | Unverified | Unverified | RHEL requires valid repository access |
| Fedora | Image-selected release | amd64 / arm64 | Adapter tested with fixtures; real installation unverified | Unverified | Unverified | No lifecycle/support assertion for arbitrary releases |
| openSUSE Leap/Tumbleweed, SLES | Image-selected/installed version | amd64 / arm64 | Adapter tested with fixtures; real installation unverified | Unverified | Unverified | zypper; SLES repository access required |
| Arch Linux | Rolling | amd64 / arm64 | Adapter tested with fixtures; real installation unverified | Unverified | Unverified | Existing synchronized package metadata; no automatic OS upgrade |
| Alpine Linux | 3.22.6 | amd64 | Source initialization, real apk install and unchanged reuse verified | System/static engines; bounded QUICK/STANDARD/MAX CPU/RAM/Disk passed | Static bundle passed in fresh VM without network or development packages | QEMU functional tests; ext4 only; [evidence](docs/COMPATIBILITY.md) |

ARM64 is experimental: portable C and Go targets are implemented; native ARM64 execution is not verified. Unknown IDs and derivatives are rejected for automatic installation even if `ID_LIKE` names a known family. Windows is a development/test host only, not a benchmark target.

## Quick start

```bash
git clone https://github.com/AndyZ1608/benchmark-tools.git
cd benchmark-tools
./start.sh
```

Use `bash start.sh` if execute bits were lost. Source installation requires Bash and standard Linux shell utilities. First launch detects the host, obtains missing dependencies after approval, selects a compatible Go toolchain, builds the CLI/native engine, checks engines, runs Go tests when building the CLI, and commits installation state after validation. Initialization does not run stress benchmarks.

Later launches reuse unchanged components. In a source checkout, use `./start.sh quick` after `git pull` to rebuild changed code. Direct `./vmbench` runs the existing executable independently and cannot rebuild itself.

## Benchmark commands

| Mode | Purpose | Behavior |
|---|---|---|
| QUICK | Preliminary check | Short samples with resource detection; limited statistical confidence |
| STANDARD | Repeated evaluation | Warm-up, repetitions, dispersion and consistency checks |
| MAX | Highest validated observed performance within limits | Searches CPU/RAM concurrency, disk depth/jobs and optional network streams; confirms promising configurations |

```bash
./start.sh quick
./vmbench standard --disk-path /data
./vmbench max --disk-path /data --network-peer 10.0.0.20
# For unattended MAX, acknowledge this invocation explicitly:
./vmbench max --non-interactive --ack-max --disk-path /data
```

The directory must already exist. Without a selected directory Disk is skipped (interactive runs may prompt). Without a peer Network is skipped. Interactive MAX requires acknowledgment; `init.sh --yes` cannot grant benchmark permission. Optional disk write measurements need both `--include-writes --confirm-writes`. Read measurements still prepare real data by writing private files.

MAX reports **Maximum Observed Performance**, not the physical maximum of the VM or host. Resource/time ceilings can truncate a search; unconfirmed results are labeled honestly.

## Installation

```bash
bash init.sh                 # approved missing dependencies, then build/check
bash init.sh --yes           # noninteractive authorization for listed packages only
bash init.sh --offline       # local dependencies only; no downloads/package-manager calls
bash init.sh --check         # read-only compatibility and existing dependency checks
bash init.sh --rebuild       # rebuild CLI/native, reuse usable installed dependencies
bash init.sh --verbose       # show build/test diagnostics
bash start.sh --offline quick
```

Engine resolution prefers a verified `vendor-engines/` bundle, then the installed `engines/` copy, then a compatible system executable. Versions, linkage and origin are recorded in `.vmbench/install-state.json`; rebuild output identifies selected engines. No extra repositories, subscription services, signature bypass, OS upgrades or reboots are requested. Packages unavailable from enabled repositories cause an actionable failure.

Go is selected locally using `go.mod` and `GOTOOLCHAIN`. The pinned fallback is Go 1.23.12, downloaded only in online mode with an architecture-specific checksum from the official Go download index. System Go and shell profiles are not overwritten. Explicit incompatible selections fail rather than being silently overridden. Build scripts use the same selector. See [installation](docs/INSTALLATION.md).

Manual source development, with an already compatible toolchain and dependencies:

```bash
GOTOOLCHAIN=local go test ./...
GOTOOLCHAIN=local CGO_ENABLED=0 go build -trimpath -ldflags "-X vmbench/internal/version.Value=$(cat VERSION)" -o vmbench ./cmd/vmbench
mkdir -p engines
cc -O2 -fno-fast-math -ffp-contract=off -pthread native/compute-bench.c -lm -o engines/compute-bench
```

Supply verified fio, stress-ng and iperf3 there, or use initialization for complete setup. The manual native command targets its build host; portable release builds apply explicit baseline ISA flags.

## Offline deployment

A source snapshot is not a complete offline binary release. On a matching Linux build VM with dependencies and cached pinned sources:

```bash
python3 scripts/fetch-engines.py           # online acquisition on the build machine only
bash scripts/build-engines.sh             # verified cached sources; no downloads
bash scripts/build.sh --candidate         # local qualification archive, not redistribution-ready
# After a real license/transitive-dependency audit:
bash scripts/build.sh --audit /path/to/approved-audit.json
```

Targets are `linux-amd64-musl-static`, `linux-amd64-glibc-static` and corresponding experimental arm64 targets. Build on the selected architecture/libc; arbitrary cross binaries are not advertised as verified. No single universal archive is claimed. All ELF executables must be static, match the target architecture, and pass safe invocation checks. A normal release also needs a matching source/license review record; see [offline packaging](docs/OFFLINE.md) and the [recorded Alpine review](docs/RELEASE-AUDIT.md).

The recorded Alpine amd64 static build passed the complete offline test. Transfer its archive/checksum to a matching offline VM:

```bash
sha256sum -c vmbench-linux-amd64-musl-static.tar.gz.sha256
tar -xzf vmbench-linux-amd64-musl-static.tar.gz
cd vmbench
./vmbench doctor
./vmbench quick
```

No Go, GCC, git, Python, package manager, or network is needed for those binary commands. `doctor` uses the Go ELF reader and safe engine version/capability queries. It does not run workloads. Filesystem/direct-I/O behavior still needs target testing. Current complete-release qualification is listed in the compatibility table; do not interpret packaging success as distribution certification.

## Benchmark methodology

CPU tests count defined xorshift64 integer iterations and floating-point multiply/add iterations, with integrity checks and worker scaling. Results are workload-specific; stress-ng bogo operations are not a universal CPU score.

RAM tests use STREAM-style copy/scale/add/triad bandwidth and shuffled dependent pointer chasing. Dataset size, cache size, thread count, memory pressure and NUMA placement affect interpretation. The latency path uses observable dependent loads so optimization cannot delete the loop.

Disk uses fio JSON+ internally, direct asynchronous io_uring or libaio, private per-job datasets, controlled depth and jobs, and random 4K/sequential 1MiB profiles. It reports IOPS, throughput, average and P99 latency where available. MAX searches the configured parameter space and checks achieved depth/engine evidence. Automatic modes report io_uring setup restrictions and can use compiled libaio; they never substitute synchronous I/O. Advanced explicit engine requests fail if unsupported.

Network uses iperf3, a literal trusted peer IP and controlled TCP streams. Optional options are checked against installed iperf3 help before the workload. A peer is required to measure a path; initialization never starts a network server. Use matching peer features and trusted networks. See [methodology](docs/BENCHMARK-METHODOLOGY.md) and [automatic modes](docs/AUTOMATIC.md).

## Disk safety

Test files are generated automatically inside private owned workspaces. Read tests can write during preparation. VMBench fills, synchronizes and verifies real data before measurement, passes owned descriptors to fio, stops children before cleanup, and removes only verified owned files. It does not format devices, change partitions or overwrite raw block devices.

Abrupt machine/process termination can leave owned data and recovery metadata; unrelated and historical workspaces are never removed automatically. Preserve unexpected data for inspection. Permission changes, free-space/quota errors, unsupported direct I/O, and failed identity checks stop safely. Avoid critical production storage during peak activity. ext4 has disposable-VM evidence; XFS/Btrfs remain unqualified. See [disk safety](docs/DISK.md).

## Example results

The following are **illustrative status excerpts**, not recorded measurements or complete console transcripts. No performance numbers are invented.

```text
QUICK
CPU - Completed
RAM - Completed
DISK - SKIPPED (no directory selected)
NETWORK - SKIPPED (no remote peer)

STANDARD
CPU/RAM: repeated samples; summary includes dispersion
Disk: dataset preparation, measurement and verified cleanup

MAX
Maximum Observed IOPS (Random Read 4K)
Best confirmed configuration and observed range
Unconfirmed configurations remain labeled unconfirmed
```

## CLI reference

`./vmbench --help`, `./vmbench --version`, `./vmbench -v`, `./vmbench version`, `./vmbench doctor`, and `./vmbench MODE --help` are safe queries. Running without arguments opens the mode menu.

Automatic flags:

| Purpose | Flags |
|---|---|
| Targets/config | `--disk-path`, `--network-peer`, `--config`, `--details`, `--non-interactive` |
| Authorization | `--ack-max`, `--include-writes`, `--confirm-writes` |
| CPU/RAM | `--max-workers`, `--max-ram` |
| Disk | `--max-disk-data`, `--min-free-disk`, `--max-disk-depth`, `--max-disk-jobs`, `--max-outstanding-io` |
| Network | `--max-network-streams`, `--network-bitrate` |
| Search/time | `--max-duration`, `--sample-seconds`, `--max-cv`, `--saturation-improvement` |

Automatic YAML is flat; an explicit `--config` or caller `.vmbench/auto.yaml` is optional. `config/auto.yaml` is an example. Acknowledgments must be supplied per invocation.

Advanced commands remain: `cpu test`, `cpu stress`, `ram test`, `ram stress`, `disk test`, `network server`, `network test`, `full test`, `system info`. Advanced presets are `quick|standard|precision`; `max` belongs to the automatic interface.

Advanced flags include `--preset`, `--config`, `--workers`/`--threads`, `--duration`, `--warmup`, `--repetitions`, `--size`, `--method`, `--path`, `--file-size`, `--data-seed`, `--profile`, `--ioengine io_uring|libaio`, `--iodepth`, `--numjobs`, `--direct`, `--precondition`, `--confirm-writes`, `--target`, `--bind`, `--port`, `--streams`, `--udp`, `--bitrate`, `--packet`, `--window`, `--reverse`, `--bidir`, `--load`, `--sample`, `--non-interactive`, `--details`, `--stability-cv`, and `--steal-warning-percent`. Relevant flags depend on the component. Advanced stability CV uses percent; automatic `--max-cv` uses a fraction. [Presentation details](docs/PRESENTATION.md).

## Troubleshooting

| Failure | Action |
|---|---|
| Unsupported distribution/architecture | Check `/etc/os-release`, `uname -m` and compatibility evidence; do not force another family's packages |
| Package manager/package missing | Ask an administrator about enabled repositories or supply verified engines; no extra repositories are enabled automatically |
| Go mismatch/download blocked | Put compatible Go on PATH or in the project-local toolchain directory; check explicit `GOTOOLCHAIN`; offline cannot download |
| Native build failure | Check C compiler/libc headers and `.vmbench/last-build.log`; use `--verbose` |
| Missing fio/iperf3 | Rerun initialization or supply compatible engines beside the CLI; `doctor` checks identity/options |
| Missing loader/shared library | Use an engine built for the actual architecture/libc; inspect `ldd`/`readelf` and the named path |
| Permission denied | Check execute bits, mount `noexec`, target ownership and container restrictions |
| io_uring denied | Automatic modes require available libaio instead; advanced users explicitly select `--ioengine libaio`; no synchronous fallback |
| Directory missing/space insufficient | Create/select an existing disposable directory with reserve space and quota headroom |
| Peer unreachable | Start the trusted peer's iperf3 server, check literal IP, port/firewall and supported features |
| Old binary after git pull | Use `bash start.sh quick`; direct binaries do not rebuild themselves |

See [diagnostics](docs/TROUBLESHOOTING.md) for details.

## Development

`cmd/vmbench` contains the CLI; `internal/benchmark` controllers and parsers; `internal/safety` disk ownership/lifecycle; `internal/system` resource detection; `internal/platform` ELF/runtime diagnostics; `native` the C kernel; `scripts/install` platform/package adapters; `tests` integration fixtures; `config` examples; `docs` methodology/evidence.

```bash
GOTOOLCHAIN=local go test ./...
GOTOOLCHAIN=local go vet ./...
python3 tests/test_bootstrap.py
python3 tests/test_platform.py
python3 tests/test_native.py
bash -n init.sh
bash -n start.sh
# On a disposable Linux build/test environment with local dependencies:
python3 scripts/compatibility-test.py --workloads --output compatibility-result.json
```

Use `--install` with that qualification script only in a disposable environment where package installation is authorized. CI uses separate distro containers and records phase results. RHEL/SLES need separately authorized repository environments. [Build details](docs/BUILD.md). Contributions should preserve measurement definitions and owned-file cleanup, include relevant tests, and report environment and qualification level. Do not label unrun targets verified.

## Limitations

VM scheduling, hypervisor overcommit, CPU steal, caches, storage contention, filesystem behavior, kernel features and peer capacity affect measurements. Container restrictions can prevent otherwise available kernel APIs. Repeated results and transparent working-set/configuration details matter more than universal hardware ratings. MAX is a bounded search, not a theoretical maximum. Distro adapters, static compilation, safe version execution, actual workloads and fully offline deployment are distinct validation levels. Platform qualification is incomplete, particularly ARM64, XFS/Btrfs, RHEL/SLES and cross-distro release redistribution.

## License

The project's [LICENSE](LICENSE) grants permissive use/copy/modify/distribute rights with its notice retained; the actual text is authoritative. Third-party engines retain their own licenses: [fio](LICENSES/fio/COPYING), [stress-ng](LICENSES/stress-ng/COPYING), [iperf3](LICENSES/iperf3/LICENSE). Static libc, libaio, compiler-runtime and optional dependency obligations also apply. Pinned engine sources/notices accompany staged bundles; a completed transitive audit is required before producing a normal distributable release.
