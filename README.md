# VMBench
An offline-first Linux CLI implementation under development. **This is not a qualified v1.0 offline release.**

## Use
Build the CLI: `go build ./cmd/vmbench`. Linux native kernel: `cc -O2 -fno-fast-math -pthread native/compute-bench.c -o engines/compute-bench` beside the CLI. Supply fio, stress-ng and iperf3 under the same engines directory; runtime never downloads anything.

```
./vmbench --help
./vmbench system info
./vmbench cpu test --preset quick
./vmbench cpu stress --workers 8 --duration 300
./vmbench ram test --threads 8 --size 4GiB
./vmbench disk test --path /data --profile randread-4k --iodepth 32 --numjobs 4
./vmbench network server --bind 127.0.0.1
./vmbench network test --target 127.0.0.1
./vmbench full test --preset precision --path /data --non-interactive
```

No arguments opens a command prompt. Bare component commands prompt for worker count, duration, warmup and repetitions. Enter accepts defaults. Flags enable noninteractive use. `--config config/default.yaml` accepts a deliberately restricted flat YAML scalar mapping; nested YAML is rejected. Precedence: CLI > file > preset > defaults. Seconds are integers; sizes support bytes, KiB, MiB, GiB.

Quick: 1s warmup, 2s per workload, one repetition. Standard: 2s warmup, 10s per workload, three repetitions. Precision: 5s warmup, 30s per workload, five repetitions. CPU and RAM run separately with one and requested worker counts. RAM total dataset is divided into three arrays and partitioned between workers. Each CPU unit means one defined recurrence iteration, not a universal operation or score. Floating-point contraction/compiler/platform changes may affect comparability.

RAM Copy/Scale/Add/Triad are independently implemented STREAM-style operations, not official STREAM results. Working sets are configurable; verify they exceed aggregate LLC before interpreting as memory bandwidth. Pointer chasing uses a deterministic shuffled single cycle per worker; reports dependent access time including loop overhead. Native CPU correctness and timing need further qualification. Memory integrity writes and reads deterministic values; completion does not prove hardware reliability.

Disk uses exclusive file creation and anchored directory descriptors, never raw devices. Each job has its own file, 1GiB minimum free-space reserve. **Even read profiles initialize their new files by writing them.** Write profiles require `--confirm-writes`. Files are isolated in a private per-run directory and removed on ordinary completion/cancellation. After abrupt process death, inspect the private `vmbench-<run-id>/` directory and its `vmbench-<run-id>-*.dat` files in the selected directory and remove only the files belonging to that run. Never benchmark valuable workloads without accounting for I/O contention. Direct I/O bypasses guest page cache in supported configurations, not hypervisor or backend caches. Engine failures are reported without fallback. fio JSON+ bytes/s and nanoseconds are preserved; read and write percentiles are separate and never averaged. Queue depth scaling uses 1,4,16,32,64.

Network defaults to loopback server bind; explicitly select an interface for peers. Use trusted networks. iperf3 throughput is not forwarding PPS. Its raw end/interval data preserves UDP jitter/loss and TCP metrics. Missing peer skips network. No independent RTT probe is implemented.

Reports under `results/` contain workload, metadata, raw results, warnings, failures and elapsed time. No ANSI escape sequences are emitted, so NO_COLOR and redirected terminals work. No sysctl/governor/cache tuning occurs.

## Validation and limitations
Run `go test ./...`. Release: `bash scripts/build.sh`; see docs/BUILD.md for incomplete third-party build prerequisites. The Go CLI and native kernel were cross-compiled for Linux; Go unit tests and Windows help/version were executed on this Windows host. Linux native kernels, real fio/stress-ng/iperf3 runs, cancellation of engine trees and Ubuntu offline compatibility remain unexecuted. Missing engines fail explicitly.

Still incomplete: detailed live telemetry sampling, cgroup v1 support, ancestor memory-usage accounting, thermal readings and detailed NUMA policy, NUMA policy, block-device metadata, progress bars, terminal metric tables, native trusted reference fixtures, user-adjustable disk block sizes/headroom/data patterns, steady-state and time-series checks, network summary statistics/RTT, qualified Linux third-party engine builds and complete transitive license audit. Do not use current output as acceptance evidence for infrastructure.
