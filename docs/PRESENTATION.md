# Benchmark output

VMBench displays an English-only terminal summary. Titles, labels, prompts, progress and help use ASCII. Results remain in memory during execution. There is no report export, automatic `results/` directory or report-saving API. Existing historical files are never removed. `system info` displays a compact metadata table instead of JSON.

## Options

| Option | Behavior |
| --- | --- |
| Default | Essential metrics, completion status, duration, assessment and relevant warnings |
| `--details` | Additional workload statistics, latency values and recorded telemetry in text tables |
| `--stability-cv 5` | Consistency screening threshold in percent; default 5, configurable from 0 to 100 |
| `--steal-warning-percent 1` | CPU steal warning screening threshold in percent; default 1, configurable from 0 to 100 |

Options follow the component and action. Flat YAML supports `details`, `stability-cv` and `steal-warning-percent`, with CLI options taking precedence. All benchmark commands support the same presentation options.

```bash
./vmbench cpu test --preset quick
./vmbench ram test --details
./vmbench disk test --path /data --profile randread-4k
./vmbench full test --preset standard --path /data
./vmbench cpu test --steal-warning-percent 2 --stability-cv 7
```

The previous `--lang`, `--json` and `--output` options are removed and fail explicitly. Remove `lang`, `json` and `output` from older configuration files. No replacement export format is provided. fio, native compute and iperf3 JSON are still parsed internally for accurate measurements; their raw objects are never printed as terminal output. The bootstrap's checksummed installation state remains separate from benchmark results and is still written only after successful initialization.

## Metrics

CPU displays integer and floating-point scaling separately, guest CPU steal, computation integrity errors and repetition stability. Scaling uses the existing same-workload multi-thread/single-thread throughput ratio divided by workers. It describes the benefit of additional threads, not absolute CPU speed. No fast/slow score is supplied. Details retain throughput for each workload and worker count. CPU stress completion and high expected utilization do not establish hardware reliability.

RAM displays STREAM-style TRIAD bandwidth in GB/s, dependent pointer-chase latency in ns, recorded integrity errors and stability. These are application measurements that include loop/cache effects. A conservative cache warning compares one worker's array size (`total size / workers / 3`) with four times the largest recorded CPU0 L3 cache. Smaller arrays warn that cache may dominate; unavailable cache metadata leaves main-memory bandwidth unconfirmed. This heuristic neither accounts for all cache/NUMA topology nor proves that larger datasets avoid cache effects. It never resizes the workload. Details include Copy/Scale/Add and per-worker statistics.

Disk displays the configured workload/depth/jobs, IOPS, MiB/s, average completion latency, p99 completion latency and stability. IOPS and bandwidth sum jobs within each repetition and use the existing median/CV calculator across repetitions. Bytes/s convert to MiB/s using 1,048,576; fio ns convert to ms using 1,000,000. Average latency and p99 show ranges of separate job/run values; a single value is shown when they coincide. No pooled mean or combined percentile is invented. Missing recorded means show N/A. Read/write metrics stay separate, including means and p99. Details retain each job/run's mean, p95, p99 and p99.9. QD scaling compares the existing sequence 1, 4, 16, 32, 64, using the highest recorded depth and noting p99 increases. Configured depth is not proof of effective queue depth or backend health. File creation, direct I/O, initialization writes and cleanup protections are unchanged.

Network displays protocol, direction, receiver-side throughput, recorded duration, and measured UDP loss/jitter where available. Throughput uses decimal Gbps and the existing median calculator across runs. Bidirectional metrics remain separate. UDP values use per-run ranges, without averaging percentages across packet counts. Unmeasured fields are omitted, including RTT. Endpoint throughput does not establish forwarding PPS. Server summaries show the configured endpoint and session state without inventing throughput.

A full test ends with one combined table and totals for completed, skipped, failed and not-run modules. An earlier failure does not turn the remaining modules into successful tests. Partial observations remain labelled; errors retain unsuccessful exit codes.

## Consistency and warning rules

Underlying statistical calculations are unchanged. One repetition shows CV as N/A in details and Stability as Insufficient data. Two repetitions may display mathematically defined CV with a limited-evidence label, but cannot establish Stable or Variable. At least three observations are required for every primary metric before interpreting consistency. The largest valid selected CV is compared with `--stability-cv`; CV must be finite, nonnegative, with a positive finite mean. This heuristic is not a confidence guarantee or hardware speed grade.

CPU consistency covers the separate integer and floating-point throughputs; RAM covers TRIAD and dependent access time; disk covers each displayed direction's IOPS, and each recorded depth for QD scaling. A single insufficient-data stability message replaces repeated repetition warnings in the default view.

Default warnings retain failures, incomplete/invalid measurements, integrity problems, disk safety notices, cache uncertainty and material steal observations. Identical and known equivalent messages are deduplicated. Critical warnings and errors have no arbitrary display cap. Unknown warning messages stay visible so future engine or safety errors are not silently hidden. General quick-test and environment caveats appear only under Measurement Notes in details.

Steal is always displayed when measured. A positive whole-run steal percentage at or above `--steal-warning-percent` triggers an actionable host-contention notice. The initial 1% threshold is a configurable screening rule, not a universal performance standard; zero shows warnings for any positive observation. Tiny values below the selected threshold do not imply a problem. Telemetry covers the whole run, including setup, not individual kernels. Unavailable data is N/A, never an invented zero.

## Terminal and startup behavior

ASCII separators and aligned tables work with redirected output. Non-ASCII characters in external messages or metadata are escaped; terminal control characters are removed. No locale or terminal settings are changed. Optional green/red/amber status colors require a suitable character-device terminal with a nonempty `TERM`. `NO_COLOR`, redirection and `TERM=dumb` disable colors. Green denotes successful execution, not superior hardware.

Progress goes to stderr only at module boundaries and actual engine launches. Interactive terminals refresh one phase line; redirected terminals show concise transitions. There are no guessed percentages, background polling or updates inside engine measurement timers. Rendering and interpretation happen after measurements.

`init.sh` and `start.sh` retain all three version aliases and their existing source/binary hash checks. A change under `cmd/` or `internal/`, including presentation code, invalidates the CLI source manifest; startup rebuilds the outdated application and reuses unchanged valid engines. Initialization still performs no stress benchmarks.

See [fixture examples](RESULT_EXAMPLES.md). They demonstrate the renderer with synthetic values, not measurements of this host.
