# Benchmark output

Benchmark commands (`cpu test`, `cpu stress`, `ram test`, `ram stress`, `disk test`, `network test`, `network server`, `full test`) share these presentation options:

| Option | Output |
| --- | --- |
| Default | Compact summary, essential metrics, assessment, important warnings, full report path |
| `--details` | Summary plus technical tables and relevant recorded telemetry; no raw JSON dump |
| `--json` | One complete report object on stdout; progress and diagnostics on stderr |
| `--lang vi` | Vietnamese explanations (default) |
| `--lang en` | English explanations |
| `--stability-cv 5` | Consistency heuristic threshold, in percent; configurable from 0 to 100 |

Options follow the component and action. They also work in the existing flat YAML configuration (`lang: en`, `details: true`, `stability-cv: 5`). CLI options retain precedence over configuration. `--details` and `--json` cannot be combined. `system info` retains its existing JSON metadata output. Global help and version retain their existing interfaces.

```bash
./vmbench cpu test --preset quick --lang en
./vmbench ram test --details --lang vi
./vmbench disk test --path /data --json > disk-report.json
./vmbench full test --path /data --lang en
```

The existing JSON report fields and raw engine results remain available under `results/`, or the selected `--output` directory. Settings gain the presentation options, and `Results.execution_status` records individual module states. Warnings include generated measurement caveats in English; human output translates known explanations. Raw engine errors remain unchanged. JSON mode preserves reports for validation failures when configuration can be parsed, and exits unsuccessfully on genuine errors. A report-save failure is displayed as a failure without claiming a saved path.

## Reading assessments

Completed describes execution, not hardware quality. Stable and Variable describe repetition consistency under a configurable heuristic. Needs investigation indicates recorded integrity errors. Insufficient data indicates missing metrics or insufficient evidence. Failed remains visible, including partial observations; an incomplete full run distinguishes failed, skipped and not-run modules.

No universal speed score or fast/slow classification is supplied. There is no baseline-import interface yet. Comparing systems requires a suitable baseline with matching workloads, units, configuration and environment.

The existing statistics calculations are unchanged. One observation shows CV as N/A and cannot establish stability. Two observations may show CV in details, explicitly marked as limited evidence; they cannot earn Stable or Variable. With at least three observations for every selected primary metric, the highest CV is compared with `--stability-cv` (default 5%). CV must be finite, nonnegative and have a positive finite mean. Stable means CV is at or below the threshold; it does not provide a confidence interval or prove repeatability beyond those observations. Primary CPU consistency covers integer and floating-point throughput separately; RAM covers TRIAD throughput and dependent access time; disk covers each displayed direction's IOPS, and each recorded depth for QD scaling.

## Metric definitions and caveats

CPU scaling is the existing matching-kernel multi-worker/single-worker throughput ratio divided by worker count. Integer and floating-point efficiencies stay separate. Missing single-worker comparison produces N/A. High efficiency does not rank absolute CPU speed. CPU busy/steal percentages and cgroup throttle deltas cover the complete run, including setup, rather than individual kernels. Missing steal counters are unavailable, not zero. High busy time during stress testing is expected; stress completion does not prove hardware reliability.

RAM bandwidth uses recorded STREAM-style TRIAD GB/s at the highest completed worker count. Access time uses the recorded dependent pointer-chase mean per worker in ns, including loop effects. These are application measurements rather than isolated DRAM properties. A conservative cache warning compares one worker's array size (`total size / workers / 3`) against four times the largest recorded CPU0 L3 cache. Smaller arrays trigger a warning; missing cache metadata leaves main-memory bandwidth unconfirmed. This heuristic neither accounts for all NUMA/cache topology nor proves that a larger dataset avoids cache effects. It does not resize the workload. Copy/Scale/Add and per-worker results remain in details and JSON. Integrity checks report recorded errors only.

Disk IOPS and bytes/s are summed across jobs within each recorded repetition, then summarized with the existing median/CV calculator. Throughput converts bytes/s to MiB/s using 1,048,576; completion latency converts ns to ms using 1,000,000. Read and write remain separate. The displayed p99 is the range of individual job/run p99 values, never an average or a reconstructed combined percentile. Details retain separate p95, p99 and p99.9 values. Invalid fio jobs are excluded from primary metrics and visibly fail the module. QD scaling maps the recorded run order to the existing configured depths 1, 4, 16, 32, 64 and repetitions; the summary compares QD1 with the highest recorded depth, shows throughput scaling and warns when the highest observed job/run p99 increases at display precision. These are configured depths, not evidence of effective achieved queue depth or backend health.

Network throughput is the median recorded receiver-side bits/s converted to decimal Gbps. Duration is the recorded receiver duration, not the requested duration. Bidirectional results retain each direction's throughput and duration. UDP loss and jitter show per-run ranges, separately by direction; percentages are not averaged across incompatible packet counts. Unsupported/missing values show N/A. Details explicitly say RTT was not measured. iperf3 endpoint throughput does not establish maximum forwarding PPS. Server commands report the listening endpoint and session status without inventing throughput.

## Terminal behavior

Human output uses aligned tables and no box-drawing characters. Optional green/red/amber status colors require a character-device terminal, a nonempty suitable `TERM`, and no `NO_COLOR` variable. Green means execution completed, not superior performance. Redirected output and `TERM=dumb` use plain text. The simple warning section shows up to three distinct important warnings and points to the full report for additional warnings; details show all distinct warnings. Errors are always displayed.

Progress goes to stderr and updates only at module boundaries and actual external-engine launches. Interactive terminals refresh a single phase line; redirected terminals print phase transitions. There are no estimated percentages, background polling or progress updates inside engine measurement timers. Rendering and interpretation occur after measurements. Full benchmarks end with one compact table; `--details` expands completed and partial module observations.

See [fixture examples](RESULT_EXAMPLES.md) for CPU, RAM and disk output. These illustrate formatting and interpretation, not measurements of the development host.
