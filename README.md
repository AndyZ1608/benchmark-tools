# VMBench
An offline-first Linux CLI implementation under development. **This is not a qualified v1.0 offline release.**

## Quick Start

Supported source-install targets: **Ubuntu 22.04 or 24.04, x86_64**.

```bash
git clone https://github.com/AndyZ1608/benchmark-tools.git
cd benchmark-tools
chmod +x init.sh start.sh
./start.sh
```

The first launch initializes the project, then opens the existing CLI. By default this is an **online development installation**: missing GCC/build-essential and fio/stress-ng/iperf3 packages are listed before apt installation, and require an interactive confirmation. The installer never performs a system upgrade. Use `./start.sh --yes` to explicitly authorize noninteractive installation of the listed missing packages. No benchmarks or background servers are started during initialization.

If necessary, initialization downloads checksum-pinned Go 1.23.12 into `.vmbench/toolchains/`; it never overwrites system Go or changes `go env -w` settings. Compatible newer installations are reused. All build/test invocations use the selected local toolchain with automatic Go switching disabled.

Subsequent launches verify source/binary hashes, engine identities, shared libraries and version commands, then launch without rebuilding, downloading or installing packages. CLI source changes rebuild the CLI; native source changes rebuild only the native kernel. `./init.sh` reruns validation and reuses unchanged components.

```bash
./start.sh cpu test --preset quick
./start.sh cpu stress --workers 8 --duration 300
./start.sh ram test --threads 2 --size 1GiB
./start.sh disk test --path /data --profile randread-4k
./start.sh system info
./vmbench cpu test --preset quick

./init.sh --offline                 # local tools and files only
./start.sh --offline --help         # startup options precede CLI arguments
./start.sh --offline --reinit --help # force source builds where source exists
./init.sh --yes                     # authorize missing apt packages
```

`--reinit` forces source rebuilding; prebuilt-only releases are revalidated. `--offline`, `--yes`, `--engine-mode system|bundled`, and `--reinit` are startup options consumed only before the first CLI argument. Use `--` to end that prefix. All remaining arguments are forwarded exactly using `exec`; application exit codes and signals are preserved. Scripts resolve the repository from their own location and also work from other directories or paths containing spaces. Relative benchmark paths retain the caller's working-directory meaning.

Offline mode never uses apt, Git network operations, remote source fetching, package registries, or automatic Go toolchain downloads. A complete prebuilt release needs no Go, GCC, Python or binutils to initialize: only the standard Ubuntu runtime/base utilities and its verified engines. An offline source build needs a compatible local Go and GCC. Missing dependencies produce an error and leave no successful new state.

Default engine mode is `system`: existing compatible local engines or installed system engines are copied beside the CLI and their runtime dependencies checked using `ldd`. **These host-linked binaries are not automatically portable.** `./init.sh --engine-mode bundled` reuses `vendor-engines/` or calls the existing pinned source-fetch/build scripts and requires static executables. It needs the Linux build dependencies documented in [docs/BUILD.md](docs/BUILD.md); offline bundled source builds require cached archives. Static validation alone does not qualify an Ubuntu standalone release.

Installation state is stored in `.vmbench/install-state.json` with checksummed binary/source manifests, engine versions, linkage and toolchain metadata. State is written only after all checks pass; it is never executed as shell code. Failures and SIGINT/SIGTERM clean up this attempt's staging directory. A stale `.vmbench/init.lock` after an uncatchable interruption must be removed only after confirming no initializer is active. `--reinit` provides explicit rebuilding for restored binaries whose source provenance is unknown.

See [docs/BOOTSTRAP.md](docs/BOOTSTRAP.md) for toolchain diagnostics, troubleshooting and validation details. The bootstrap does not change the existing benchmark methods or remedy the benchmark qualification limitations below.

## Use
Build the CLI: `go build ./cmd/vmbench`. Linux native kernel: `cc -O2 -fno-fast-math -pthread native/compute-bench.c -o engines/compute-bench` beside the CLI. Supply fio, stress-ng and iperf3 under the same engines directory; runtime never downloads anything.

```
./vmbench --help
./vmbench system info
./vmbench cpu test --preset quick
./vmbench cpu stress --workers 8 --duration 300
./vmbench ram test --threads 8 --size 4GiB
./vmbench disk test --path /data --profile randread-4k --iodepth 32 --numjobs 4 --size 2GiB
./vmbench network server --bind 127.0.0.1
./vmbench network test --target 127.0.0.1
./vmbench full test --preset precision --path /data --non-interactive
```

Benchmark results use a compact English summary with ASCII layouts. `--details` adds human-readable workload statistics and recorded telemetry. Progress and errors go to stderr. Results stay in memory; the CLI does not print raw JSON, save benchmark reports or create `results/`. Existing historical reports are left untouched. `system info` also uses a text table. See [output rules](docs/PRESENTATION.md) and [illustrative summaries](docs/RESULT_EXAMPLES.md).

