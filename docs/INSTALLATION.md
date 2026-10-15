# Source installation

Use Bash on a recognized Linux distribution with standard coreutils/findutils, tar, timeout and the libc inspection utility ldd. Alpine minimal images may need Bash/coreutils/findutils before running scripts; the precompiled CLI has no such build prerequisites. Run `bash start.sh` to initialize and open the menu; subsequent calls can use `bash start.sh cpu quick`.

## Detection and dependency selection

`scripts/install/platform.sh` reads ID, ID_LIKE, VERSION_ID and PRETTY_NAME as data, identifies uname architecture/kernel, detects glibc through getconf or musl through ldd, and reports root/sudo/doas and obvious container markers. Unknown fields, missing metadata and unsupported architectures have explicit errors. ID_LIKE is diagnostic, not automatic authorization for derivatives. Versions are recorded; recognition does not certify every version or lifecycle. See [qualification](COMPATIBILITY.md).

`scripts/install/packages.sh` translates logical compiler/pkg-config/libaio requirements into family package names. apt-get/dnf/zypper/pacman/apk installation is restricted to missing requirements after interactive approval or `--yes`. pacman uses existing metadata without `-Sy`, which would risk partial upgrades. Administrators must handle stale Arch systems outside VMBench. Adapter syntax follows the [Fedora package-management guide](https://fedoraproject.org/wiki/User_Guide_-_Managing_Software), [zypper manual](https://manpages.opensuse.org/Tumbleweed/zypper/zypper.8.en.html), and [Alpine apk handbook](https://docs.alpinelinux.org/user-handbook/0.1a/Working/apk.html). No yum fallback, EPEL, third-party sources, subscription activation, signature bypass, OS upgrade or reboot occurs. Unavailable packages remain errors; use compatible verified bundles or ask an administrator about approved repositories.

Native source builds accept `CC` naming one compiler executable, or detect cc/gcc/clang. Static source engine builds may need additional development/static libraries in enabled repositories. Failing compilation is reported in `.vmbench/last-build.log`, not hidden as successful setup.

## Go

The initializer reads go.mod; current minimum is 1.23.0 and preferred pinned toolchain go1.23.12. Its compatibility floor is 1.23.12. Compatible local/newer Go is reused subject to explicit GOTOOLCHAIN preference. An incompatible explicit version fails. `path`/`+path` never downloads. Normal online fallback installs checksum-pinned Go under `.vmbench/toolchains`, without changing system Go or global settings. Both amd64 and arm64 checksums are recorded in `scripts/go-toolchain.lock`, retrieved from the [official Go download index](https://go.dev/dl/?mode=json&include=all). Go builds/tests use a project-local .vmbench/cache/go-build cache and initialization uses the project staging directory for temporary files, avoiding an assumption that a RAM-backed /tmp has the disk fixtures' reserve space. All Go commands use GOTOOLCHAIN=local; offline builds also set GOPROXY=off and GOSUMDB=off. [Go telemetry](https://github.com/golang/go/blob/go1.24.0/src/cmd/internal/telemetry/telemetry.go) is disabled per invocation using the shipped scripts/go-telemetry/mode and TEST_TELEMETRY_DIR override, so an opted-in global telemetry setting cannot upload during an offline build; user settings are unchanged.

A supplied incompatible/corrupt local Go directory is not deleted automatically. Explicitly inspect and move it aside before retrying. Release builds use the initializer's selector in offline mode and require compatible Go already present on the build machine.

## Modes and startup state

- `--offline`: no downloads, package-manager calls or automatic Go switching.
- `--check`: read-only detection and existing-binary/tool availability checks; no staging directories or state changes. Missing/invalid engines cause nonzero exit. A compiler is not needed for a complete prebuilt installation.
- `--rebuild`: rebuild CLI/native; reuse usable external dependencies.
- `--yes`: package installation permission only, never disk-write/MAX permission.
- `--verbose`: expose build/test output; failures retain a diagnostic log.
- `--engine-mode system`: prefer a validated vendor bundle, existing installed copy, then system PATH.
- `--engine-mode bundled`: require static binaries, or build cached pinned engines (source acquisition only online).

A source checkout with binaries but no CLI/native source receipts rebuilds those components once; a successful --version alone cannot establish that an old executable matches current sources. Binary-only releases contain no buildable cmd/go.mod and do not require compilers. Source fingerprints cover nested Go/native files, Go module files, VERSION, configuration and build/install scripts. CLI/native fingerprints select incremental rebuilds; setup configuration changes revalidate the installation. Changing a recipe used for both kernels rebuilds both. Copied binaries and metadata have integrity hashes. Schema 2 migrates old states by revalidating usable components; it deletes no user data. Platform identity/libc changes invalidate the saved host state. State is data, never sourced shell.

Files are staged and checked before replacement. The final state/hash marker is written only after validation and unchanged-source confirmation. An interrupted multi-file commit fails subsequent checks; there is no false successful state or whole-directory transactional guarantee. A stale lock is not removed automatically: first establish the initializer is no longer active, then remove only `.vmbench/init.lock`.

`start.sh` consumes only a leading prefix of `--offline`, `--yes`, `--reinit`, `--engine-mode`, `--verbose`, or `--`, then forwards CLI arguments exactly and preserves the caller's working directory. Direct `vmbench` does not rebuild itself. Initialization invokes safe help/version/doctor checks and Go tests when building; no CPU/RAM/disk stress workload or network server starts.

## Engine/runtime diagnostics

Every selected binary must have executable permissions, the matching ELF machine, working loader/libraries and recognized version output. fio engine queries must succeed; compiled async candidates and setup permissions are reported independently. A missing/unpermitted async candidate blocks only Disk, whose real direct-I/O and depth checks wait for an explicit target. Safe iperf3 help queries check required options. Initialization records versions, origin and linkage; this is host-specific evidence, not portable release qualification. Run `./vmbench doctor` for diagnostics without ldd/binutils/Go. Actual runtime invocation catches failures that dependency lists alone cannot establish.
