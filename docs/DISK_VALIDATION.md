# Executed disk validation

Validation date: 2026-10-14. These are functional checks, not storage performance qualification.

## Environments

- Development host: Windows x86_64, Go 1.23.12. Linux executables were cross-compiled with `GOOS=linux GOARCH=amd64 CGO_ENABLED=0`.
- Execution guest: disposable Alpine 3.22.6 x86_64, Linux 6.12.110-0-virt, QEMU software emulation, fio 3.39. Test data lived on an isolated 4GiB virtual disk formatted as ext4. No production storage was passed through to the guest.
- Ubuntu 24.04 was not available for execution. This does not establish Ubuntu compatibility, backend allocation guarantees, or representative hardware performance.

## Build and automated checks

| Check | Result |
| --- | --- |
| `go test ./...` on Windows | Passed; Linux-specific files are excluded on this platform |
| Linux test executables for every test-bearing package, executed inside the guest | All 11 packages passed; no skipped tests |
| `go vet ./...`, Windows and Linux targets | Passed |
| `bash -n init.sh`, `start.sh`, `scripts/bootstrap-common.sh`, `scripts/build.sh` | Passed |
| Windows and Linux CLI builds with version injected from `VERSION` | Passed |
| Built CLI `--version`, `-v`, `version`, `--help` | Passed; version aliases agreed |

The Linux suite used `go test -c` for each package, copied the binaries and required fio parsing fixture into matching package directories, then executed each binary in the guest. It includes all CLI/config/presentation/runner/parsing/statistics tests in addition to the new Linux lifecycle and filesystem tests. Initial temporary harness failures from CRLF shell lines and relative test executable paths were corrected; the final complete suite passed.

New automated coverage includes:

- Existing/missing/nondirectory targets, permissions, read-only mounted filesystems, pseudo-filesystems and symlinked path components.
- Capacity planning for every job, the 1GiB reserve, automatic sizing, explicit size failures and integer overflow before file creation.
- Private permissions, collision refusal, full deterministic data generation and readback, seed/job separation, short writes, sizes, checksums, and non-sparse extents where supported.
- Preparation failure, partial cancellation, actual `EFBIG` and file-descriptor exhaustion with cleanup. Temporary process resource limits are restored by the tests.
- Real child-process execution with inherited prepared files, preparation before measurement, unchanged fio runtime, four independent jobs, all profile configurations and write confirmation.
- Engine failure, invalid metrics, cancellation, timeout, cleanup refusal and preservation of the original error and ownership metadata.
- Replacement regular files, symlinks, added hardlinks, replaced workspaces, renamed target paths and untracked entries. Sentinel contents survive and cleanup cannot redirect to the replacement target.
- Missing fio, a missing executable interpreter, and shared-library diagnostic propagation before preparation. Shared-library failure text is a controlled process fixture, not an actual removed host library.
- Internal JSON+ parsing, human-readable lifecycle output and no automatic results files.

## Real fio smoke checks

The final CLI ran the following profiles successfully with real fio 3.39:

| Profile | Jobs | Queue depth | Dataset per job | Measurement |
| --- | --- | --- | --- | --- |
| `randread-4k` | 4 | 32 | 16MiB | 1 second, no ramp |
| `seqread-1m` | 4 | 32 | 16MiB | 1 second, no ramp |
| `randwrite-4k` | 4 | 32 | 16MiB | 1 second, no ramp |
| `seqwrite-1m` | 4 | 32 | 16MiB | 1 second, no ramp |
| `mixed-70-30` | 4 | 32 | 16MiB | 1 second, no ramp |

The target directory contained spaces. Each run used `--preset quick --duration 1 --warmup 0 --size 16MiB --data-seed 42`; write/mixed runs also used `--confirm-writes`. The default `io_uring` engine and direct I/O remained selected. These small datasets and emulated storage validate execution only.

After every run, the new workspace was gone and SHA-256 comparisons of an existing file, a nested user file and an unrelated historical `.vmbench-*` workspace were unchanged. No benchmark report directory was created. Preparation messages preceded fio measurement and cleanup completed before the successful summary.

Real CLI SIGINT and SIGTERM tests interrupted a running 30-second read workload: both returned failure/cancellation, removed current-run files and preserved sentinel hashes. A SIGKILL during preparation left identifiable `.vmbench-owner` metadata and preserved unrelated data; no automatic orphan deletion was attempted.

## Original failure reproduction

With `/data` absent, the previous committed CLI returned the reported bare `ERROR: no such file or directory` from its pre-benchmark filesystem inspection. The updated CLI identified `open target directory "/data" (component "data")` and the underlying cause. Both runs exited unsuccessfully; the updated CLI did not create `/data` or start fio measurement.

This reproduces the failing code path. It cannot determine whether the original user's Ubuntu target was missing or encountered another unavailable path/dependency.

## Boundaries

CPU, RAM and network benchmarking algorithms and native kernels were unchanged. No CPU/RAM/stress or network workload, system upgrade, host kernel change or host reboot was performed. Only the disposable guest's disk was benchmarked.

Quota exhaustion, storage-device faults, host power loss and every possible filesystem/backend were not exercised. Same-UID/privileged concurrent mutation and abrupt-termination limits are documented in [DISK.md](DISK.md). A full static release build and Ubuntu offline qualification remain separate work.
