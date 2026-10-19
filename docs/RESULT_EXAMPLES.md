# Illustrative benchmark summaries

All components share a compact summary. The following **synthetic examples** illustrate the actual formatting; these are not measurements of this host. Integrity failures, unsafe pressure, incomplete measurements and cleanup failures remain visible.

CPU displays workload throughput and scaling together. Scaling is thread efficiency, not an absolute compute score; measured steal is separate:

```text
========================================
       VMBENCH - CPU MAX
========================================
Status: COMPLETED
Results: Maximum observed within tested limits

Integer Compute: 3,600 defined iterations/s (scaling 90.0 %)
Float Compute  : 1,900 defined iterations/s (scaling 95.0 %)
CPU Steal      : 0.02 %
Stability      : Confirmed
========================================
```

RAM keeps the bandwidth workload explicit and shows measured dependent-access latency:

```text
========================================
       VMBENCH - RAM MAX
========================================
Status: COMPLETED
Results: Maximum observed within tested limits

Memory Bandwidth: 28.60 GB/s (TRIAD)
Memory Latency  : 85.00 ns/access
========================================
```

Disk shows random-read IOPS and throughput, sequential throughput, per-workload P99 ranges, and independent confidence labels:

```text
========================================
       VMBENCH - DISK MAX
========================================
Status: COMPLETED
Results: Maximum observed within tested limits
Target: /data

RANDOM READ 4K
----------------------------------------
IOPS           : 15,083
Throughput     : 58.92 MiB/s
P99 Latency    : 18.5 - 25.6 ms (job/run range)
Confidence     : CONFIRMED

SEQUENTIAL READ
----------------------------------------
Throughput     : 95.61 MiB/s
P99 Latency    : 2.02 - 2.84 s (job/run range)
Confidence     : CONFIRMED

----------------------------------------
Temporary Files: Cleaned
========================================
```

Network shows only tested directions. Here both were tested; QUICK normally tests upload only and prints `Direction: Upload`. Rates below 1 Gbps use Mbps. Available TCP retransmits show a count or a per-run range, never a packet-loss percentage:

```text
========================================
       VMBENCH - NETWORK MAX
========================================
Status: COMPLETED
Results: Maximum observed within tested limits

TCP Upload          : 842.00 Mbps
Upload Retransmits  : 3 - 5 (per-run range)
TCP Download        : 1.20 Gbps
Download Retransmits: 3 - 5 (per-run range)
========================================
```


These fixtures demonstrate presentation only. Real safety and quality warnings appear when warranted. Disk observations remain visible when unconfirmed; see [retention and selection](DISK-RESULTS.md). Use `--details` or View Details for retained diagnostics; neither view creates a JSON report.
