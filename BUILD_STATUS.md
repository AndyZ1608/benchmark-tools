# VMBench development validation — 2026-10-13

**Status: implemented development snapshot; v1.0 acceptance criteria are NOT met.**

Executed on Windows:
- `go test ./...`: passed all existing Go unit tests (CPU-result validation, statistics, configuration, fio fixture parsing, network parsing, report serialization, missing engine, child timeout and context cancellation).
- `go vet ./...`: passed for Windows and Linux target builds.
- Windows CLI build and `version` / `--help`: passed.
- Invalid duration CLI smoke: correctly rejected with nonzero exit status.
- `GOOS=linux GOARCH=amd64 go build -trimpath ...`: passed.
- Linux disk safety tests compiled with `go test -c`; NOT executed.
- Native kernel cross-compilation with Zig 0.13.0, x86_64-linux-musl, -O2 -fno-fast-math -pthread -static: passed.
- ELF64 x86_64 inspection of both Linux binaries: no PT_INTERP and no DT_NEEDED entries.
- Cached upstream source archives: SHA-256 checks passed against scripts/engines.lock.json.
- Development archive: contents reopened and verified, checksum produced.

Not executed:
- Native CPU/RAM workload correctness or real throughput/latency runs on Linux.
- Real fio, stress-ng, iperf3 or full-suite smoke tests.
- Linux disk safety and Linux process-group cancellation tests.
- Third-party engine compilation, full static dependency/license verification.
- Ubuntu 22.04/24.04 offline package validation.

The development archive includes source, cached pinned upstream source archives, the Linux Go executable and static native kernel. It deliberately lacks fio, stress-ng and iperf3 executables. It cannot execute disk/network/CPU stress until those are built and installed beside the CLI. Packaging scripts reject a complete release when required engine binaries are missing. No complete vmbench-linux-amd64.tar.gz release has been produced.

See README.md for unfinished advanced features and docs/BUILD.md for Linux qualification commands. No fabricated benchmark results or test success are included.
