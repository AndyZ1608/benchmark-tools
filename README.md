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

`--reinit` forces source rebuilding; prebuilt-only releases are revalidated. `--offline`, `--yes`, `--engine-mode system|bundled`, and `--reinit` are startup options consumed only before the first CLI argument. Use `--` to end that prefix. All remaining arguments are forwarded exactly using `exec`; application exit codes and signals are preserved. Scripts resolve the repository from their own location and also work from other directories or paths containing spaces. Relative benchmark/output paths retain the caller's working-directory meaning.

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
./vmbench disk test --path /data --profile randread-4k --iodepth 32 --numjobs 4
./vmbench network server --bind 127.0.0.1
./vmbench network test --target 127.0.0.1
./vmbench full test --preset precision --path /data --non-interactive
```

Benchmark results default to a compact Vietnamese summary. Use `--lang en` for English, `--details` for technical tables, or `--json` for the complete report as JSON on stdout. Progress and diagnostics go to stderr. Every benchmark still saves its full report; `--details` and `--json` are mutually exclusive. See [output rules](docs/PRESENTATION.md) and [illustrative summaries](docs/RESULT_EXAMPLES.md).

```bash
./vmbench cpu test --preset quick --lang en
./vmbench ram test --details --lang vi
./vmbench disk test --path /data --json > disk-report.json
./vmbench full test --path /data --lang en --stability-cv 5
```

No arguments opens a command prompt. Bare component commands prompt for worker count, duration, warmup and repetitions. Enter accepts defaults. Flags enable noninteractive use. `--config config/default.yaml` accepts a deliberately restricted flat YAML scalar mapping; nested YAML is rejected. Precedence: CLI > file > preset > defaults. Seconds are integers; sizes support bytes, KiB, MiB, GiB.

Quick: 1s warmup, 2s per workload, one repetition. Standard: 2s warmup, 10s per workload, three repetitions. Precision: 5s warmup, 30s per workload, five repetitions. CPU and RAM run separately with one and requested worker counts. RAM total dataset is divided into three arrays and partitioned between workers. Each CPU unit means one defined recurrence iteration, not a universal operation or score. Floating-point contraction/compiler/platform changes may affect comparability.

RAM Copy/Scale/Add/Triad are independently implemented STREAM-style operations, not official STREAM results. Working sets are configurable; verify they exceed aggregate LLC before interpreting as memory bandwidth. Pointer chasing uses a deterministic shuffled single cycle per worker; reports dependent access time including loop overhead. Native CPU correctness and timing need further qualification. Memory integrity writes and reads deterministic values; completion does not prove hardware reliability.

Disk uses exclusive file creation and anchored directory descriptors, never raw devices. Each job has its own file, 1GiB minimum free-space reserve. **Even read profiles initialize their new files by writing them.** Write profiles require `--confirm-writes`. Files are isolated in a private per-run directory and removed on ordinary completion/cancellation. After abrupt process death, inspect the private `vmbench-<run-id>/` directory and its `vmbench-<run-id>-*.dat` files in the selected directory and remove only the files belonging to that run. Never benchmark valuable workloads without accounting for I/O contention. Direct I/O bypasses guest page cache in supported configurations, not hypervisor or backend caches. Engine failures are reported without fallback. fio JSON+ bytes/s and nanoseconds are preserved; read and write percentiles are separate and never averaged. Queue depth scaling uses 1,4,16,32,64.

Network defaults to loopback server bind; explicitly select an interface for peers. Use trusted networks. iperf3 throughput is not forwarding PPS. Its raw end/interval data preserves UDP jitter/loss and TCP metrics. Missing peer skips network. No independent RTT probe is implemented.

Reports under `results/` contain workload, metadata, raw results, warnings, failures and elapsed time. Human output uses aligned text tables with optional status colors on suitable interactive terminals. `NO_COLOR`, redirected output and `TERM=dumb` disable colors. Green indicates successful execution only. No sysctl/governor/cache tuning occurs.

## Validation and limitations
Run `go test ./...`. Release: `bash scripts/build.sh`; see docs/BUILD.md for incomplete third-party build prerequisites. The Go CLI and native kernel were cross-compiled for Linux; Go unit tests and Windows help/version were executed on this Windows host. Linux native kernels, real fio/stress-ng/iperf3 runs, cancellation of engine trees and Ubuntu offline compatibility remain unexecuted. Missing engines fail explicitly.

Still incomplete: detailed live telemetry sampling, cgroup v1 support, ancestor memory-usage accounting, thermal readings and detailed NUMA policy, block-device metadata, percentage progress, native trusted reference fixtures, user-adjustable disk block sizes/headroom/data patterns, steady-state and time-series checks, independent network RTT measurement, qualified Linux third-party engine builds and complete transitive license audit. Summary assessments describe recorded observations and do not qualify hardware or rank speed. Do not use current output as acceptance evidence for infrastructure.
