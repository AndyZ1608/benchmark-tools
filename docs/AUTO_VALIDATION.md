# Automatic-mode validation

Date: 2026-10-14. These checks establish functional behavior within the tested limits, not hardware qualification or an absolute performance maximum.

## Environments and build

- Windows x86_64 development host with Go 1.23.12 and Git Bash.
- Disposable Alpine 3.22.6 x86_64 guest, Linux 6.12.110-0-virt, QEMU software emulation, two virtual CPUs and 1536MiB guest RAM.
- fio 3.39 on an isolated 4GiB ext4 virtual disk. No production device was passed through.
- CLI built for Linux amd64 with CGO disabled and for Windows, injecting the repository `VERSION`. Native code cross-built with Zig 0.13.0, `-O2 -fno-fast-math -pthread -static`; the optimized integrity regression also compiled with guest GCC 14.2.0.

Ubuntu 22.04/24.04 execution, an actual remote iperf3 peer, real stress-ng workloads and a complete static release qualification were not performed. The guest's lack of resolvable cgroup mounts was reported as unavailable; cgroup restriction behavior was tested with deterministic Linux filesystem fixtures.

## Automated checks

| Check | Executed result |
| --- | --- |
| `go test ./...` on Windows | Passed |
| All Linux test-bearing package binaries, compiled with `go test -c` and run in the guest | All 13 passed on the final rerun, no skipped tests |
| `go vet ./...`, Windows and Linux targets | Passed |
| `bash -n init.sh`, `start.sh`, `scripts/bootstrap-common.sh`, `scripts/build.sh` | Passed |
| `python3 tests/test_bootstrap.py` in Linux with controlled compilers/engines | All 34 passed |
| `python3 tests/test_native.py` with optimized GCC compilation | Passed |
| Windows and Linux builds, help/menu and three version aliases | Passed |

The Go suite covers mode policies, cgroup v1/v2 hierarchy/mount roots, nominal versus affinity CPU count, quotas including an equal-capacity quota, ancestor memory usage, zero memory.high, safe RAM budgets, pressure reductions, independent Integer/FP peaks, RAM thread discovery, adaptive QD/jobs, actual fio engine/depth evidence, saturation heuristics, longer confirmation medians, unstable/spike rejection, boundary warnings, network streams/directions and offered-rate ceilings (including very low rates), no peer, insufficient storage, partial failures, cleanup safety stops, cancellation and compact ASCII output. Existing component/CLI, filesystem replacement, descriptor exhaustion, data materialization and no-report regressions remain included.

Bootstrap checks use real Bash/state/hash workflows with controlled platform/compiler/engine fixtures. A new case edits `internal/benchmark/auto/run.go`, verifies only the CLI rebuilds, confirms exact forwarding of mode/acknowledgment/path arguments, then confirms reuse on the next launch. Linux fixture PATH now excludes installed real compilers/engines so missing-dependency cases remain deterministic. Source fixtures transferred from Windows use LF, matching a Linux Git checkout, and the suite receives noninteractive stdin. Early harness attempts exposed those setup issues; the completed isolated suite passed all 34 tests. A Windows run had one transient directory-rename permission failure in the simulated Go download; that case passed on its standalone rerun.

## Real native and fio runs

QUICK, STANDARD and MAX were exercised as complete commands with real native CPU/RAM kernels and real fio. Limits were deliberately small:

- `--non-interactive --disk-path` pointing to a disposable directory whose name contains spaces.
- `--max-ram 64MiB --max-disk-data 64MiB --max-disk-depth 4 --max-disk-jobs 2 --max-outstanding-io 8`.
- `--sample-seconds 1 --max-duration 10m --max-cv 0.5`; MAX received `--ack-max`.
- QUICK/MAX allowed two workers; STANDARD allowed one. The relaxed CV ceiling is for emulated functional checks and is not the product default or evidence of reliable hardware performance.

MAX executed real QD1/2/4 and one/two-job candidates, fresh longer repeated confirmations, and separate Random Read 4K/Sequential Read optimization. The summary showed measured winners, observed fio depth buckets and search-boundary/cache limitations. Each disk module prepared its dataset once and removed its own workspace. Three sentinel hashes (existing file, nested file and historical `.vmbench-*` directory) were compared after every mode and stayed unchanged. Network was explicitly SKIPPED without localhost substitution. No automatic result files were created.

Real testing found two correctness defects and drove repairs:

1. The original optimized pointer chase was eliminated because its atomic sink was unobserved. Assembly inspection confirmed the missing traversal. Volatile dependent loads plus the known-cycle check restored real traversal; the optimized regression accepts a known cycle and detects a deliberately corrupted one. Fresh native execution returned zero integrity errors. Earlier affected latency values are invalid for comparison.
2. Forked fio jobs used `setsid` and could survive process-group cancellation. A real SIGINT check caught a surviving worker even though the old cleanup had removed the files. Explicit `thread=1` keeps all jobs in the reaped fio process. Independent files, engine/QD/direct I/O and per-job metrics remain preserved. The owned worker from that diagnostic run was identified and terminated inside the disposable guest.

## Final execution record

| Real final check | Result |
| --- | --- |
| Complete QUICK, STANDARD and MAX with thread-based fio jobs | All passed with the bounded settings above |
| QUICK with `--include-writes --confirm-writes` | Passed Random/Sequential Read, Random/Sequential Write and Mixed 70/30; separate mixed read/write metrics visible |
| SIGINT during disk execution | Expected unsuccessful exit; workspace removed, sentinels unchanged, no remaining fio workers |
| SIGTERM during disk execution | Expected unsuccessful exit; workspace removed, sentinels unchanged, no remaining fio workers |
| Whole-run one-second timeout | Expected unsuccessful exit with deadline error; no automatic report files |
| Noninteractive MAX without acknowledgment | Refused before benchmark load |
| No-argument menu exit, help and version aliases | Passed |
| Final Linux package suite after the fio lifecycle repair | All 13 passed; zero skipped |

The final script verified workspace removal and sentinel hashes after every mode/cancellation, and explicitly checked for leftover fio processes. Engine work and storage were confined to the disposable guest. See [mode methodology and limits](AUTOMATIC.md) and [disk ownership guarantees](DISK.md).

These tests do not establish storage backend allocation, immunity to caching/compression, cross-NUMA placement quality, target Ubuntu compatibility, a network maximum without a peer, or independent native-kernel timing calibration. No host kernel change, system upgrade or host reboot was performed. Only the disposable VM was used for real load.
