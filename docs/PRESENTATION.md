# Benchmark output

Automatic component `quick`, `standard` and `max` commands show essential measured metrics. A successful Disk MAX read benchmark result has two sections: RANDOM READ 4K (IOPS and job/run P99 range) and SEQUENTIAL READ (MiB/s and job/run P99 range), plus target, execution status and temporary-file cleanup. MAX means validated maximum observed within tested limits. QUICK is labeled preliminary; STANDARD includes a concise stability indicator. Unavailable metrics are omitted; incomplete/unconfirmed measurements retain warnings. CPU, RAM and Network use the same concise style, displaying only tested metrics.

`--details` retains selected configurations, engine, dataset, confirmation medians/CV and repetitions, job/run latency tables including P95/P99, observed depth evidence, search trials/bounds and resource ceilings. Progress hides successful search/configuration and capability telemetry by default and exposes it with `--details`. Unknown notices and terminal failures remain visible. Missing peers/targets are skipped, and invalid results and cleanup failures remain visible.

Optional automatic `--disk-p99-warning-ms N` (local YAML `disk-p99-warning-ms: N`) is disabled at `0`. A positive finite threshold in milliseconds warns if the highest job/run P99 in the displayed workload exceeds it. Warnings identify the workload and threshold, describe latency under load and do not classify hardware health. This presentation setting does not change the workload, search policy or measurement. No universal latency threshold is imposed.

Default P99 ranges preserve all observed job/run endpoints without averaging percentiles. Display units use seconds when the entire range is at least 1,000 ms, microseconds when the entire range is below 1 ms, and milliseconds otherwise. Rounding changes display precision only. Material cache, search boundary, system-filesystem, instability, incomplete-run and cleanup caveats remain visible; only known informational methodology/metadata notices move to details. Unknown warnings are retained.

Advanced `test`/`stress` commands use the same compact header and essential metrics. `--details` preserves their configuration, full statistics and recorded telemetry. The interactive post-run View Details option renders the retained in-memory result without repeating any workload.

VMBench displays an English-only terminal summary. Titles, labels, prompts, progress and help use ASCII. Results remain in memory during execution. There is no report export, automatic `results/` directory or report-saving API. Existing historical files are never removed. `system info` displays a compact metadata table instead of JSON.

## Options

| Option | Behavior |
| --- | --- |
| Default | Essential measured metrics, completion/partial/failure status, important warnings and disk cleanup |
| `--details` | Additional workload statistics, latency values and recorded telemetry in text tables |
| `--stability-cv 5` | Consistency screening threshold in percent; default 5, configurable from 0 to 100 |
| `--steal-warning-percent 1` | CPU steal warning screening threshold in percent; default 1, configurable from 0 to 100 |

Options follow the component and action. Flat YAML supports `details`, `stability-cv` and `steal-warning-percent`, with CLI options taking precedence. All benchmark commands support `--details`; the CV/steal flags in this table belong to advanced `test`/`stress` commands. Automatic modes retain `--max-cv` and their existing contention warning rule.

```bash
./vmbench cpu test --preset quick
./vmbench ram test --details
./vmbench disk test --path /data --profile randread-4k
./vmbench full test --preset standard --path /data
./vmbench cpu test --steal-warning-percent 2 --stability-cv 7
```

The previous `--lang`, `--json` and `--output` options are removed and fail explicitly. Remove `lang`, `json` and `output` from older configuration files. No replacement export format is provided. fio, native compute and iperf3 JSON are still parsed internally for accurate measurements; their raw objects are never printed as terminal output. The bootstrap's checksummed installation state remains separate from benchmark results and is still written only after successful initialization.

## Metrics

CPU displays defined integer and floating-point throughput, with each workload's scaling efficiency alongside it, measured guest CPU steal and concise stability. Nonzero integrity errors are warnings and prevent retained invalid observations from appearing successful. Routine zero counters, worker searches, topology, affinity and quota remain in details. Scaling uses the existing same-workload multi-thread/single-thread throughput ratio divided by workers. It describes the benefit of additional threads, not absolute CPU speed. No fast/slow score is supplied. Details retain per-run measurements, elapsed times, dataset bytes, recorded integrity counts and worker configurations. CPU stress completion and high expected utilization do not establish hardware reliability.

RAM displays STREAM-style TRIAD bandwidth in GB/s and measured dependent pointer-chase latency in ns/access, with concise STANDARD stability. Copy/Scale/Add values are not mixed into the displayed TRIAD bandwidth. Nonzero integrity errors and unsafe resource pressure remain warnings; routine counters and allocation information stay in details. These are application measurements that include loop/cache effects. A conservative cache warning compares one worker's array size (`total size / workers / 3`) with four times the largest recorded CPU0 L3 cache. Smaller arrays warn that cache may dominate; unavailable cache metadata leaves main-memory bandwidth unconfirmed. This heuristic neither accounts for all cache/NUMA topology nor proves that larger datasets avoid cache effects. It never resizes the workload. Details include Copy/Scale/Add and per-worker statistics.