```bash
./vmbench cpu test --preset quick
./vmbench ram test --details
./vmbench disk test --path /data
./vmbench full test --path /data --stability-cv 5 --steal-warning-percent 1
```

The old language, JSON and output-directory options have been removed. Remove those fields from older YAML configurations. The default configuration already uses the supported options.

No arguments opens a command prompt. Bare component commands prompt for worker count, duration, warmup and repetitions. Enter accepts defaults. Flags enable noninteractive use. `--config config/default.yaml` accepts a deliberately restricted flat YAML scalar mapping; nested YAML is rejected. Precedence: CLI > file > preset > defaults. Seconds are integers; sizes support bytes, KiB, MiB, GiB.

Quick: 1s warmup, 2s per workload, one repetition. Standard: 2s warmup, 10s per workload, three repetitions. Precision: 5s warmup, 30s per workload, five repetitions. CPU and RAM run separately with one and requested worker counts. RAM total dataset is divided into three arrays and partitioned between workers. Each CPU unit means one defined recurrence iteration, not a universal operation or score. Floating-point contraction/compiler/platform changes may affect comparability.

RAM Copy/Scale/Add/Triad are independently implemented STREAM-style operations, not official STREAM results. Working sets are configurable; verify they exceed aggregate LLC before interpreting as memory bandwidth. Pointer chasing uses a deterministic shuffled single cycle per worker; reports dependent access time including loop overhead. Native CPU correctness and timing need further qualification. Memory integrity writes and reads deterministic values; completion does not prove hardware reliability.

Disk tests validate an existing writable directory, create a private `.vmbench-<random-id>/` workspace, and write/verify seeded pseudo-random data across every job file before fio starts. **Read tests also generate preparation writes.** `--size` is per job for `disk test`; four jobs at `--size 2GiB` require 8GiB plus at least 1GiB free-space reserve and metadata headroom. `--file-size` remains an alias for disk tests; full tests use it for disk size while `--size` keeps its RAM meaning. Unspecified disk size defaults to a maximum of 1GiB per job and may shrink safely to available capacity; explicit sizes never shrink silently. Write/mixed profiles require `--confirm-writes`.

fio receives inherited handles to the exact prepared regular files. Direct I/O, engines, profiles, queue depths, job count, runtime, warmup, repetitions and internal JSON+ parsing remain intact. Temporary files are removed by tracked identity and directory handles after fio exits, including recoverable errors and cancellation. Cleanup refusals are visible and preserve the original error. SIGKILL, VM shutdown and host failure can leave workspace ownership metadata; old workspaces are never automatically removed. Direct I/O does not bypass all backend caches, and written pseudo-random data cannot guarantee physical allocation or defeat every compression/deduplication layer. See [disk lifecycle and safety](docs/DISK.md).

Network defaults to loopback server bind; explicitly select an interface for peers. Use trusted networks. iperf3 throughput is not forwarding PPS. Its raw end/interval data preserves UDP jitter/loss and TCP metrics. Missing peer skips network. No independent RTT probe is implemented.

Default warnings cover actionable failures, invalid results, data safety and material measurement limitations. General caveats are confined to details. CPU steal warnings use a configurable screening threshold (`--steal-warning-percent`, default 1%); small measurements remain visible without implying a performance problem. Human output supports `NO_COLOR`, redirected output and `TERM=dumb`; optional green status means successful execution only. No sysctl/governor/cache tuning occurs.

## Validation and limitations
Run `go test ./...`. Release: `bash scripts/build.sh`; see docs/BUILD.md for incomplete third-party build prerequisites. The Go CLI and native kernel were cross-compiled for Linux; Go unit tests and Windows help/version were executed on this Windows host. Native CPU/RAM kernels, real stress-ng/iperf3 runs and Ubuntu offline compatibility remain unexecuted. All Go package tests and real fio 3.39 read/write/mixed and cancellation smoke tests passed in an isolated Alpine Linux guest on disposable ext4 storage; these are functional checks, not representative hardware measurements. See [executed disk validation](docs/DISK_VALIDATION.md). Missing engines fail explicitly.

Still incomplete: detailed live telemetry sampling, cgroup v1 support, ancestor memory-usage accounting, thermal readings and detailed NUMA policy, block-device metadata, percentage progress, native trusted reference fixtures, user-adjustable disk block sizes/headroom/data patterns, steady-state and time-series checks, independent network RTT measurement, qualified Linux third-party engine builds and complete transitive license audit. Summary assessments describe recorded observations and do not qualify hardware or rank speed. Do not use current output as acceptance evidence for infrastructure.
