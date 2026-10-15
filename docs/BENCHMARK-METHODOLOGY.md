# Measurement definitions and portability

[Automatic modes](AUTOMATIC.md) defines the resource plan, sample/confirmation strategy and ceilings. [Disk lifecycle](DISK.md) defines ownership, preparation and cleanup. [Presentation](PRESENTATION.md) defines units, statistical summaries and honest unavailable/unstable status.

CPU: deterministic xorshift64 integer iterations and dependent floating-point multiply/add iterations, verified known answers, elapsed monotonic time and controlled worker concurrency. No universal hardware rating is derived. stress-ng remains an explicit stress tool; its bogo-ops are not substituted for these native measurements.

RAM: STREAM-style copy, scale, add and triad report transferred bytes/time; shuffled dependent pointer chasing reports mean access latency per worker. Data sizes/cache/NUMA placement and shared backend contention affect values. Pointer loads remain observable under optimizing compilers, and cycle/integrity checks are independent of performance output.

Disk: fio parses internal JSON+; random read/write 4K, mixed 70/30, sequential read/write 1MiB, latency and depth scaling preserve existing definitions. Per-job owned data is actually filled, fsynced and verified before reads. Direct asynchronous I/O is required for automatic depth discovery; compiled engine listing alone is insufficient. io_uring setup permissions are probed without submitting I/O. Automatic modes use available libaio if io_uring is denied, expose that restriction, and check engine/achieved depth in real results. A filesystem/kernel that refuses direct asynchronous I/O fails rather than silently becoming buffered or synchronous. ext4 evidence does not qualify XFS/Btrfs or all mounts/quotas.

Network: iperf3 TCP receiver throughput, controlled streams and optional aggregate pacing. Advanced UDP/reverse/bidirectional/window controls retain existing behavior and require options exposed by the selected engine. A trusted remote server and compatible features are required to measure a path. Loopback functional checks cannot establish VM network performance; no daemon is started by initialization.

CPU/RAM/disk/network modules execute sequentially. Warm-ups, repeated measurements, CV and confirmation ranges are workload-specific. MAX reports the best validated observation within tested worker/depth/job/stream/data/time ceilings; theoretical maxima cannot be inferred. Scheduling, hypervisor overcommit, steal, throttling, cache state and storage contention remain practical limits.
