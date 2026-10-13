#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
source "$ROOT/scripts/bootstrap-common.sh"
OFFLINE=0 YES=0 FORCE=0 ENGINE_MODE=system
GO_VERSION=go1.23.12 GO_REQUIRED=1.23.0 GO_BIN= GO_SELECTED=not-required
STATE_DIR="$ROOT/.vmbench"
STAGE= LOCK_HELD=0
PACKAGES=()
VERSION_NAMES=() VERSION_VALUES=() LINKAGES=() ORIGINS=()
usage() {
    cat <<'EOF'
Usage: ./init.sh [--offline] [--yes] [--rebuild] [--engine-mode system|bundled]
Online setup is the default. --yes authorizes installation of listed missing apt packages.
Offline mode performs no downloads, apt calls, or Go automatic toolchain selection.
System engines are host-linked development dependencies, not portable release bundles.
Bundled mode uses vendor-engines, or the existing checksum-pinned source build scripts.
EOF
}
detect_platform() {
    [[ $(uname -s) == Linux ]] || { fail 'VMBench initialization requires Linux (Ubuntu 22.04 or 24.04).'; return 1; }
    local release=${VMBENCH_OS_RELEASE:-/etc/os-release}
    [[ -r $release ]] || { fail "Cannot read OS identity: $release"; return 1; }
    OS_ID=$(sed -n 's/^ID=//p' "$release" | tr -d '"')
    OS_VERSION=$(sed -n 's/^VERSION_ID=//p' "$release" | tr -d '"')
    [[ $OS_ID == ubuntu && ( $OS_VERSION == 22.04 || $OS_VERSION == 24.04 ) ]] || { fail "Supported systems: Ubuntu 22.04/24.04; detected $OS_ID $OS_VERSION"; return 1; }
    log INFO "Operating System : Ubuntu $OS_VERSION"
}
detect_architecture() {
    ARCH=$(uname -m)
    [[ $ARCH == x86_64 ]] || { fail "Unsupported architecture: $ARCH (requires x86_64)"; return 1; }
    log INFO "Architecture     : $ARCH"
}
cleanup() {
    local status=$?
    trap - EXIT
    if [[ -n $STAGE && $STAGE == "$STATE_DIR"/staging.* && -d $STAGE ]]; then rm -rf -- "$STAGE"; fi
    if (( LOCK_HELD )); then rmdir -- "$STATE_DIR/init.lock" || true; fi
    if (( status != 0 )); then log ERROR 'Initialization failed; no successful new installation state was committed.' >&2; fi
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
            version=$(cd /; GOTOOLCHAIN=local "$candidate" version | awk '{print $3}')
            if go_compatible "$version"; then compatible=1; fi
        done
        (( compatible )) || PACKAGES+=(curl)
    fi
    if (( BUILD_NATIVE )); then command -v gcc >/dev/null || PACKAGES+=(build-essential); fi
    if [[ $ENGINE_MODE == bundled ]] && (( NEED_ENGINES )); then
        for utility in gcc make pkg-config python3 readelf; do
            command -v "$utility" >/dev/null || case "$utility" in gcc|make) PACKAGES+=(build-essential);; readelf) PACKAGES+=(binutils);; *) PACKAGES+=("$utility");; esac
        done
        # Static source builds need development headers in addition to basic compilation.
        if ! pkg-config --exists libaio 2>/dev/null && [[ ! -f /usr/include/libaio.h ]]; then PACKAGES+=(libaio-dev); fi
    elif [[ $ENGINE_MODE == system ]]; then
        local engine
        for engine in fio stress-ng iperf3; do
            if ! validate_binary "$ROOT/engines/$engine" "$engine" >/dev/null 2>&1 && ! command -v "$engine" >/dev/null; then PACKAGES+=("$engine"); fi
        done
    fi
    if ((${#PACKAGES[@]})); then mapfile -t PACKAGES < <(printf '%s\n' "${PACKAGES[@]}" | sort -u); fi
}
install_build_dependencies() {
    ((${#PACKAGES[@]})) || return 0
    log INFO "Missing packages: ${PACKAGES[*]}"
    (( ! OFFLINE )) || { fail "Offline initialization cannot install: ${PACKAGES[*]}. Supply local build tools/engines or a complete release."; return 1; }
    local answer privilege=()
    if (( ! YES )); then
        [[ -t 0 ]] || { fail 'Package installation needs confirmation; rerun interactively or with --yes.'; return 1; }
        read -r -p "Install only these packages using apt? [y/N] " answer
        [[ $answer == y || $answer == Y ]] || { fail 'Package installation declined.'; return 1; }
    fi
    if (( EUID != 0 )); then command -v sudo >/dev/null || { fail 'sudo unavailable; install listed packages as an administrator.'; return 1; }; privilege=(sudo); fi
    "${privilege[@]}" apt-get update
    # Suppress service restarts/configuration prompts, in particular the iperf3 daemon prompt.
    printf 'iperf3 iperf3/start_daemon boolean false\n' | "${privilege[@]}" debconf-set-selections
    "${privilege[@]}" env DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=l apt-get install -y --no-install-recommends "${PACKAGES[@]}"
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
    GO_REQUIRED=$(awk '$1=="go"{print $2}' "$ROOT/go.mod")
    [[ $GO_REQUIRED =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]] || { fail 'Cannot determine required Go version from go.mod'; return 1; }
    log CHECK "Go toolchain (go.mod requires $GO_REQUIRED; preferred $GO_VERSION; automatic switching disabled)"
    local candidate version requested=${GOTOOLCHAIN:-} configured= goenv preference no_download=0 allow_newer=0
    # Inspect persistent Go settings without allowing Go to perform a download.
    if command -v go >/dev/null; then
        goenv=$(cd /; GOTOOLCHAIN=local go env GOENV)
        if [[ -z $requested && -f $goenv ]]; then configured=$(sed -n 's/^GOTOOLCHAIN=//p' "$goenv"); requested=$configured; fi
        log INFO "Active GOTOOLCHAIN preference: ${requested:-auto (default)}"
    fi
    preference=$requested
    [[ $preference != path && $preference != *+path ]] || no_download=1
    [[ $preference != *+auto && $preference != *+path ]] || allow_newer=1
    requested=${requested%+auto}; requested=${requested%+path}
    if [[ $requested == go* ]] && ! go_compatible "$requested"; then
        log INFO "Requested $requested is incompatible; selecting a local compatible toolchain without modifying global settings."
        requested=
    fi
    for candidate in "$STATE_DIR/toolchains/$GO_VERSION/bin/go" "$(command -v go || true)"; do
        [[ -n $candidate && -x $candidate ]] || continue
        version=$(cd /; GOTOOLCHAIN=local "$candidate" version | awk '{print $3}')
        if go_compatible "$version" && { [[ $requested != go* ]] || [[ $requested == "$version" ]] || { (( allow_newer )) && [[ $(printf '%s\n' "${requested#go}" "${version#go}" | sort -V | head -n1) == "${requested#go}" ]]; }; }; then GO_BIN=$candidate; GO_SELECTED=$version; log OK "$version detected"; return 0; fi
        log INFO "Toolchain $version does not satisfy required/selected version."
    done
    if [[ $requested == go* && $requested != "$GO_VERSION" ]]; then fail "Selected $requested is unavailable locally. Put that toolchain on PATH, or set GOTOOLCHAIN=local to allow project-local $GO_VERSION."; return 1; fi
    (( ! no_download )) || { fail "GOTOOLCHAIN=$preference prohibits downloads and its toolchain is not available locally."; return 1; }
    (( ! OFFLINE )) || { fail 'Offline initialization needs local Go 1.23.12 or newer. Put it on PATH or provide .vmbench/toolchains/go1.23.12/bin/go.'; return 1; }
    go_compatible "$GO_VERSION" || { fail "go.mod requires $GO_REQUIRED, beyond the checksum-pinned $GO_VERSION. Supply a compatible toolchain on PATH."; return 1; }
    local archive="$STAGE/$GO_VERSION.linux-amd64.tar.gz" expected
    expected=$(cut -d ' ' -f1 "$ROOT/scripts/go-toolchain.lock")
    if command -v curl >/dev/null; then curl --fail --location --retry 2 "https://go.dev/dl/$GO_VERSION.linux-amd64.tar.gz" -o "$archive"
    elif command -v wget >/dev/null; then wget -O "$archive" "https://go.dev/dl/$GO_VERSION.linux-amd64.tar.gz"
    else fail 'Go download needs curl or wget; install either or provide a compatible local Go.'; return 1; fi
    [[ $(hash_file "$archive") == "$expected" ]] || { fail 'Go archive checksum mismatch'; return 1; }
    mkdir -p "$STATE_DIR/toolchains"
    tar -xzf "$archive" -C "$STAGE"
    GO_BIN="$STAGE/go/bin/go"
    version=$(GOTOOLCHAIN=local "$GO_BIN" version | awk '{print $3}')
    [[ $version == "$GO_VERSION" ]] || { fail 'Downloaded Go version does not match the pin'; return 1; }
    [[ ! -e $STATE_DIR/toolchains/$GO_VERSION ]] || { fail 'Existing project-local Go is invalid; move it aside before retrying.'; return 1; }
    mv "$STAGE/go" "$STATE_DIR/toolchains/$GO_VERSION"
    GO_BIN="$STATE_DIR/toolchains/$GO_VERSION/bin/go"; GO_SELECTED=$version
    log OK "$version installed locally; system Go unchanged"
}
go_local() {
    # Source builds currently use only the standard library. Offline module access is disabled explicitly.
    if (( OFFLINE )); then env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off "$GO_BIN" "$@"
    else env GOTOOLCHAIN=local "$GO_BIN" "$@"; fi
}
build_vmbench() {
    if (( BUILD_CLI )); then log BUILD 'VMBench CLI'; local version_flags; version_flags=$(build_version_ldflags); (cd "$ROOT"; CGO_ENABLED=0 go_local build -trimpath -ldflags "$version_flags" -o "$STAGE/vmbench" ./cmd/vmbench); log OK 'CLI build completed'
    else cp "$ROOT/vmbench" "$STAGE/vmbench"; log OK 'CLI unchanged; reusing executable'; fi
}
build_native_engine() {
    if (( BUILD_NATIVE )); then log BUILD 'Native CPU/Memory engine'; local linkage=dynamic; [[ $ENGINE_MODE != bundled ]] || linkage=static; compile_native_kernel "$STAGE/engines/compute-bench" "$linkage" gcc; log OK 'compute-bench compiled'
    else cp "$ROOT/engines/compute-bench" "$STAGE/engines/compute-bench"; log OK 'Native engine unchanged; reusing executable'; fi
}
prepare_benchmark_engines() {
    local engine source origin
    if [[ $ENGINE_MODE == bundled ]] && (( NEED_ENGINES )); then
        if (( ! OFFLINE )); then (cd "$ROOT"; python3 scripts/fetch-engines.py); fi
        (cd "$ROOT"; bash scripts/build-engines.sh)
    fi
    for engine in fio stress-ng iperf3; do
        origin=existing-local
        if [[ $ENGINE_MODE == bundled && -x $ROOT/vendor-engines/$engine ]]; then source="$ROOT/vendor-engines/$engine"; origin=vendor-bundle
        elif validate_binary "$ROOT/engines/$engine" "$engine" >/dev/null 2>&1; then source="$ROOT/engines/$engine"
                elif [[ $ENGINE_MODE == system ]] && source=$(command -v "$engine"); then origin=system-development
        else fail "$engine missing; run online initialization or supply a compatible bundled engine."; return 1; fi
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
        log OK "$name ready ($ENGINE_LINKAGE)"
    done
}
run_smoke_tests() {
    # These checks never start benchmark workloads or an iperf server.
    log CHECK 'CLI help/version and engine version/capability checks'
    timeout 10 "$STAGE/vmbench" --help >/dev/null
    timeout 10 "$STAGE/vmbench" --version >/dev/null
    if (( BUILD_CLI )); then log CHECK "Go unit tests using $GO_SELECTED"; (cd "$ROOT"; go_local test ./...); fi
}
save_installation_state() {
    local i temporary="$STATE_DIR/install-state.json.tmp" full current_cli current_native
    full=$(fingerprint all); [[ $full == "$INITIAL_SOURCE" ]] || { fail 'Source changed during initialization; rerun after editing is complete.'; return 1; }
    current_cli=$(fingerprint cli); current_native=$(fingerprint native)
    mkdir -p "$ROOT/engines"
    for name in vmbench engines/compute-bench engines/fio engines/stress-ng engines/iperf3; do
        if [[ ! -f $ROOT/$name ]] || [[ $(hash_file "$ROOT/$name") != "$(hash_file "$STAGE/$name")" ]]; then mv -f -- "$STAGE/$name" "$ROOT/$name"; fi
    done
    printf '%s\n' "$full" > "$STATE_DIR/source.sha256"
    printf '%s\n' "$current_cli" > "$STATE_DIR/cli-source.sha256"
    printf '%s\n' "$current_native" > "$STATE_DIR/native-source.sha256"
    printf '%s\n' "$GO_SELECTED" > "$STATE_DIR/go-toolchain"
    (cd "$ROOT"; sha256sum vmbench engines/compute-bench engines/fio engines/stress-ng engines/iperf3 .vmbench/source.sha256 .vmbench/cli-source.sha256 .vmbench/native-source.sha256 .vmbench/go-toolchain > .vmbench/install-files.sha256)
    {
        printf '{\n  "schema": %s,\n  "status": "validated",\n  "version": ' "$BOOTSTRAP_SCHEMA"; json_string "$(cat "$ROOT/VERSION" 2>/dev/null || echo unknown)"
        printf ',\n  "platform": '; json_string "Ubuntu $OS_VERSION"; printf ',\n  "architecture": '; json_string "$ARCH"
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
main() {
    while (($#)); do
        case $1 in --offline) OFFLINE=1;; --yes) YES=1;; --rebuild) FORCE=1;; --engine-mode) shift; [[ ${1:-} == system || ${1:-} == bundled ]] || { fail '--engine-mode expects system or bundled'; return 1; }; ENGINE_MODE=$1;; --help) usage; return 0;; *) fail "Unknown initializer option: $1"; return 1;; esac
        shift
    done
    printf '=================================================\n             VMBENCH INITIALIZATION\n=================================================\n'
    detect_platform; detect_architecture
    [[ ! -L $STATE_DIR && ! -L $ROOT/engines ]] || { fail 'Installation state/engine directories must not be symlinks.'; return 1; }
    mkdir -p "$STATE_DIR"
    mkdir "$STATE_DIR/init.lock" 2>/dev/null || { fail 'Initialization lock exists. Another initializer may be active. After confirming it has stopped, remove only .vmbench/init.lock and rerun.'; return 1; }
    LOCK_HELD=1
    trap cleanup EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    STAGE=$(mktemp -d "$STATE_DIR/staging.XXXXXXXX"); mkdir "$STAGE/engines"
    INITIAL_SOURCE=$(fingerprint all)
    BUILD_CLI=0 BUILD_NATIVE=0 NEED_ENGINES=0
    local old_cli= old_native= source_cli source_native
    [[ ! -f $STATE_DIR/cli-source.sha256 ]] || old_cli=$(cat "$STATE_DIR/cli-source.sha256")
    [[ ! -f $STATE_DIR/native-source.sha256 ]] || old_native=$(cat "$STATE_DIR/native-source.sha256")
    source_cli=$(fingerprint cli); source_native=$(fingerprint native)
    if [[ -f $ROOT/go.mod && -d $ROOT/cmd ]]; then
        if ! validate_binary "$ROOT/vmbench" vmbench >/dev/null 2>&1 || (( FORCE )) || { [[ -n $old_cli && $old_cli != "$source_cli" ]]; }; then BUILD_CLI=1; fi
        if ! validate_binary "$ROOT/engines/compute-bench" compute-bench >/dev/null 2>&1 || (( FORCE )) || { [[ -n $old_native && $old_native != "$source_native" ]]; }; then BUILD_NATIVE=1; fi
    else
        [[ -x $ROOT/vmbench && -x $ROOT/engines/compute-bench ]] || { fail 'Neither complete prebuilt binaries nor buildable source is available.'; return 1; }
    fi
    if [[ $ENGINE_MODE == bundled ]]; then
        if [[ -f $ROOT/go.mod && -d $ROOT/cmd ]]; then
            if validate_binary "$ROOT/vmbench" vmbench >/dev/null 2>&1 && [[ $ENGINE_LINKAGE != static ]]; then BUILD_CLI=1; fi
            if validate_binary "$ROOT/engines/compute-bench" compute-bench >/dev/null 2>&1 && [[ $ENGINE_LINKAGE != static ]]; then BUILD_NATIVE=1; fi
        fi
        for name in fio stress-ng iperf3; do
            if [[ -x $ROOT/vendor-engines/$name ]]; then continue; fi
            if ! validate_binary "$ROOT/engines/$name" "$name" >/dev/null 2>&1 || [[ $ENGINE_LINKAGE != static ]]; then NEED_ENGINES=1; fi
        done
    else
        for name in fio stress-ng iperf3; do [[ -x $ROOT/engines/$name || -x $ROOT/vendor-engines/$name ]] || NEED_ENGINES=1; done
    fi
    check_dependencies; install_build_dependencies; ensure_go_toolchain
    build_vmbench; build_native_engine; prepare_benchmark_engines
    validate_engine_dependencies; run_smoke_tests; save_installation_state
    log SUCCESS 'VMBench initialization completed.'
    log INFO 'Ready on this host. System-linked engines do not constitute a portable offline release.'
}
if [[ ${BASH_SOURCE[0]} == "$0" ]]; then main "$@"; fi
