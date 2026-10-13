# Initialization and startup

`start.sh` verifies the installation manifests and engine runtime/version checks. Missing, changed or invalid state triggers `init.sh`. Startup uses absolute script paths and replaces itself with the root `vmbench` executable. CLI arguments after the startup prefix remain untouched.

`init.sh` supports `--offline`, `--yes`, `--rebuild`, and `--engine-mode system|bundled`. Online is the default. Explicit apt approval applies only to the displayed missing packages; no system upgrade, repository changes, tuning, firewall changes, Docker installation or benchmark execution occurs. The iperf3 daemon option is preseeded off and automatic needrestart restarts are suppressed. Compilation and CLI execution use the invoking user, without sudo. Administrative privileges are used only for approved apt/debconf operations.

The initializer locks `.vmbench/init.lock`, builds/copies into a private staging directory, validates ELF architecture, runtime libraries, engine identity and fio's io_uring capability, runs help/version checks and the Go unit suite when rebuilding the CLI, then publishes executables and installation metadata. Changing only native source rebuilds only compute-bench. An interruption never commits a successful new state; a previous installation can be reused only when its manifests still match. A first-time prebuilt installation validates executables without requiring compilers; its source-to-binary provenance is not established by the installer. Hashes detect accidental changes, not malicious modification of both content and manifests.

## Go version diagnosis

The previous `go.mod` used the language-family spelling `go 1.23` with no toolchain directive. The observed Go 1.22.2 error names `go1.23`, which is a family version rather than the full downloadable release name expected by that toolchain-selection path. The exact original global GOTOOLCHAIN setting was not supplied, so it cannot be reconstructed here.

The module now uses `go 1.23.0` and `toolchain go1.23.12`. Bootstrap requires Go 1.23.12 or a compatible newer release and inspects both the module's minimum version and the environment/persistent GOTOOLCHAIN preference. It invokes raw local tools with `GOTOOLCHAIN=local`, avoiding implicit downloads and never modifying global Go configuration. Exact compatible toolchain preferences are honored only when that exact local installation exists; `+auto` and `+path` allow compatible newer local versions. `path` and `+path` forbid downloads. An incompatible preference is diagnosed explicitly rather than passed blindly to Go.

If neither local project Go nor system Go is suitable, online initialization downloads the official Linux amd64 Go 1.23.12 archive, checks its SHA-256 against `scripts/go-toolchain.lock`, extracts it locally and checks its version. A newer module minimum beyond that pin requires a suitable local toolchain. Offline mode never downloads and uses `GOPROXY=off GOSUMDB=off` for builds/tests. Source modules currently depend only on the standard library.

Useful commands:

```bash
./init.sh --offline
./init.sh --yes
./start.sh --reinit --help
./start.sh --offline cpu test --preset quick
GOTOOLCHAIN=local .vmbench/toolchains/go1.23.12/bin/go test ./...
```

## Troubleshooting

- Go 1.22.2: run online initialization to install project-local 1.23.12, or put a compatible Go on PATH before offline setup.
- Requested compatible Go unavailable: put the requested version on PATH, or use `GOTOOLCHAIN=local` to select available tools and permit the pinned project-local installer.
- Missing GCC: approve the displayed build-essential package online or supply local tools. A complete prebuilt release does not need GCC.
- Missing engine: install the named package online or supply a compatible local/bundled binary. A similarly named program, such as Python Fiona's `fio`, fails the identity check.
- Missing `libXYZ.so`: restore that runtime library on this host or supply genuinely self-contained engines. Offline initialization never invokes apt.
- Bundled-mode dynamic binary: use system mode for local development, or rebuild the complete static engine set. No dynamic binary is claimed portable merely because it was copied.
- Stale lock: verify no initialization process is active, then remove only the empty `.vmbench/init.lock` directory and rerun.
- Existing invalid local Go directory: move `.vmbench/toolchains/go1.23.12` aside explicitly; bootstrap will not overwrite that toolchain directory automatically.
- Source fingerprint changed during initialization: finish edits and rerun.

`VMBENCH_OS_RELEASE` optionally points to a local os-release file for controlled bootstrap testing; normal installations use `/etc/os-release`.

## Tests and qualification

`python tests/test_bootstrap.py` runs the real Bash state/build/launch workflow against controlled platform/compiler/engine fixtures. It covers source installation, reuse, Go selection, missing dependencies, offline network prohibitions, source changes, state corruption, arguments, paths with spaces, failures and termination. These fixtures do not measure performance and do not establish Ubuntu runtime compatibility.

Run `bash -n init.sh`, `bash -n start.sh` and `bash -n scripts/bootstrap-common.sh`, plus ShellCheck when installed, and `GOTOOLCHAIN=local go test ./...`. Actual Ubuntu 22.04/24.04 apt, native-engine and offline-release qualification still require those environments. Bootstrap version/capability checks intentionally do not run any benchmark.
