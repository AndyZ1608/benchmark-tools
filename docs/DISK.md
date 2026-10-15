# Disk lifecycle and safety

VMBench benchmarks new private regular files using fio. The selected target must already exist and be writable/searchable. The CLI never creates a missing mount point, opens a user's existing file, or benchmarks a raw device.

```bash
./vmbench disk test --path /data --profile randread-4k --iodepth 32 --numjobs 4
./vmbench disk test --path /data --size 2GiB --numjobs 4 --data-seed 42
./vmbench disk test --path /data --profile randwrite-4k --size 1GiB --confirm-writes
```

Use a disposable directory for smoke checks. Preparation and benchmarks generate real I/O and can contend with other workloads. A directory on a filesystem with valuable workloads is not an isolated physical device.

## Diagnosing the original failure

The earlier CLI called `system.Filesystem` before disk preparation. Its first `statfs(path)` returned a bare syscall error. A missing `/data` therefore produced `ERROR: no such file or directory` before fio could run. This was reproduced using the previous committed CLI in an isolated Linux guest with `/data` absent. It identifies a reproducible source of the exact message, not the state of an inaccessible user's Ubuntu machine.

The new validation identifies the operation and target/component path, and leaves missing directories uncreated. It also probes the bundled fio executable, its version and the requested engine before creating files. Missing executables, execute failures, missing ELF interpreters, and loader/shared-library errors include the engine path and the original cause. Requested engines are never silently substituted.

## Dataset size

For `disk test`, `--size` and `--file-size` select **bytes per job**. Four jobs at 2GiB each need 8GiB of test data. The reserve is at least 1GiB, with an additional 1MiB metadata allowance. Available filesystem capacity is checked for all files with overflow-safe arithmetic before workspace creation. In a `full test`, `--size` retains its RAM meaning and `--file-size` selects the disk file size.

Without an explicit disk size, the default maximum is 1GiB per job. If necessary, it is reduced to available capacity after the reserve, rounded down to whole MiB; fewer than 16MiB per job cannot be selected automatically. The selected per-job and total size are displayed. Explicit CLI/YAML size values never shrink silently and take precedence as before. Dataset size must be a multiple of the workload's unchanged block size (4KiB or 1MiB).

Start with the default for ordinary tests, and use larger files when practical to reduce cache effects. Small files such as 16MiB per job are suitable for functional smoke checks and are not reliable infrastructure performance evidence. Backend cache sizes may be unknown, and no selected size proves cache effects are absent.

Quota-aware filesystem availability is used when the filesystem exposes it. Optional `fallocate` reserves job-file space before preparation where supported; this can detect ENOSPC/EDQUOT early. Unsupported reservation falls back to actual writes, not sparse initialization. Capacity/reserve checks continue during preparation. Kernel quota and write/sync failures remain explicit errors. Dedicated quota-limit discovery is not implemented; other processes can consume space concurrently, so free-space checks cannot reserve capacity against arbitrary external writers.

## Preparation before measurement

Each run creates a cryptographically random private directory with mode 0700. Each job gets its own newly created `job-001.dat`, etc., with mode 0600, O_EXCL and O_NOFOLLOW. No existing file is reused. File creation and optional space reservation alone do not complete preparation.

An AES-CTR pseudo-random stream derives a separate key for each job from the configured seed. `--data-seed random` is the default and chooses a fresh random 64-bit seed; explicit unsigned values such as 42 or 0x2a permit deterministic replay. Fixed seeds repeat a job's data across runs and may be deduplicated by a backend, so use fresh defaults for ordinary storage testing. Generation uses bounded 1MiB chunks instead of allocating the dataset in memory.

Every intended byte is written, short writes are handled, files are fsynced, sizes are checked, and the entire deterministic stream is read back and verified. Filesystems supporting SEEK_HOLE must report no hole within the prepared extent. A checksum and preparation state are recorded in the private ownership metadata. File-specific FADV_DONTNEED is attempted after verification; fio's file-specific invalidate behavior remains enabled. No global caches are dropped, and no governor/kernel/locale settings are changed.

Preparation progress and duration are separate from the measurement phase. fio alone measures IOPS, throughput, completion-latency means and percentiles. Its runtime, warmup and repetitions retain their original meanings. CLI overall Duration includes the complete lifecycle; the separately displayed preparation duration is not fio runtime. Ordinary full-file preparation is implemented; the existing unsupported steady-state `--precondition` option continues to fail explicitly rather than claiming hardware steady state.

Written data and filesystem extent checks establish that application file data was materialized and verified. They do not guarantee physical backend allocation, persistence beyond storage guarantees, or immunity from compression, deduplication, VM/backend caches and thin provisioning.

## fio file isolation

