# Automatic benchmark modes

```bash
./start.sh                 # Component -> mode -> essential target -> results
./vmbench cpu quick --non-interactive
./vmbench ram standard --non-interactive
./vmbench disk max --path /data --non-interactive --ack-max
./vmbench network max --target 10.0.0.20 --non-interactive --ack-max
```

The main menu offers CPU, RAM, DISK, NETWORK and Exit. Each component opens QUICK, STANDARD and MAX, with Back (`0`). Only the selected component executes; CPU/RAM need no manual worker or dataset settings. Disk prompts for an existing directory, validates permissions/symlinks/free space without creating data, and allows retry or Back. Network prompts for an unconfigured remote IP, validates it and displays server instructions. After success or failure, Back to Main Menu allows a new explicit selection. Invalid selections never launch workloads. EOF or Ctrl+C ends navigation; cancellation during work stops/reaps the engine and preserves disk cleanup.

Direct component commands share the same automatic parser and controller. `--path` aliases `--disk-path` for Disk; `--target` aliases `--network-peer` for Network. Noninteractive selected Disk/Network fails if its target is missing. Shared configuration cannot make a selected component run other modules; unrelated endpoints and write-profile settings are ignored. Selected results and resource ceilings omit unrelated modules.

Legacy top-level `quick`, `standard` and `max` remain explicit combined CPU/RAM/Disk/Network runs, with missing endpoints skipped; they are never the main menu. The existing advanced component commands (`cpu test`, `cpu stress`, `ram test`, `ram stress`, `disk test`, `network test`, `network server`, `full test`) and `--preset quick|standard|precision` remain available. Use `COMPONENT MODE --help` for automatic options. Results and intermediate observations stay in memory. There is no automatic report, GUI, service, database or raw JSON output.

## Mode policy

| Mode | Exploratory measurement per workload | Repetitions | Confirmation | Search |
| --- | --- | --- | --- | --- |
| QUICK | 1 second, no warmup | 1 | None | CPU single/available workers; selected RAM workers; one read disk depth; one TCP stream/TX |
| STANDARD | 3 seconds, 1 second warmup | 3 | Fresh 3-second measurements, 3 repetitions | CPU/RAM single and available workers; disk QD1/8/32 within limits; TCP 1/2/4 streams and separate TX/RX |
| MAX | 2 seconds, 1 second warmup | 2 | Fresh 5-second measurements, 3 repetitions | Adaptive concurrency searches, refinement and independent workload winners |

Times apply to each native workload or fio/iperf invocation, not the whole command. The unchanged native CPU executable measures Integer and FP together; its exploratory observations are shared by the two optimizers. RAM invokes Copy, Scale, Add, TRIAD and dependent pointer chasing. Its initialization, warmup and measurement phases retain the engine's existing behavior.

`--sample-seconds N` overrides measurement duration for functional checks or longer evaluation. MAX confirmations always use at least N+1 seconds. Default full-run wall time is capped at 30 minutes (`--max-duration`, maximum 24 hours), including detection and preparation after interactive configuration. The controller cancels and waits for the active engine when this budget expires. A completed short run is not representative infrastructure qualification.

## Detection and allocation

Detection reads Linux affinity, visible logical CPUs, cgroup v1/v2 mount and membership data, CPU quotas/cpusets, architecture/instruction flags, cache sharing domains, NUMA nodes, guest total/available memory, memory pressure/swap counters, interfaces and reported link speed. Missing information is labelled unavailable. A reported interface speed is not assumed to describe a VM's end-to-end network path.

Real RAM confirmation also samples guest swap counters before/after execution and warns if swap activity increases. These are guest observations, not proof that the benchmark alone caused the activity.

Worker ceiling is `min(configured ceiling, ceil(effective CPU capacity))`; capacity accounts for affinity and restricted ancestors. Fractional CPU quotas may require a final partially scheduled worker. Memory headroom uses **each ancestor's own current usage** with its hard/high limit, rather than subtracting only leaf usage from an ancestor limit. Missing usage for a restricted memory group disables automatic allocation.

The RAM dataset is at most one quarter of available headroom after keeping 256MiB aside, and at most three quarters of the configured RAM allocation ceiling. This leaves room for the unchanged native pointer-shuffle scratch array (up to another third of the dataset), plus conservative guest headroom. QUICK additionally caps the dataset at 256MiB. Swap history or PSI avg10 above 1% halves the allocation. History alone does not establish current swapping. Less than 16MiB safe dataset skips RAM. Current headroom is checked again before every real RAM invocation; external allocations can still race a capacity check.

An aggregate cache size is deduplicated by cache sharing domain. When the dataset is not over four times aggregate last-level cache, or cache metadata is unavailable, the main-memory interpretation is explicitly unconfirmed. The native kernel initializes arrays in its parent thread; no NUMA policy, affinity, governor or kernel tuning is forced. Detected multiple NUMA nodes therefore carry a placement limitation. CPU confirmation records guest steal and resolved cgroup throttling changes; relevant observations warn about measurement conditions.

