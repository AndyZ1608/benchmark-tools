# RHEL async initialization repair - validation record

Date: 2026-10-15. Source changes were implemented in the existing checkout; prior component-first CLI work was preserved. No commit or release archive was created.

## Root cause

The initializer called `vmbench doctor`, whose async capability gate treated a missing compiled/permitted Disk engine as a whole-installation failure. The setup probes never submitted I/O. In the supplied report, AIO setup succeeded, so the old gate also lacked an exact accepted `libaio` token from its fio engine query. The report lacks `fio --enghelp`; whether that build lacks libaio or reports different output remains unverified. Kernel context creation cannot establish compiled fio support, filesystem direct I/O, completed operations, or achieved depth.

Executable identity, ELF architecture, loader/shared-library and failed engine-query checks remain mandatory. Doctor now reports READY / AVAILABLE_NOT_VALIDATED / UNAVAILABLE / FAILED independently, without filesystem writes. Target-specific Disk capability is deferred, and missing async capability does not disable CPU/RAM/Network.

## Implemented behavior

Automatic Disk tries compiled io_uring first and libaio second, using actual bounded I/O in the selected directory. Explicit engine requests never substitute. A unique owned 1MiB populated/synchronized file supports at most 64 direct 4KiB reads per candidate at QD4, with a five-second child deadline. The existing protected file descriptors, identity checks, process-group cancellation and restricted cleanup remain. Probe results do not enter benchmark statistics. Cleanup must finish before readiness is reported.

Measured results require valid completed-I/O counters, byte/count agreement with block size, no short/dropped I/O, the selected engine, and meaningful achieved depth. At requested QD >1, at least 50% of observations must be in buckets reaching the power-of-two range at half requested depth, with a minimum lower bound of two. QD1-dominated high-depth results are rejected with a measurement limitation. The engine stays fixed throughout the search and is displayed in results.

The CLI reads io_uring policy and probes setup permission without modifying sysctl, SELinux, seccomp, capabilities or group membership. Sysctl 0 does not establish runtime permission; 1 permits the configured group or CAP_SYS_ADMIN, with no group exemption for -1; 2 disables setup for all processes.

## Files changed for this repair

- `init.sh` (visible capability diagnostics and accurate validation scope), `start.sh` (Git executable mode), `scripts/bootstrap-common.sh` (separate successful fio query from Disk engine availability).
- `internal/platform/platform.go`, `uring_linux.go`, `platform_test.go`, `doctor_linux_test.go` (independent status, read-only policy diagnostics, retained fatal health checks, regression tests).
- `internal/benchmark/disk/capability.go`, `capability_test.go`, `capability_linux_test.go`, `disk.go`, `lifecycle.go`, `lifecycle_linux_test.go`, `session.go`, `session_test.go` (bounded selection, completed I/O/depth checks, engine consistency and protected cleanup tests).
- `internal/benchmark/auto/backend.go`, `run.go`, `options.go`, `auto_test.go` (validated engine selection, explicit `--ioengine`, independent modules and regression tests).
- `internal/config/config.go`, `cmd/vmbench/main.go`, `automatic.go` (advanced auto selection and CLI help), `internal/presentation/automatic.go`, `disk.go` (selected engine display).
- `tests/test_bootstrap.py`, `tests/test_platform.py` (RHEL recovery, no-async initialization, query failure, README and Git modes).
- `README.md`, `docs/COMPATIBILITY.md`, `AUTOMATIC.md`, `DISK.md`, `INSTALLATION.md`, and this record (permissions, engine policy, methodology and qualification limits).

CPU/RAM/Network engines and disk filesystem safety implementation were not modified. Existing working-tree changes from the preceding menu task remain present.

## Actual checks

| Check | Environment | Result |
|---|---|---|
| `go test ./...` | Windows Go 1.23.12 | Passed |
| `go vet ./...` | Windows | Passed |
| Full Go suite, uncached | Alpine 3.22.6 amd64, private QEMU/ext4 VM, UID 1000 | Passed |
| Functional probe fixtures | Linux UID 1000 | io_uring preference, EPERM fallback, explicit refusal, failed submission, QD1 rejection and cleanup failure passed |
| Bounded real fio probe | Explicit `.vmbench/disposable-async` directory in private VM | auto selected io_uring; explicit libaio passed; 1.16s test runtime; owned files removed |
| `./start.sh --offline`, version/help/doctor | Linux UID 1000 | Passed, including source rebuild and independent capability output |
| Repeated offline startup | Linux UID 1000 | CLI/native/fio/stress-ng/iperf3 and installation-state hashes unchanged |
| `bash -n init.sh`, `bash -n start.sh` | Windows Git Bash and Linux Bash | Passed |
| ShellCheck with `-x`, both entry points and sourced/build scripts | Linux | Passed; no diagnostic exclusions added |
| Bootstrap fixtures | Linux | 55 tests passed in 243.316s |
| Platform fixtures | Linux | 15 tests, one expected Git-checkout test skip in source export; otherwise passed |
| README and Git mode tests | Host Git checkout | Both passed; index records 100755 for init.sh and start.sh |
| Linux ARM64 CLI cross-build | Windows Go toolchain | Passed; no native ARM64 execution |

ShellCheck initially ran without `-x` and reported unresolved sourced-module/variable diagnostics. The final invocation matches the repository compatibility workflow and follows those modules. Initialization and all final checks completed successfully. The optional real-I/O test only runs when both an explicit disposable directory and a real fio binary are supplied; it was executed separately here. Unprivileged read-only mount testing remains subject to mount capability.

The tested Linux Go source hashes were compared with the workspace and matched. Detailed logs and checksums are retained locally under `.tools/rhel-async-validation/` (ignored development evidence).

## Remaining limitations

No real RHEL 9.6 host or its fio 3.35 binary was available. RHEL EPERM behavior and fio 3.35 executable identity were modeled with fixtures. Upstream fio 3.35 statistics and bounded-operation option definitions were checked, but actual I/O here used the pinned fio 3.39 build on Alpine. This is not fresh RHEL/XFS/Btrfs, native ARM64, real network, or offline release qualification. No full benchmarks or MAX searches were run in this repair validation; real I/O was confined to the explicitly designated disposable probe directory.

See [RHEL report and primary references](../COMPATIBILITY.md) for the reported host and security-policy definitions.