The selected engine, direct I/O flag, read/write profile, block size, I/O depth, four named jobs where requested, runtime, warmup, repetitions, mixed 70/30 ratio and internal JSON+ parsing are preserved. Fio receives inherited handles via `/proc/self/fd/3`, `/proc/self/fd/4`, etc., each referring to a file VMBench opened and retains. Replacing a workspace/file pathname cannot redirect fio into an unrelated file.

`allow_file_create=0`, `unlink=0`, `unlink_each_loop=0` and `fallocate=none` prevent fio from managing the application-owned file lifecycle. Write buffers retain refill/scramble behavior and explicitly disable requested compression/deduplication percentages. These settings do not defeat all backend optimizations. The [fio documentation](https://fio.readthedocs.io/en/latest/fio_doc.html) describes these controls.

Results must include all requested jobs and valid active-direction IOPS, bandwidth, runtime, mean completion latency and p99 values. Missing/invalid required measurements fail the run. Read/write metrics remain separate; percentile distributions are not averaged. The CLI displays human-readable English/ASCII summaries with no automatic JSON reports.

## Cleanup and interruptions

Target components are opened relative to directory handles without following symlinks. Workspace and file device/inode identities, UID, regular-file/directory type, restrictive permissions and file link counts are checked. Handles keep the original objects alive. Cleanup only unlinks explicitly tracked matching entries and removes the verified private directory when empty. It does not glob by prefix, recurse, follow symlinks, remove the target, or touch an older workspace. Unknown entries and replacement objects remain untouched and cause a visible cleanup refusal. An emergency descriptor slot allows verified cleanup even after file-descriptor exhaustion during preparation.

fio processes are waited for before application cleanup. Timeout/cancellation uses the existing Linux process-group termination behavior. SIGINT/SIGTERM trigger the same owned-file cleanup. Original fio/preparation errors are preserved alongside cleanup failures; a cleanup failure cannot become COMPLETED or claim deletion succeeded.

Named jobs now run as fio threads (`thread=1`), keeping every worker in the application-owned fio process. The previous default forked fio workers could call `setsid` and escape process-group termination. The owned process is reaped before cleanup; logical jobs, separate files, async engines, direct I/O and per-job metrics are preserved. This execution-backend choice is recorded in fio options and should be considered in comparisons with older runs.

Private permissions isolate other users. A process running as the same UID or a privileged administrator can bypass that isolation and mutate objects concurrently; no claim is made to defeat arbitrary privileged/same-UID races between separate filesystem operations. Do not concurrently alter an active workspace. Detected replacements, ownership/permission changes and added hardlinks are refused. Linux [openat2](https://man7.org/linux/man-pages/man2/openat2.2.html) and [unlinkat](https://man7.org/linux/man-pages/man2/unlink.2.html) documentation describe the handle-relative and unlink primitives and their limits.

SIGKILL, VM shutdown or host/storage failure cannot guarantee immediate cleanup. `.vmbench-owner` records run identity, UID/PID/time, workspace and job device/inode identities, sizes, seed and verified checksums. It is safety metadata inside the temporary workspace, not a benchmark-results export. It is removed on ordinary successful cleanup and retained when cleanup is incomplete. No orphan-cleanup command or automatic deletion of older directories is provided. Before any manual removal, verify identities/ownership against current filesystem state and ensure no live VMBench/fio process holds those files. A prefix or metadata file alone is not sufficient authority to remove arbitrary data.

See [executed validation](DISK_VALIDATION.md).


## Asynchronous engine validation

Installation checks engine identity and runtime dependencies, while `doctor` reports compiled/setup candidates without writing data. Disk setup requires a selected writable filesystem. Automatic component modes functionally probe io_uring first and then libaio; advanced `--ioengine auto` opts into the same order. An explicit io_uring/libaio request uses only that engine and reports alternatives without substituting them.

The separate capability probe prepares and synchronizes one private 1MiB regular file using the existing protected lifecycle, then submits at most 64 direct 4K reads at QD4 per candidate, under a five-second deadline. fio must return success, the selected engine, nonzero valid completion metrics, exactly 64 completed reads/256KiB and a meaningful achieved-depth distribution. Probe results are discarded; only owned probe files are removed after the child exits. No target means validation is deferred; no engine means Disk is unsupported, while other components remain available. No security policy is modified.

Measured samples also require completed-I/O counters, no short/dropped I/O and valid engine/depth evidence. At requested QD >1, at least 50% of observations must be in depth buckets reaching the power-of-two range at half the requested depth, with a minimum lower bound of two. A QD1-dominated QD64 candidate is rejected, not published as a maximum. Displayed QD remains a requested setting and the observed distribution is reported separately. Results from different engines are not silently combined.