## Bounded optimization

CPU MAX searches 1, 2, 4, ... through the effective worker ceiling, then tries nearby worker counts (approximately +/-25%) around the promising point. Integer and FP select separate winners and scaling efficiencies. RAM uses the same thread search with a fixed safe dataset and TRIAD bandwidth as the objective. Copy/Scale/Add, dependent access latency and computation integrity remain available; no universal CPU score or absolute speed grade is invented.

For disk MAX, each independent workload starts with one job and QD1/2/4/8/16/32/64 through the configured depth/outstanding limit. It then explores 1/2/4/8 jobs through the effective job ceiling at the promising depth, reducing depth where necessary to respect outstanding I/O. Neighboring depths around the best measured combination refine the search. This is a bounded adaptive path, not an exhaustive Cartesian grid. Random Read 4K optimizes IOPS; Sequential Read 1MiB independently optimizes MiB/s. QD, jobs and their product are always bounded. Supported async fio engines receive a filesystem-specific functional probe; io_uring is preferred, then libaio. A real invocation must succeed and fio must record the selected engine. An execution failure is reported, not silently converted into another engine or workload.

TCP MAX searches 1/2/4/8/16 streams through the configured ceiling, separately for transmit and receive. A configured literal, trusted remote unicast peer is required. Loopback, this guest's own interface addresses, unspecified and multicast addresses are rejected. In legacy combined runs, no peer means SKIPPED, without localhost substitution; selected Network requires a peer. The controller never starts a server or UDP flood. Aggregate offered payload rate defaults to 1Gbit/s, divided across streams with iperf3 TCP `-b` pacing (configured ceiling at most 100Gbit/s). Measured throughput may be limited by this ceiling; TCP throughput is not PPS or RTT. Automatic UDP offered-load ceiling is zero: UDP probing is not implemented in these modes. Advanced component commands retain their existing UDP controls.

At each increasing search step, relative gain is compared with the larger of the configured improvement threshold (default 5%) and twice the observed CV. Two consecutive small gains with nondecreasing p99 (or without a latency metric for CPU/RAM/TCP) can stop that axis. High variability prevents this early-stop heuristic. It is **not proof of physical saturation**, a confidence interval, or proof of the hardware maximum. CPU/RAM refinement, extra jobs and remaining axes can still run.

Up to three promising configurations receive fresh confirmation, prioritizing less variable exploration results. The final choice uses the highest confirmation median among candidates with at least three valid repetitions and CV <=10% by default. It never uses the highest individual sample as the maximum. `--max-cv` and `--saturation-improvement` are fractions, bounded to (0, 0.5]. If none confirms, Disk retains usable observations with UNCONFIRMED confidence and a reliability warning, independently for each workload. Confirmed winners retain first priority. Without one, Disk prefers the highest median from a valid trial with at least three repetitions, otherwise the highest individually measured sample, explicitly labeled as a single observation. Only workloads with no valid data show N/A. The other modules retain their existing confirmed-result policy. See [Disk retention and selection](DISK-RESULTS.md). A boundary winner warns that higher performance may be possible. All MAX labels mean **maximum observed within this search and these limits**. Latency is reported alongside throughput; there is no implicit latency SLA or guarantee of hardware steady state.

## Disk consent and private dataset

An explicit/configured target must already exist and pass the existing secure filesystem validation. Without one, an interactive terminal prompts for a directory; noninteractive selected Disk fails. Legacy combined runs skip Disk without a target. The filesystem, mount and system-filesystem warning are shown before load acknowledgment. No arbitrary mount point is created and no raw block device is benchmarked. MAX always warns about high load and requires an interactive yes or per-invocation `--ack-max`.

Read profiles also write preparation data. High-intensity write tests require **both** `--include-writes --confirm-writes`, independently of MAX acknowledgment. They add separately labelled Random Write 4K, Sequential Write and Mixed 70/30 results. Configuring `include-writes: true` cannot replace per-invocation write confirmation. Mixed read/write throughput stays separate; latency values are ranges of separate job/direction observations, never averaged percentiles.

The total prepared dataset is at most the configured disk-data ceiling and one quarter of free space remaining after the reserve and metadata allowance. QUICK further caps it at 128MiB; other modes also cap it at available guest memory when known. Job count is reduced if needed to keep at least 16MiB per job. Per-job sizes round down to MiB and are available with the total in `--details`. Active jobs each use their own file; smaller job candidates use a subset of the prepared files. The entire dataset is prepared and fully verified **once per disk module**, reused across all depths/jobs/profiles, then removed after the last process exits. Preparation is outside fio measurement runtime/metrics. Small datasets cannot eliminate backend caching.

