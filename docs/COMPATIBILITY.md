# Compatibility evidence

The installer recognizes explicit Debian/Ubuntu, RHEL-family, SUSE-family, Arch and Alpine IDs. Recognition and package adapters are implementation capabilities, not distribution certification. Unknown derivatives are rejected even when ID_LIKE resembles a known family. ARM64 is experimental; cross compilation is not runtime qualification.

The [machine-readable summary](compatibility.json) records actual validation levels. Runtime diagnostics are host-specific. The CI matrix covers Ubuntu 22.04/24.04, Debian 12/13, Rocky/AlmaLinux 9/10, CentOS Stream 9/10, image-selected Fedora/openSUSE, Arch and Alpine 3.22 containers. These jobs have been implemented but not executed from this local session. Fedora/SUSE/rolling image versions must be recorded from os-release at execution time. No assertion about lifecycle support is inferred from an image tag. RHEL/SLES need authorized subscription/repository environments and are not included in public CI.

## Validation levels

1. Build verified: toolchain accepted and binaries compiled for the named architecture/libc.
2. Unit tests passed: tests executed, not just cross compiled.
3. Installation verified: real source init, runtime checks, successful state commit and repeated reuse on that OS.
4. Engine runtime verified: safe versions/help plus explicitly requested bounded engine workloads where recorded.
5. CPU/RAM/Disk verified: bounded real workloads on disposable storage, with isolated files/sentinels/cleanup.
6. Offline deployment verified: complete bundle works without network and without development tools.
7. Redistribution approved: separate transitive license/source review; runtime checks cannot grant it.

Container smoke checks never establish full host/VM behavior. QEMU TCG guest workloads establish functional behavior, not representative performance. ext4 evidence does not qualify XFS or Btrfs. Loopback iperf checks do not measure a remote VM network path.

## Local session

Development runs on Windows using Go 1.23.12 and Git Bash. Actual Linux checks use an isolated Alpine 3.22.6 amd64 QEMU guest, musl 1.2.5, Linux 6.12.110, with a private ext4 virtual disk; no host disk passthrough. Earlier automatic-mode work exercised CPU/RAM/fio, bounded Quick/Standard/MAX, controlled writes, cancellation and owned-file preservation there. This compatibility task verified source initialization with checksum-pinned project-local Go, a real missing-iperf3 apk installation, unchanged binary reuse, read-only state hashes, all version aliases/help, 49 bootstrap tests, 13 platform tests, the optimized native correctness test, all 14 Go test-bearing packages, vet, shell syntax and ShellCheck. See the [dated phase record](validation/alpine-3.22.6-amd64-2026-10-14.json).

A separately started diskless Alpine guest used the baseline qemu64 CPU model. Its root had no Go/GCC/Clang/git/Python/pip/Docker packages, and eth0 stayed DOWN with no external route. The complete static bundle passed doctor/version/help, a one-second verified stress-ng worker and one-second iperf client/server loopback smoke. QUICK, STANDARD and MAX each completed CPU, RAM and isolated ext4 disk workloads with one worker, a 64MiB RAM ceiling, 16MiB disk data, QD2, one job, one-second exploration samples and a ten-minute mode ceiling. Cleanup removed owned files; the unrelated sentinel remained unchanged. No persistent benchmark result directory appeared. The initial 16MiB RAM ceiling correctly skipped RAM; final runs used 64MiB, and the CI runner now rejects skipped modules as a workload pass.

The [recorded dependency review](RELEASE-AUDIT.md) retains exact source revisions, notices, build configuration and static link maps, and is bound to the five executable hashes. This review applies only to that build. Final archive documentation was refreshed afterward while preserving all tested executable hashes.

All other named distributions remain unverified locally. Native ARM64 CPU/RAM execution, XFS/Btrfs, real remote-network throughput and glibc/other-build redistribution reviews remain outstanding. No production-ready universal Linux archive is claimed.

Use `python3 scripts/compatibility-test.py --workloads --output compatibility-result.json` in a disposable Linux environment. Add `--install` only when package installation is authorized. It records OS, architecture, environment and individual phase outputs even when a phase fails. The CI workflow also records setup failures when repositories/images are unavailable.


## RHEL 9.6 user report and async validation repair - 2026-10-15

Reported host: Red Hat Enterprise Linux 9.6, x86_64, kernel `5.14.0-570.12.1.el9_6.x86_64`, glibc 2.34/dnf; Go 1.23.12, fio 3.35, stress-ng 0.19.03, iperf3 3.9. The user reports successful compilation/preparation, followed by initialization failure: io_uring setup returned EPERM while Linux AIO setup/destruction succeeded without submitting I/O. These are separate observations; **functional RHEL Disk benchmarking remains unverified**.

The old doctor gate required both an exact compiled fio engine name and a successful matching setup syscall. Given AIO setup success, the reported error means the running fio query did not supply the exact accepted `libaio` token to that gate. The report does not include `fio --enghelp`, so a missing compiled engine versus different output cannot be determined remotely. Successful kernel `io_setup` cannot establish fio's build capabilities, completed direct I/O, or achieved queue depth. Treating this Disk capability gate as a fatal whole-application installation condition was the interface defect.

The repair preserves executable/ELF/library integrity failures as fatal initialization errors. Async candidates have independent status, and filesystem validation is deferred until an explicit Disk target is selected. Automatic selection functionally tests io_uring first, then libaio; explicit requests never substitute. A private populated/synchronized file, bounded direct reads, completed-I/O counters and achieved depth establish target-specific capability. A setup-only success never means READY. No synchronous replacement is used; failed or underachieved Disk results are rejected without disabling independent CPU/RAM/Network operations.

Policy diagnostics follow the [Linux kernel sysctl definitions](https://www.kernel.org/doc/html/latest/admin-guide/sysctl/kernel.html): `io_uring_disabled=0` permits setup through that sysctl only; `1` restricts creation to CAP_SYS_ADMIN or the configured `io_uring_group`; `2` disables creation for everyone. Missing sysctls and seccomp/SELinux/kernel restrictions remain possible; the actual setup result is recorded. VMBench changes none of these settings. [fio documents direct I/O and achieved-depth requirements](https://fio.readthedocs.io/en/latest/fio_doc.html#cmdoption-arg-iodepth); compiled engines, setup permission, completed I/O and achieved queueing are distinct checks.

This repair is tested with controlled kernel/engine fixtures and the separately documented disposable Linux test environment. It is not a claim of fresh real RHEL 9.6 installation, workload, XFS/Btrfs, or release qualification. Other distributions' existing qualification levels remain unchanged.


The completed-I/O counters and bounded `number_ios` option were also checked against the upstream fio 3.35 [JSON statistics implementation](https://raw.githubusercontent.com/axboe/fio/fio-3.35/stat.c) and [option definitions](https://raw.githubusercontent.com/axboe/fio/fio-3.35/options.c). This source-interface check does not substitute for execution of the RHEL-packaged binary.
