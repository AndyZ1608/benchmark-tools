# Diagnostics

Run `bash init.sh --check` for read-only source-install diagnostics, or `./vmbench doctor` for standalone bundle diagnostics. Run `bash init.sh --verbose` for build logs. The initializer retains `.vmbench/last-build.log`; errors do not commit a successful new state.

- Unknown OS/ID_LIKE: inspect `/etc/os-release`. Family resemblance does not authorize untested package management. Supply a qualified matching bundle or add/test an explicit adapter.
- Missing package manager/package: use approved enabled repositories or verified local engines. RHEL/SLES subscriptions and optional repository enablement are administrator decisions. No yum guessing or signature bypass occurs.
- Old/explicit Go mismatch: inspect `GOTOOLCHAIN`, go.mod and `go version` with GOTOOLCHAIN=local. Supply compatible Go; offline cannot download. Do not overwrite system Go or change shell profiles to work around a project-local problem.
- Native compilation: confirm a C compiler, libc headers, pthread/barrier support and libm. Static builds also need static dependency archives. See the exact failed command output in the diagnostic log.
- Missing engine/wrong identity: ensure executable fio/stress-ng/iperf3 in the adjacent engines directory, or initialize from verified vendor/system binaries. Fiona's `fio` command is not the fio benchmark.
- Existing file reports missing: inspect its ELF interpreter with readelf; the named loader can be absent despite the binary existing. Wrong ELF machine and library loader errors are distinct. doctor invokes the binary to detect actual unresolved/transitive libraries and unsupported CPU instructions. Source init also inspects ldd output.
- Permission denied: check executable bits, noexec mounts, directory permissions and container seccomp. Running a benchmark as root is not the default remedy.
- io_uring denied/unsupported: the kernel or policy may block ring setup. Automatic modes require compiled libaio and report the limitation; advanced commands explicitly choose `--ioengine libaio`. Synchronous fallback is never used for requested depth semantics.
- Disk missing/quota/space: select an existing writable directory with reserve headroom. Preparation writes real data even for read tests. Quota or allocation failures stop and clean verified owned files. Do not use production-critical storage during busy periods.
- XFS/Btrfs/direct-I/O errors: these filesystems have not been qualified here. Preserve the exact fio error; do not disable direct I/O to manufacture a comparable result.
- Peer unreachable/option missing: check literal trusted IP, port, firewall and peer iperf3 features. No peer means Network is skipped; help/version checks do not test connectivity.
- Stale source binary: use `bash start.sh cpu quick` after git pull. The executable itself has no self-rebuild mechanism.
- Stale initialization lock: confirm no initializer is running, then remove only `.vmbench/init.lock`. Never remove benchmark data by wildcard/prefix.
- Abrupt termination: preserve leftover ownership/recovery metadata for manual inspection. Cleanup never recursively removes arbitrary historical workspaces or unrelated files.
