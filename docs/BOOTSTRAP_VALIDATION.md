# Bootstrap validation — 2026-10-13

Implemented in the existing repository; no benchmark algorithm, workload, disk safety or statistics code was changed.

Executed on this Windows host:

- Go 1.23.12, with GOTOOLCHAIN=local: `go test ./...` passed; `go vet ./...` passed.
- Linux amd64 CLI cross-build and Linux-target `go vet ./...` passed.
- Git Bash syntax checks passed individually for init.sh, start.sh, scripts/bootstrap-common.sh and scripts/build.sh.
- 27 controlled bootstrap workflow tests passed in the final parallel run (`python .tools/run-bootstrap-parallel.py`, invoking individual cases in tests/test_bootstrap.py).
- An additional focused `GOTOOLCHAIN=path` prohibition test passed after adding support for that policy, bringing executed passing cases to 28.
- Earlier concurrent tests hit a 30-second test harness deadline; raising the harness deadline to 90 seconds resolved those host-load timeouts. Installer engine probes remain limited to 10 seconds.
- Development archive regenerated and its contents/checksum verified.
- ShellCheck was not available and was not executed.

Workflow coverage includes first initialization; second launch without rebuilding; Go 1.22.2 rejection and project-local Go reuse; compatible newer Go; verified local toolchain archive installation; missing GCC and each external engine; package-install confirmation and --yes; offline prebuilt validation without compilers; offline download prohibitions; failed/interrupted initialization; selective source rebuilds; malformed/modified state and checksum checks; missing CLI; missing shared libraries; incompatible engine identity; CLI argument and exit-code forwarding; another working directory; paths containing spaces; rerun/reinit behavior; stale locks and unsupported architecture.

The workflow tests use platform/compiler/engine fixtures, including locally provided synthetic executable headers for the fixture scripts. They test actual Bash control flow, files, hashes, state publication, argument handling and failure paths. They do not compile C, run actual apt, download an actual Go archive, execute benchmarks, or qualify Linux runtime compatibility.

Not executed here: actual Ubuntu 22.04/24.04 initialization, actual apt installation, GCC builds through init.sh, Linux runtime engine probes, source-built third-party engine compilation, and complete standalone offline-release qualification. A real supported Ubuntu environment is still needed for these checks. The original development snapshot still lacks the three third-party executable engines; the online installer prepares installed system engines for development, and bundled mode reuses the existing pinned source build scripts.

To reproduce workflow tests: `python tests/test_bootstrap.py`. On Windows this uses Git Bash by default; override VMBENCH_TEST_BASH to select a Bash executable. On Linux it uses bash from PATH.
