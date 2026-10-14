# Illustrative benchmark summaries

These deterministic fixtures demonstrate the actual renderer. Every number is synthetic; no benchmark was run on this host. Repeated fixture values demonstrate formatting, not evidence about hardware.

## CPU (English, three repetitions)

```text
================================================
VMBENCH CPU TEST
================================================
Status: COMPLETED
Duration: 12.00 seconds

PERFORMANCE
------------------------------------------------
Integer scaling efficiency   : 90.0% (4 threads)
FP scaling efficiency        : 95.0% (4 threads)
CPU steal time               : 0.00%
Computation errors           : 0
Stability                    : Stable

ASSESSMENT
------------------------------------------------
Measurements were consistent across the completed runs. No computation errors were reported.
Scaling measures the benefit of additional threads, not absolute CPU speed.
================================================
```

## RAM (English, three repetitions)

```text
================================================
VMBENCH MEMORY TEST
================================================
Status: COMPLETED
Duration: 12.00 seconds

PERFORMANCE
------------------------------------------------
Memory bandwidth (TRIAD)            : 28.60 GB/s
Memory latency (dependent access)   : 85.00 ns
Stability                           : Stable
Integrity errors                    : 0

ASSESSMENT
------------------------------------------------
Measurements were consistent across the completed runs. No memory integrity errors were reported.
================================================
```

## Disk (English, three repetitions)

```text
================================================
VMBENCH DISK TEST
================================================
Status: COMPLETED
Duration: 12.00 seconds

PERFORMANCE
------------------------------------------------
Workload                          : Random Read 4K; QD32 x 1 jobs
IOPS                              : 85,000 IOPS
Throughput                        : 332.03 MiB/s
Average latency (job/run range)   : 0.380 ms
P99 latency (job/run range)       : 1.240 ms
Stability                         : Stable

ASSESSMENT
------------------------------------------------
Measurements were consistent across the completed runs.
================================================
```
