# fio counter accounting and result validation

## Root cause and reviewed versions

VMBench previously treated `io_bytes == total_ios * block_size` as an unconditional invariant. That rejects successful asynchronous workloads when requests cross fio's warmup statistics reset. The supplied fio 3.36 excerpt has 124,276,736 read bytes, 30,340 operations, a 4,096-byte block, and no job/short/dropped-I/O errors: its difference is **+4,096 bytes**, within the outstanding-request bound for QD2 when ramp-up is enabled. The same numbers without verified ramp/workload evidence are rejected.

Upstream fio tags **3.35, 3.36, 3.37, 3.38 and 3.39** were inspected. The relevant accounting/reset behavior is the same across those tags; unrelated priority/timestamp/reporting changes do not change the derivation below. 3.39 is the repository's pinned engine. Other versions can still pass exact counter validation; unexplained differences on unreviewed versions are rejected, with their version and counters in the error. Extending this list requires reviewing the upstream accounting paths, not increasing a tolerance.

## Upstream evidence

- [`td_io_queue`, fio 3.35](https://github.com/axboe/fio/blob/fio-3.35/ioengines.c) and [3.36](https://github.com/axboe/fio/blob/fio-3.36/ioengines.c): `total_io_u` increments when a request is queued or synchronously completed. It is not exclusively a completed-byte counter. The asynchronous commit can subsequently update submission statistics.
- [`io_completed` / `io_u_queued`, fio 3.35](https://github.com/axboe/fio/blob/fio-3.35/io_u.c) and [3.36](https://github.com/axboe/fio/blob/fio-3.36/io_u.c): transferred bytes accumulate on successful completion. Submission and completion accounting can both invoke the ramp transition. Short transfers have a separate counter and remain failures in VMBench.
- [`ramp_time_over`, fio 3.36](https://github.com/axboe/fio/blob/fio-3.36/time.c): the transition occurs once and resets statistics without first draining outstanding requests. [`reset_all_stats`](https://github.com/axboe/fio/blob/fio-3.36/libfio.c) clears byte and request counters; [`reset_io_stats`](https://github.com/axboe/fio/blob/fio-3.36/stat.c) clears statistical accumulators. Requests already queued at that boundary can complete after their queued-operation count has been cleared.
- [`do_io`, fio 3.36](https://github.com/axboe/fio/blob/fio-3.36/backend.c): successful completion reaps remaining asynchronous requests. [`td_io_getevents`](https://github.com/axboe/fio/blob/fio-3.36/ioengines.c) adjusts the completion maximum to satisfy the requested minimum. Error cleanup follows a different path. Therefore normal time-based termination does **not** earn another discrepancy allowance; unsuccessful/canceled processes remain failures.
- [fio time options](https://fio.readthedocs.io/en/latest/fio_doc.html#time-related-parameters): ramp time is a lead-in before reported measurements; time-based execution loops the workload for the requested runtime. Reaching the dataset boundary during that loop is not an additional warmup reset.
- [`add_ddir_status_json`, fio 3.36](https://github.com/axboe/fio/blob/fio-3.36/stat.c): reported IOPS uses `total_ios * 1000 / runtime_ms`; reported `bw_bytes` uses integer division of `io_bytes * 1000 / runtime_ms`. These are separate performance metrics with a shared reported runtime. [`json.c`](https://github.com/axboe/fio/blob/fio-3.36/json.c) serializes floating-point values to six decimal places.

The corresponding accounting paths were compared with [3.37](https://github.com/axboe/fio/tree/fio-3.37), [3.38](https://github.com/axboe/fio/tree/fio-3.38), and [3.39](https://github.com/axboe/fio/tree/fio-3.39). This is an accounting compatibility review, not platform performance certification.

## Bound, rather than a percentage tolerance

For each independent job and active direction, let:

```
delta_blocks = io_bytes / fixed_block_bytes - total_ios
```

After the one ramp reset, operations submitted afterwards contribute both their count and eventual full-block completion. A request queued before the reset can contribute only its later completion. There can be at most `iodepth` such outstanding requests in the job. A reset detected during a completion can clear that completion's byte contribution too; it does not enlarge the bound.

Consequently, successful fixed-block jobs may have nonnegative, whole-block differences whose **sum across read and write is at most that job's requested depth**. Jobs have separate queues, so each job has its own bound. The limit is not multiplied by runtime, repetitions, dataset wraps, or direction count. Neither negative differences nor partial blocks fit this explanation. At warmup zero, there is no boundary with outstanding requests and exact equality remains mandatory.

VMBench applies this rule only to reviewed fio versions, direct I/O through `io_uring` or `libaio`, positive configured warmup, time-based measurement, independent job reporting, and matching echoed engine/workload/block/depth/ramp options. Functional capability probes are finite, have no ramp, and still require exactly 64 completed 4KiB reads. Offload mode is not covered.

## Independent validity gates and performance values

Process exit/cancellation, job error (including reported continued errors), job count, required-field presence, positive finite metrics, short/dropped I/O, completion latency percentiles, engine identity, and achieved-depth distributions remain separate validity checks. Counter multiplication and bandwidth arithmetic are checked for overflow. Read and write are validated independently before their shared outstanding budget is checked.

IOPS must match fio's count/runtime formula within its six-decimal serialization rounding and floating-point representation error. Bandwidth must match fio's integer bytes/runtime formula. These checks do not normalize the two counters to each other. VMBench retains fio's original IOPS, bandwidth in bytes/s, independent directions, and completion percentiles. Presentation converts bytes/s to MiB/s using 1,048,576 bytes/MiB.

Accepted counter observations retain actual bytes, count, block size, signed byte difference, and the applicable explanation in memory. `--details` exposes differences alongside achieved-depth evidence. Normal successful execution remains concise and writes no persistent JSON report.

## Failure diagnostics

Failures display workload, selected engine, requested depth, job, bytes by active direction, operation count, exact signed byte difference, short/dropped I/O, and fio job error. Missing/unparseable values say `unavailable`.

Opt in to retaining **failed** fio stdout with:

```bash
./vmbench disk standard --path /existing/test-directory --fio-diagnostics .vmbench/fio-failures
./vmbench disk test --path /existing/test-directory --fio-diagnostics .vmbench/fio-failures
```

No diagnostic directory/file is created for successful samples. Failed output uses an exclusive random filename and mode 0600, without overwriting existing files; a newly created directory uses mode 0700 subject to umask. Malformed or partial stdout is retained exactly as received; the `.json` extension does not guarantee the failed process produced valid JSON. If fio produced no stdout, there is nothing to retain. Retention errors are reported alongside the original workload failure. Keep diagnostics outside the private test workspace. Dataset ownership, file creation, preparation, confirmation, and cleanup rules are unchanged.

## Regression and integration tests

`tests/fixtures/fio-3.36-qd2-accounting.json` reconstructs a minimal JSON record around the user's supplied counters and IOPS. Runtime 5001ms follows that IOPS/count relationship, and bandwidth follows fio's integer formula. Option/depth and latency fields are synthetic supporting fields; this fixture is **not** represented as the complete original captured JSON. Tests exercise both 3.35 and 3.36 schemas with global/per-job options, exact counts, positive and negative differences, shared mixed-direction budgets, multiple jobs, invalid/missing fields, and real failures.

Disk MAX now visits every configured initial depth before confirmation: by default QD1, QD2, QD4, QD8, QD16, QD32, QD64. CPU/RAM/Network search policies are unchanged. A failed or canceled sweep cannot become a confirmed Disk maximum. Configured resource limits still bound the search, and existing boundary/uncertainty notes remain applicable.

An explicit integration test is available only for a disposable Linux environment:

```bash
VMBENCH_REAL_FIO=1 \
VMBENCH_FIO_BINARIES=/absolute/trusted/fio-3.35:/absolute/trusted/fio-3.36 \
TMPDIR=/disk-backed/disposable/tmp \
go test -tags integration -run '^TestRealFioAsyncAccounting$' -v ./internal/benchmark/disk -count=1
```

Run as a regular user with at least the existing 1GiB free-space reserve. This test prepares private files, exercises the original QD2 profile and QD1–64 for both asynchronous engines, tests multiple jobs/mixed read-write, injects a real invalid direct-read failure, and verifies cleanup and sentinel preservation. It does not run during initialization or ordinary `go test ./...`. An optional `VMBENCH_FIO_EVIDENCE` directory explicitly archives research samples in this integration test only; it is unrelated to normal CLI report behavior.

### Recorded execution, 2026-10-15

On a disposable Alpine 3.22.6 amd64 VM (kernel 6.12.110), UID 1000, unmodified fio 3.35/3.36 built from upstream tags and the existing pinned 3.39 binary completed the integration matrix. Each version passed both async engines at QD1, 2, 4, 8, 16, 32, 64, multiple jobs, mixed read/write, no ramp, and sentinel/cleanup checks. Deliberately misaligned direct reads failed and were rejected. These tests establish accounting behavior; their rates are not performance ratings for production storage.

The original five-second QD2 io_uring workload reproduced the mismatch:

| fio | Read bytes | total_ios | Byte difference |
| --- | ---: | ---: | ---: |
| 3.35 | 245006336 | 59814 | +8192 |
| 3.36 | 221454336 | 54065 | +4096 |
| 3.39 | 175517696 | 42850 | +4096 |

The 66 successful raw research captures contain 80 nonzero directional accounting observations. Higher depths and mixed read/write exercise the shared outstanding bound; all no-ramp captures have exact counter agreement. In particular, queued-transition resets can reach the full `iodepth` bound, so a universal `iodepth - 1` or one-block exemption would also be incorrect.
