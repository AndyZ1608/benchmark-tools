#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
source "$ROOT/scripts/bootstrap-common.sh"
OFFLINE=0 YES=0 FORCE=0 CHECK_ONLY=0 VERBOSE=0 ENGINE_MODE=system
GO_VERSION=go1.23.12 GO_REQUIRED=1.23.0 GO_BIN='' GO_SELECTED=not-required
STATE_DIR="$ROOT/.vmbench"
STAGE='' LOCK_HELD=0
INITIALIZATION_PHASE='setup'
PACKAGES=()
VERSION_NAMES=() VERSION_VALUES=() LINKAGES=() ORIGINS=()
usage() {
    cat <<'EOF'
Usage: ./init.sh [--offline] [--check] [--yes] [--rebuild] [--verbose] [--engine-mode system|bundled]
--check performs read-only platform, dependency and existing binary checks (no downloads).
--yes authorizes only listed missing packages using the detected distribution adapter.
Offline mode performs no downloads, package-manager calls, or Go automatic switching.
System mode prefers verified bundles, then existing local/system engines.
Bundled mode requires static engines built from checksum-pinned sources.
EOF
}
cleanup() {
    local status=$?
    trap - EXIT
    if [[ -n $STAGE && $STAGE == "$STATE_DIR"/staging.* && -d $STAGE ]]; then rm -rf -- "$STAGE"; fi
    if (( LOCK_HELD )); then rmdir -- "$STATE_DIR/init.lock" || true; fi
    if (( status != 0 )); then
        if [[ -f $STATE_DIR/last-build.log ]]; then tail -n 50 "$STATE_DIR/last-build.log" >&2; log INFO "Diagnostic log: $STATE_DIR/last-build.log" >&2; fi
        log ERROR "Initialization failed during $INITIALIZATION_PHASE (exit $status); no successful new installation state was committed." >&2; fi
    exit "$status"
}
check_dependencies() {
    local utility
    for utility in sha256sum od timeout ldd find sort xargs sed grep cut tr mktemp awk head tar date; do
        command -v "$utility" >/dev/null || { fail "Required base utility missing: $utility. Install coreutils/findutils/libc-bin."; return 1; }
    done
    PACKAGES=()
    if (( BUILD_CLI )) && ! command -v curl >/dev/null && ! command -v wget >/dev/null; then
        local candidate version compatible=0
        for candidate in "$STATE_DIR/toolchains/$GO_VERSION/bin/go" "$(command -v go || true)"; do
            [[ -x $candidate ]] || continue
            version=$(cd /; go_inspect "$candidate" version | awk '{print $3}')
            if go_compatible "$version"; then compatible=1; fi
        done
        (( compatible )) || add_package curl
    fi
    if (( BUILD_NATIVE )); then find_compiler >/dev/null || add_package compiler; fi
    if [[ $ENGINE_MODE == bundled ]] && (( NEED_ENGINES )); then
        find_compiler >/dev/null || add_package compiler
        for utility in make pkg-config python3 readelf; do
            command -v "$utility" >/dev/null || case "$utility" in readelf) add_package binutils;; *) add_package "$utility";; esac
        done
        # Static source builds need development headers in addition to basic compilation.
        if ! pkg-config --exists libaio 2>/dev/null && [[ ! -f /usr/include/libaio.h ]]; then add_package libaio; fi
    elif [[ $ENGINE_MODE == system ]]; then
        local engine
        for engine in fio stress-ng iperf3; do
            if ! resolve_engine "$engine" >/dev/null 2>&1 && ! command -v "$engine" >/dev/null; then add_package "$engine"; fi
        done
    fi
    if ((${#PACKAGES[@]})); then mapfile -t PACKAGES < <(printf '%s\n' "${PACKAGES[@]}" | sort -u); fi
}
go_compatible() {
    local version=${1#go} major minor patch
    [[ $version =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]] || return 1
    major=${BASH_REMATCH[1]} minor=${BASH_REMATCH[2]} patch=${BASH_REMATCH[3]}
    (( major > 1 || (major == 1 && (minor > 23 || (minor == 23 && patch >= 12))) )) || return 1
    [[ $(printf '%s\n' "$GO_REQUIRED" "$version" | sort -V | head -n1) == "$GO_REQUIRED" ]]
}
ensure_go_toolchain() {
    (( BUILD_CLI )) || { [[ ! -f $STATE_DIR/go-toolchain ]] || GO_SELECTED=$(cat "$STATE_DIR/go-toolchain"); log INFO 'Prebuilt CLI: Go is not required.'; return 0; }
    GO_REQUIRED=$(awk '$1=="go"{gsub(/\r/,"",$2);print $2}' "$ROOT/go.mod")
    local preferred; preferred=$(awk '$1=="toolchain"{gsub(/\r/,"",$2);print $2}' "$ROOT/go.mod"); GO_VERSION=${preferred:-go1.23.12}
    [[ $GO_REQUIRED =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]] || { fail 'Cannot determine required Go version from go.mod'; return 1; }
    log CHECK "Go toolchain (go.mod requires $GO_REQUIRED; preferred $GO_VERSION; automatic switching disabled)"
    local candidate version requested=${GOTOOLCHAIN:-} configured='' goenv preference no_download=0 allow_newer=0
    # Inspect persistent Go settings without allowing Go to perform a download.
    if command -v go >/dev/null; then
        goenv=$(cd /; go_inspect go env GOENV)
        if [[ -z $requested && -f $goenv ]]; then configured=$(sed -n 's/^GOTOOLCHAIN=//p' "$goenv"); requested=$configured; fi
        log INFO "Active GOTOOLCHAIN preference: ${requested:-auto (default)}"
    fi
    preference=$requested
    case $preference in ''|local|auto|path|go[0-9]*) ;; *) fail "Invalid GOTOOLCHAIN preference: $preference"; return 1;; esac
    [[ $preference != path && $preference != *+path ]] || no_download=1
    [[ $preference != *+auto && $preference != *+path ]] || allow_newer=1
    requested=${requested%+auto}; requested=${requested%+path}
    if [[ $requested == go* ]] && ! go_compatible "$requested"; then
        fail "Explicit GOTOOLCHAIN=$requested is incompatible with go.mod; select a compatible local toolchain."; return 1
    fi
    for candidate in "$(command -v "$requested" || true)" "$STATE_DIR/toolchains/$GO_VERSION/bin/go" "$(command -v go || true)"; do
        [[ -n $candidate && -x $candidate ]] || continue
        version=$(cd /; go_inspect "$candidate" version | awk '{print $3}')
        if go_compatible "$version" && { [[ $requested != go* ]] || [[ $requested == "$version" ]] || { (( allow_newer )) && [[ $(printf '%s\n' "${requested#go}" "${version#go}" | sort -V | head -n1) == "${requested#go}" ]]; }; }; then GO_BIN=$candidate; GO_SELECTED=$version; log OK "$version detected"; return 0; fi
        log INFO "Toolchain $version does not satisfy required/selected version."
    done
    if [[ $requested == go* && $requested != "$GO_VERSION" ]]; then fail "Selected $requested is unavailable locally. Put that toolchain on PATH, or set GOTOOLCHAIN=local to allow project-local $GO_VERSION."; return 1; fi
    (( ! no_download )) || { fail "GOTOOLCHAIN=$preference prohibits downloads and its toolchain is not available locally."; return 1; }
    (( ! OFFLINE )) || { fail 'Offline initialization needs local Go 1.23.12 or newer. Put it on PATH or provide .vmbench/toolchains/go1.23.12/bin/go.'; return 1; }
    go_compatible "$GO_VERSION" || { fail "go.mod requires $GO_REQUIRED, beyond the checksum-pinned $GO_VERSION. Supply a compatible toolchain on PATH."; return 1; }
    local filename="$GO_VERSION.linux-$GO_ARCH.tar.gz" archive expected
    archive="$STAGE/$filename"
    expected=$(awk -v file="$filename" '{sub(/\r$/,"",$2)} $2==file{print $1}' "$ROOT/scripts/go-toolchain.lock")
    [[ $expected =~ ^[0-9a-f]{64}$ ]] || { fail "No trusted checksum pin for $filename; supply a compatible local Go toolchain."; return 1; }
    if command -v curl >/dev/null; then curl --fail --location --retry 2 "https://go.dev/dl/$filename" -o "$archive"
    elif command -v wget >/dev/null; then wget -O "$archive" "https://go.dev/dl/$filename"
    else fail 'Go download needs curl or wget; install either or provide a compatible local Go.'; return 1; fi
    [[ $(hash_file "$archive") == "$expected" ]] || { fail 'Go archive checksum mismatch'; return 1; }
    mkdir -p "$STATE_DIR/toolchains"
    tar -xzf "$archive" -C "$STAGE"
    GO_BIN="$STAGE/go/bin/go"
    version=$(go_inspect "$GO_BIN" version | awk '{print $3}')
    [[ $version == "$GO_VERSION" ]] || { fail 'Downloaded Go version does not match the pin'; return 1; }
    [[ ! -e $STATE_DIR/toolchains/$GO_VERSION ]] || { fail 'Existing project-local Go is invalid; move it aside before retrying.'; return 1; }
    mv "$STAGE/go" "$STATE_DIR/toolchains/$GO_VERSION"
    GO_BIN="$STATE_DIR/toolchains/$GO_VERSION/bin/go"; GO_SELECTED=$version
    log OK "$version installed locally; system Go unchanged"
}
go_local() {
    verify_go_policy || return
    # Source builds currently use only the standard library. Offline module access is disabled explicitly.
    if (( OFFLINE )); then env GOOS=linux GOARCH="$GO_ARCH" GOAMD64=v1 GOARM64=v8.0 TEST_TELEMETRY_DIR="$ROOT/scripts/go-telemetry" GOCACHE="$STATE_DIR/cache/go-build" TMPDIR="${STAGE:-${TMPDIR:-/tmp}}" GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off "$GO_BIN" "$@"
    else env GOOS=linux GOARCH="$GO_ARCH" GOAMD64=v1 GOARM64=v8.0 TEST_TELEMETRY_DIR="$ROOT/scripts/go-telemetry" GOCACHE="$STATE_DIR/cache/go-build" TMPDIR="${STAGE:-${TMPDIR:-/tmp}}" GOTOOLCHAIN=local "$GO_BIN" "$@"; fi
}
build_vmbench() {
    if (( BUILD_CLI )); then log BUILD 'VMBench CLI'; local version_flags; version_flags=$(build_version_ldflags); (cd "$ROOT"; CGO_ENABLED=0 go_local build -trimpath -ldflags "$version_flags" -o "$STAGE/vmbench" ./cmd/vmbench) || { fail 'CLI compilation failed'; return 1; }; log OK 'CLI build completed'
    else cp "$ROOT/vmbench" "$STAGE/vmbench" || return; log OK 'CLI unchanged; reusing executable'; fi
}
build_native_engine() {
    if (( BUILD_NATIVE )); then log BUILD 'Native CPU/Memory engine'; local linkage=dynamic; [[ $ENGINE_MODE != bundled ]] || linkage=static; compile_native_kernel "$STAGE/engines/compute-bench" "$linkage" "$(find_compiler)" || { fail 'Native compilation failed'; return 1; }; log OK 'compute-bench compiled'
    else cp "$ROOT/engines/compute-bench" "$STAGE/engines/compute-bench" || return; log OK 'Native engine unchanged; reusing executable'; fi
}
prepare_benchmark_engines() {
    local engine source origin
    if [[ $ENGINE_MODE == bundled ]] && (( NEED_ENGINES )); then
        if (( ! OFFLINE )); then run_step 'Pinned engine source acquisition' python3 "$ROOT/scripts/fetch-engines.py"; fi
        run_step 'Static benchmark engine build' bash "$ROOT/scripts/build-engines.sh"
    fi
    for engine in fio stress-ng iperf3; do
        resolve_engine "$engine" || { fail "$engine missing or incompatible; install a compatible system engine or supply a verified vendor-engines bundle."; return 1; }
        source=$RESOLVED_ENGINE; origin=$RESOLVED_ORIGIN
        cp "$source" "$STAGE/engines/$engine"
        ORIGINS+=("$origin")
        log OK "$engine prepared ($origin)"
    done
}
validate_engine_dependencies() {
    local name linkage
    VERSION_NAMES=(); VERSION_VALUES=(); LINKAGES=()
    for name in vmbench compute-bench fio stress-ng iperf3; do
        local path="$STAGE/engines/$name"; [[ $name != vmbench ]] || path="$STAGE/vmbench"
        log CHECK "$name executable and runtime dependencies"
        validate_binary "$path" "$name"
        VERSION_NAMES+=("$name"); VERSION_VALUES+=("$ENGINE_VERSION"); LINKAGES+=("$ENGINE_LINKAGE")
        if [[ $ENGINE_MODE == bundled && $ENGINE_LINKAGE != static ]]; then fail "$name is dynamically linked; bundled mode requires static executables. Use --engine-mode system for this host only."; return 1; fi
        log OK "$name ready ($ENGINE_LINKAGE): ${ENGINE_VERSION%%$'\n'*}"
    done
}
run_smoke_tests() {
    # These checks never start benchmark workloads or an iperf server.
    log CHECK 'CLI help/version and engine version/capability checks'
    timeout 10 "$STAGE/vmbench" --help >/dev/null
    timeout 10 "$STAGE/vmbench" --version >/dev/null
    timeout 30 "$STAGE/vmbench" doctor
}
run_unit_tests() {
    log CHECK "Go unit tests using $GO_SELECTED"
    (cd "$ROOT" && go_local test ./...) || {
        fail 'Go unit tests failed; staged builds were not published. Fix the failing tests and rerun initialization.'
        return 1
    }
}
save_installation_state() {
    local i temporary="$STATE_DIR/install-state.json.tmp" full current_cli current_native
    full=$(fingerprint all); [[ $full == "$INITIAL_SOURCE" ]] || { fail 'Source changed during initialization; rerun after editing is complete.'; return 1; }
    current_cli=$(fingerprint cli); current_native=$(fingerprint native)
    mkdir -p "$ROOT/engines"
    for name in vmbench engines/compute-bench engines/fio engines/stress-ng engines/iperf3; do
        if [[ ! -f $ROOT/$name ]] || [[ $(hash_file "$ROOT/$name") != "$(hash_file "$STAGE/$name")" ]]; then mv -f -- "$STAGE/$name" "$ROOT/$name"; fi
    done
    platform_key > "$STATE_DIR/platform.sha256"
    printf '%s\n' "$full" > "$STATE_DIR/source.sha256"
    printf '%s\n' "$current_cli" > "$STATE_DIR/cli-source.sha256"
    printf '%s\n' "$current_native" > "$STATE_DIR/native-source.sha256"
    printf '%s\n' "$GO_SELECTED" > "$STATE_DIR/go-toolchain"
    (cd "$ROOT"; sha256sum vmbench engines/compute-bench engines/fio engines/stress-ng engines/iperf3 .vmbench/source.sha256 .vmbench/cli-source.sha256 .vmbench/native-source.sha256 .vmbench/go-toolchain .vmbench/platform.sha256 > .vmbench/install-files.sha256)
    {
        printf '{\n  "schema": %s,\n  "status": "validated",\n  "version": ' "$BOOTSTRAP_SCHEMA"; json_string "$(cat "$ROOT/VERSION" 2>/dev/null || echo unknown)"
        printf ',\n  "platform": '; json_string "$OS_PRETTY"; printf ',\n  "architecture": '; json_string "$ARCH"
        printf ',\n  "libc": '; json_string "$LIBC $LIBC_VERSION"
        printf ',\n  "kernel": '; json_string "$KERNEL"
        printf ',\n  "package_manager": '; json_string "$PACKAGE_MANAGER"
        printf ',\n  "go_toolchain": '; json_string "$GO_SELECTED"; printf ',\n  "engine_mode": '; json_string "$ENGINE_MODE"
        printf ',\n  "build_status": {"cli_built": %s, "native_built": %s}' "$BUILD_CLI" "$BUILD_NATIVE"
        printf ',\n  "initialized_at": '; json_string "$(date -u +%FT%TZ)"
        printf ',\n  "source_fingerprint": '; json_string "$full"
        printf ',\n  "portability": "runtime validated on this host; standalone release qualification not implied",\n  "engines": {\n'
        for i in "${!VERSION_NAMES[@]}"; do
            (( i == 0 )) || printf ',\n'
            printf '    '; json_string "${VERSION_NAMES[i]}"; printf ': {"version": '; json_string "${VERSION_VALUES[i]}"; printf ', "linkage": '; json_string "${LINKAGES[i]}"
            if (( i >= 2 )); then printf ', "origin": '; json_string "${ORIGINS[i-2]}"; fi
            printf '}' 
        done
        printf '\n  }\n}\n'
    } > "$temporary"
    mv -f -- "$temporary" "$STATE_DIR/install-state.json"
    (cd "$ROOT"; sha256sum .vmbench/install-state.json .vmbench/install-files.sha256 > .vmbench/install-state.sha256.tmp)
    mv -f -- "$STATE_DIR/install-state.sha256.tmp" "$STATE_DIR/install-state.sha256"
}
run_step() {
    local label=$1; shift
    INITIALIZATION_PHASE=$label
    log CHECK "$label"
    if (( VERBOSE )); then "$@" 2>&1 | tee -a "$STATE_DIR/last-build.log"
    else "$@" >> "$STATE_DIR/last-build.log" 2>&1; fi
    log OK "$label"
}
main() {
    while (($#)); do
        case $1 in --check) CHECK_ONLY=1;; --verbose) VERBOSE=1;; --offline) OFFLINE=1;; --yes) YES=1;; --rebuild) FORCE=1;; --engine-mode) shift; [[ ${1:-} == system || ${1:-} == bundled ]] || { fail '--engine-mode expects system or bundled'; return 1; }; ENGINE_MODE=$1;; --help) usage; return 0;; *) fail "Unknown initializer option: $1"; return 1;; esac
        shift
    done
    printf '=================================================\n             VMBENCH INITIALIZATION\n=================================================\n'
    detect_linux; platform_diagnostics; require_install_family
    GO_REQUIRED=$(awk '$1=="go"{gsub(/\r/,"",$2);print $2}' "$ROOT/go.mod" 2>/dev/null || echo 1.23.0)
    local preferred
    preferred=$(awk '$1=="toolchain"{gsub(/\r/,"",$2);print $2}' "$ROOT/go.mod" 2>/dev/null || true)
    GO_VERSION=${preferred:-go1.23.12}
    if (( CHECK_ONLY )); then check_installation; return; fi
    [[ ! -L $STATE_DIR && ! -L $ROOT/engines ]] || { fail 'Installation state/engine directories must not be symlinks.'; return 1; }
    mkdir -p "$STATE_DIR"
    mkdir "$STATE_DIR/init.lock" 2>/dev/null || { fail 'Initialization lock exists. Another initializer may be active. After confirming it has stopped, remove only .vmbench/init.lock and rerun.'; return 1; }
    LOCK_HELD=1
    trap cleanup EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    STAGE=$(mktemp -d "$STATE_DIR/staging.XXXXXXXX"); mkdir "$STAGE/engines"
    : > "$STATE_DIR/last-build.log"
    INITIAL_SOURCE=$(fingerprint all)
    BUILD_CLI=0 BUILD_NATIVE=0 NEED_ENGINES=0
    local old_cli='' old_native='' source_cli source_native
    [[ ! -f $STATE_DIR/cli-source.sha256 ]] || old_cli=$(cat "$STATE_DIR/cli-source.sha256")
    [[ ! -f $STATE_DIR/native-source.sha256 ]] || old_native=$(cat "$STATE_DIR/native-source.sha256")
    source_cli=$(fingerprint cli); source_native=$(fingerprint native)
    if [[ -f $ROOT/go.mod && -d $ROOT/cmd ]]; then
        if ! validate_binary "$ROOT/vmbench" vmbench >/dev/null 2>&1 || (( FORCE )) || { [[ -z $old_cli || $old_cli != "$source_cli" ]]; }; then BUILD_CLI=1; fi
        if ! validate_binary "$ROOT/engines/compute-bench" compute-bench >/dev/null 2>&1 || (( FORCE )) || { [[ -z $old_native || $old_native != "$source_native" ]]; }; then BUILD_NATIVE=1; fi
    else
        [[ -x $ROOT/vmbench && -x $ROOT/engines/compute-bench ]] || { fail 'Neither complete prebuilt binaries nor buildable source is available.'; return 1; }
    fi
    if [[ $ENGINE_MODE == bundled ]]; then
        if [[ -f $ROOT/go.mod && -d $ROOT/cmd ]]; then
            if validate_binary "$ROOT/vmbench" vmbench >/dev/null 2>&1 && [[ $ENGINE_LINKAGE != static ]]; then BUILD_CLI=1; fi
            if validate_binary "$ROOT/engines/compute-bench" compute-bench >/dev/null 2>&1 && [[ $ENGINE_LINKAGE != static ]]; then BUILD_NATIVE=1; fi
        fi
        for name in fio stress-ng iperf3; do
            if ! resolve_engine "$name" >/dev/null 2>&1; then NEED_ENGINES=1; fi
        done
    else
        for name in fio stress-ng iperf3; do [[ -x $ROOT/engines/$name || -x $ROOT/vendor-engines/$name ]] || NEED_ENGINES=1; done
    fi
    INITIALIZATION_PHASE='dependency preparation'
    check_dependencies; install_packages
    INITIALIZATION_PHASE='Go toolchain selection'
    log CHECK 'Go toolchain selection'
    if (( VERBOSE )); then ensure_go_toolchain
    else ensure_go_toolchain >> "$STATE_DIR/last-build.log" 2>&1; fi
    log OK "Go toolchain: $GO_SELECTED"
    run_step 'CLI preparation' build_vmbench
    run_step 'Native engine preparation' build_native_engine
    INITIALIZATION_PHASE='benchmark engine preparation'
    prepare_benchmark_engines
    INITIALIZATION_PHASE='engine runtime validation'
    validate_engine_dependencies
    run_step 'Safe runtime checks' run_smoke_tests
    if (( BUILD_CLI )); then run_step 'Go unit tests' run_unit_tests; fi
    INITIALIZATION_PHASE='publishing validated installation'
    save_installation_state
    log SUCCESS 'VMBench initialization completed. Run ./vmbench quick, standard, or max.'
    log INFO 'Ready on this host. System-linked engines do not constitute a portable offline release.'
}
if [[ ${BASH_SOURCE[0]} == "$0" ]]; then main "$@"; fi