The existing crypto-random private workspace, seeded full real-data generation, inherited file descriptors, no-symlink directory walk, inode/ownership validation, no recursive/glob cleanup and minimum 1GiB reserve remain. A stronger configured free reserve is checked during preparation as well. fio cannot create/delete its workload files. Each result must include the recorded async engine and achieved depth distribution; `--details` shows observed **buckets**, not an invented mean or a claim that requested QD was achieved. Underachieved depth is visible. For requested QD above one, at least 50% of observations must lie in buckets whose lower bound reaches the power-of-two range at half the requested depth (minimum two); otherwise the result is rejected as a measurement limitation. These buckets never establish an exact achieved mean or exact requested depth.

Timeout, SIGINT/SIGTERM and failures stop/reap the active child before identity-checked cleanup. Cleanup failures preserve the original error, show failure and stop further modules. SIGKILL/host failure can leave ownership metadata; there is no automatic deletion of old workspaces. Same-UID/privileged race and filesystem/backend limitations remain as described in [DISK.md](DISK.md). The fio [iodepth documentation](https://fio.readthedocs.io/en/latest/fio_doc.html#cmdoption-arg-iodepth) explains why achieved depth must be inspected.

Named fio jobs explicitly use `thread=1`. fio's default forked workers [call setsid](https://github.com/axboe/fio/blob/fio-3.39/backend.c), which allowed a worker to survive the existing process-group cancellation. Thread jobs share the owned fio process and its lifecycle. The job count, independent files, selected async engine, direct I/O, QD and per-job JSON+ statistics remain explicit. This backend choice is part of the measurement configuration; comparisons with earlier process-job runs should account for it.

## Configuration and defaults

Normal use needs no tuning file. Optional `.vmbench/auto.yaml` in the caller's working directory is loaded if present; `--config FILE` selects another flat file. Explicit flags override it. [Example automatic configuration](../config/auto.yaml) can save a disk target, remote peer and limits. `ack-max` and `confirm-writes` are rejected in files so consent cannot be silently replayed. The advanced `config/default.yaml` continues to configure component commands, not automatic modes.

| Ceiling | Default | Hard accepted bound |
| --- | --- | --- |
| CPU workers | 32 and effective capacity | 128 and effective capacity |
| RAM allocation including native shuffle scratch | 2GiB, further reduced by current headroom | Size parser limit; conservative fraction still applies |
| Total prepared disk data | 2GiB, further reduced by space/mode/memory | Size parser limit; reserve and quarter-space limit still apply |
| Free disk reserve | 1GiB plus metadata headroom | Cannot be below 1GiB |
| Disk QD / jobs / total outstanding | 64 / 4 / 128 | 64 / 8 / 512 |
| TCP streams / offered rate | 8 / 1Gbit/s aggregate | 16 / 100Gbit/s aggregate |
| Complete run duration | 30 minutes | 24 hours |

An ordinary engine/module error marks that module failed and permits independent later modules; cancellation or incomplete disk cleanup stops continuation. Overall failure retains a nonzero exit status. Unconfirmed stable maxima are reported as limitations, not fabricated measurements. `--details` shows searched configurations, per-trial medians/CV/durations and detected metadata; the default summary shows essential measured metrics, applicable latency, cleanup and material warnings. Search ranges and resource ceilings stay in details. The interactive View Details option renders retained results without rerunning benchmarks.

`start.sh` already hashes all Go files recursively under `cmd/` and `internal/`; changes to this controller invalidate only the CLI build while valid native/third-party engines are reused. Initialization still performs no benchmarks. See [executed automatic-mode validation](AUTO_VALIDATION.md).

## Native latency correctness repair

Real Linux checks exposed an existing optimizer issue: the otherwise-unread atomic sink could be eliminated, taking the pointer-chase loads with it. This produced invalid dependent-access latency. The same shuffled-cycle algorithm now uses volatile dependent reads and verifies that a full cycle returns to the starting index. Copy/Scale/Add/TRIAD and CPU kernels retain their algorithms. The optimized known-cycle/corrupted-cycle regression is in [tests/test_native.py](../tests/test_native.py). Rebuild the native executable after updating; prior affected latency values are not comparable. This repair does not qualify the native kernels against independent hardware reference measurements.


Installation doctor performs no filesystem I/O and never turns setup-only AIO success into functional validation. Actual Disk selection uses a separate private 1MiB probe dataset: 64 direct 4K reads at QD4, at most five seconds per candidate. Its data and results do not enter optimizer statistics. Every measured sample additionally requires nonzero completed `total_ios` / `io_bytes`, zero short/dropped I/O and valid achieved-depth evidence. An execution failure does not select another engine halfway through the search. The selected engine is retained in the detailed view and lifecycle metadata.