Disk defaults show random IOPS or sequential MiB/s, plus the applicable separate job/run P99 range. Optional mixed read/write profiles retain directional ranges. The automatic Disk read module has the agreed two workload sections, target and cleanup status. Engine, depth/jobs, dataset, preparation time (when recorded), mean latency, per-job P95/P99 and CV/repetitions remain in details. IOPS and bandwidth sum jobs within each repetition and use the existing median/CV calculator across repetitions. Bytes/s convert to MiB/s using 1,048,576; fio ns convert to ms using 1,000,000. No pooled mean or combined percentile is invented. Advanced QD scaling defaults to the observation at the highest tested depth, without claiming it is the best-performing depth; details retain the complete scaling comparison. Configured depth is not proof of effective depth or backend health. The [disk lifecycle](DISK.md) and identity-checked cleanup are unchanged.

Network displays receiver-side throughput in Mbps below 1 Gbps and Gbps otherwise. A single tested direction is labeled Upload or Download; both appear only when both were tested. Available sender `retransmits` counters remain integer counts; independent runs use a min/max range, never a sum or packet-loss percentage. Missing/null counters are omitted and recorded zero is preserved. Partially recorded counters are labeled as such. Advanced UDP results display only measured packet loss and jitter, keeping directions separate and preserving per-run ranges. TCP loss percentages, inferred RTT and forwarding PPS are never invented. Invalid/missing receiver duration or throughput produces an incomplete-transfer warning and PARTIAL/FAILED presentation. Optional telemetry errors remain visible without changing engine validation or execution. Details retain durations, per-run results, configuration and counters. Server summaries show the listening endpoint and session state.

A full test ends with one combined table and totals for completed, skipped, failed and not-run modules. An earlier failure does not turn the remaining modules into successful tests. Partial observations remain labelled; errors retain unsuccessful exit codes.

## Consistency and warning rules

Underlying statistical calculations are unchanged. One repetition shows CV as N/A in details and Stability as Insufficient data. Two repetitions may display mathematically defined CV with a limited-evidence label, but cannot establish Stable or Variable. At least three observations are required for every primary metric before interpreting consistency. The largest valid selected CV is compared with `--stability-cv`; CV must be finite, nonnegative, with a positive finite mean. This heuristic is not a confidence guarantee or hardware speed grade.

CPU consistency covers the separate integer and floating-point throughputs; RAM covers TRIAD and dependent access time; disk covers each displayed direction's IOPS, and each recorded depth for QD scaling. A single insufficient-data stability message replaces repeated repetition warnings in the default view.

Default warnings retain failures, incomplete/invalid measurements, integrity problems, disk safety notices, cache uncertainty and material steal observations. Identical and known equivalent messages are deduplicated. Critical warnings and errors have no arbitrary display cap. Unknown warning messages stay visible so future engine or safety errors are not silently hidden. General quick-test and environment caveats appear only under Measurement Notes in details.

Steal is always displayed when measured. A positive whole-run steal percentage at or above `--steal-warning-percent` triggers an actionable host-contention notice. The initial 1% threshold is a configurable screening rule, not a universal performance standard; zero shows warnings for any positive observation. Tiny values below the selected threshold do not imply a problem. Telemetry covers the whole run, including setup, not individual kernels. Unavailable data is omitted from the default view; recorded zero remains visible. Automatic steal ranges cover the retained native invocation observations; advanced steal covers the recorded whole run. Neither is a workload-specific CPU speed score.

## Terminal and startup behavior

ASCII separators and aligned tables work with redirected output. Non-ASCII characters in external messages or metadata are escaped; terminal control characters are removed. No locale or terminal settings are changed. Optional green/red/amber status colors require a suitable character-device terminal with a nonempty `TERM`. `NO_COLOR`, redirection and `TERM=dumb` disable colors. Green denotes successful execution, not superior hardware.

Progress goes to stderr only at module boundaries and actual engine launches. Interactive terminals refresh one phase line; redirected terminals show concise transitions. There are no guessed percentages, background polling or updates inside engine measurement timers. Rendering and interpretation happen after measurements.

`init.sh` and `start.sh` retain all three version aliases and their existing source/binary hash checks. A change under `cmd/` or `internal/`, including presentation code, invalidates the CLI source manifest; startup rebuilds the outdated application and reuses unchanged valid engines. Initialization still performs no stress benchmarks.

See [fixture examples](RESULT_EXAMPLES.md). They demonstrate the renderer with synthetic values, not measurements of this host.
