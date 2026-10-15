#!/usr/bin/env bash
# shellcheck disable=SC2034
# API variables in this module are consumed by the sourcing entrypoints.
# Shared by init.sh and start.sh. State is data: never source installation metadata.
BOOTSTRAP_SCHEMA=2
source "$ROOT/scripts/install/platform.sh"
source "$ROOT/scripts/install/packages.sh"
log() { printf '[%s] %s\n' "$1" "$2"; }
fail() { log ERROR "$*" >&2; return 1; }
json_string() {
    local value=$1
    value=${value//\\/\\\\}; value=${value//\"/\\\"}
    value=${value//$'\n'/\\n}; value=${value//$'\r'/\\r}; value=${value//$'\t'/\\t}
    printf '"%s"' "$value"
}
hash_file() { sha256sum "$1" | cut -d ' ' -f1; }
fingerprint() {
    local scope=$1
    (
        cd "$ROOT" || exit 1
        case "$scope" in
            cli) for item in cmd internal; do [[ ! -d $item ]] || find "$item" -type f -name '*.go' -print0; done; [[ ! -f go.mod ]] || printf '%s\0' go.mod; [[ ! -f go.sum ]] || printf '%s\0' go.sum; [[ ! -f VERSION ]] || printf '%s\0' VERSION; printf '%s\0' scripts/bootstrap-common.sh init.sh ;;
            native) [[ ! -d native ]] || find native -type f -print0; printf '%s\0' scripts/bootstrap-common.sh init.sh ;;
            all) for item in cmd internal native scripts config; do [[ ! -d $item ]] || find "$item" -type f ! -path '*/__pycache__/*' -print0; done
                 for item in go.mod go.sum VERSION init.sh start.sh; do [[ ! -f $item ]] || printf '%s\0' "$item"; done ;;
        esac
    ) | sort -z | (cd "$ROOT" || exit 1; xargs -0 -r sha256sum) | sha256sum | cut -d ' ' -f1
}
validate_binary() {
    local binary=$1 name=$2 output status=0 header machine linked=unknown expected_machine=${ELF_MACHINE:-62}
    [[ -f $binary && -x $binary ]] || { fail "$name executable missing or not executable: $binary"; return 1; }
    header=$(od -An -tx1 -N4 "$binary" | tr -d ' \n')
    machine=$(od -An -tu2 -j18 -N2 "$binary" | tr -d ' \n')
    [[ $header == 7f454c46 && $machine == "$expected_machine" ]] || { fail "$name is not a Linux ${GO_ARCH:-amd64} ELF executable"; return 1; }
    # musl ldd rejects static ELF with "Not a valid dynamic program"; inspect
    # program headers first when available, rather than interpreting that as a loader failure.
    local elf_headers=
    if command -v readelf >/dev/null; then elf_headers=$(LC_ALL=C readelf -l "$binary" 2>/dev/null || true); fi
    if [[ $elf_headers == *'Elf file type is'* && $elf_headers != *INTERP* ]]; then
        output='statically linked'
    else output=$(LC_ALL=C ldd "$binary" 2>&1) || status=$?; fi
    if [[ $output == *'not found'* || $output == *'Error loading shared library'* || $output == *'Error relocating'* ]]; then fail "$name has missing runtime libraries: $output"; return 1; fi
    if [[ $output == *'not a dynamic executable'* || $output == *'statically linked'* ]] || { [[ $output == *'Not a valid dynamic program'* ]] && elf_no_interpreter "$binary"; }; then
        linked=static
    elif (( status == 0 )) || [[ $output == *'ld-musl-'* && $output == *' => '* ]]; then linked=dynamic
    else fail "$name dependency inspection failed: $output"; return 1
    fi
    output=$(timeout 10 "$binary" --version 2>&1) || { fail "$name --version failed: $output"; return 1; }
    case "$name" in
        vmbench) [[ $output == VMBench* ]] ;;
        compute-bench) [[ $output == compute-bench-v1* ]] ;;
        fio) [[ $output == fio-[0-9]* ]] ;;
        stress-ng) [[ $output == stress-ng* ]] ;;
        iperf3) [[ $output == iperf\ 3.* ]] ;;
    esac || { fail "$name returned an incompatible version: $output"; return 1; }
    # Executable health is mandatory; async engines are independent Disk capabilities.
    # Capture first so an execution/loader error cannot be hidden by a matching token.
    if [[ $name == fio ]]; then
        local engine_help
        engine_help=$(timeout 10 "$binary" --enghelp 2>&1) || { fail "fio engine query failed: $engine_help"; return 1; }
    fi
    ENGINE_VERSION=$output
    ENGINE_LINKAGE=$linked
}
elf_no_interpreter() {
    # ELF64 little-endian is already enforced by the supported architecture check.
    # This fallback avoids requiring binutils on an offline target with musl ldd.
    local binary=$1 offset size count type i
    offset=$(od -An -tu8 -j32 -N8 "$binary" | tr -d ' \n')
    size=$(od -An -tu2 -j54 -N2 "$binary" | tr -d ' \n')
    count=$(od -An -tu2 -j56 -N2 "$binary" | tr -d ' \n')
    [[ $offset =~ ^[0-9]{1,12}$ && $size == 56 && $count =~ ^[0-9]{1,4}$ ]] || return 1
    (( count > 0 && count <= 1024 )) || return 1
    for (( i=0; i<count; i++ )); do
        type=$(od -An -tu4 -j"$((offset+i*size))" -N4 "$binary" | tr -d ' \n')
        [[ $type =~ ^[0-9]+$ && $type != 3 ]] || return 1
    done
}
installation_ready() {
    local state="$ROOT/.vmbench" binary
    detect_linux >/dev/null 2>&1 && require_install_family >/dev/null 2>&1 || return 1
    [[ -f $state/install-state.json && -f $state/install-state.sha256 && -f $state/install-files.sha256 && -f $state/source.sha256 ]] || return 1
    (cd "$ROOT" || exit 1; sha256sum -c --status .vmbench/install-state.sha256 && sha256sum -c --status .vmbench/install-files.sha256) || return 1
    grep -q '"schema": 2' "$state/install-state.json" && grep -q '"status": "validated"' "$state/install-state.json" || return 1
    [[ $(cat "$state/source.sha256") == "$(fingerprint all)" ]] || return 1
    [[ -f $state/platform.sha256 && $(cat "$state/platform.sha256") == "$(platform_key)" ]] || return 1
    for binary in vmbench engines/compute-bench engines/fio engines/stress-ng engines/iperf3; do
        validate_binary "$ROOT/$binary" "${binary##*/}" >/dev/null 2>&1 || return 1
    done
}

compile_native_kernel() {
    local output=$1 linkage=$2 compiler=$3 flags=()
    [[ $linkage != static ]] || flags=(-static)
    case ${GO_ARCH:-$(uname -m)} in amd64|x86_64) flags+=(-march=x86-64 -mtune=generic);; arm64|aarch64) flags+=(-march=armv8-a);; *) fail 'Unsupported native architecture'; return 1;; esac
    "$compiler" -O2 -fno-fast-math -ffp-contract=off -pthread "${flags[@]}" "$ROOT/native/compute-bench.c" -lm -o "$output"
}

# VERSION is validated as data, then passed as a single linker argument (never evaluated).
build_version_ldflags() {
    [[ -f $ROOT/VERSION ]] || return 0
    local value
    value=$(tr -d '\r' < "$ROOT/VERSION")
    [[ $value =~ ^[[:alnum:]][[:alnum:].+_-]*$ ]] || { fail 'VERSION must contain one nonempty version token (letters, digits, dot, plus, underscore, hyphen).'; return 1; }
    printf '%s' "-X vmbench/internal/version.Value=$value"
}

find_compiler() {
    local compiler
    if [[ -n ${CC:-} ]]; then command -v "$CC"; return; fi
    for compiler in cc gcc clang; do command -v "$compiler" && return; done
    return 1
}
check_compiler_target() {
    local macros compiler=$1
    macros=$(printf '#include <features.h>\n' | "$compiler" -dM -E -x c -) || { fail 'Cannot inspect compiler target/libc headers'; return 1; }
    case $GO_ARCH in
        amd64) [[ $macros == *'define __x86_64__ 1'* ]] || { fail 'Compiler does not target x86_64'; return 1; };;
        arm64) [[ $macros == *'define __aarch64__ 1'* ]] || { fail 'Compiler does not target aarch64'; return 1; };;
    esac
    if [[ $LIBC == glibc && $macros != *'define __GLIBC__ '* ]] || [[ $LIBC == musl && $macros == *'define __GLIBC__ '* ]]; then
        fail "Compiler libc headers do not match the detected $LIBC build host; use a matching build VM/compiler."
        return 1
    fi
}
resolve_engine() {
    local name=$1 candidate last=''
    local candidates=("$ROOT/vendor-engines/$name" "$ROOT/engines/$name")
    [[ ${ENGINE_MODE:-system} != system ]] || candidates+=("$(command -v "$name" || true)")
    for candidate in "${candidates[@]}"; do
        [[ -n $candidate ]] || continue
        [[ ! -f $candidate ]] || last=$candidate
        if validate_binary "$candidate" "$name" >/dev/null 2>&1; then
            [[ ${ENGINE_MODE:-system} != bundled || $ENGINE_LINKAGE == static ]] || continue
            RESOLVED_ENGINE=$candidate
            case $candidate in "$ROOT/vendor-engines/"*) RESOLVED_ORIGIN=vendor-bundle;; "$ROOT/engines/"*) RESOLVED_ORIGIN=existing-local;; *) RESOLVED_ORIGIN=system-development;; esac
            return 0
        fi
    done
    [[ -z $last ]] || validate_binary "$last" "$name"
    return 1
}
check_installation() {
    local name status=0 candidate version
    # No mkdir, state writes, installation, compilation, or network operations.
    for name in vmbench compute-bench fio stress-ng iperf3; do
        if [[ $name == vmbench || $name == compute-bench ]]; then
            candidate="$ROOT/engines/$name"; [[ $name != vmbench ]] || candidate="$ROOT/vmbench"
            if ! validate_binary "$candidate" "$name"; then status=1; continue; fi
        else
            if ! resolve_engine "$name"; then fail "$name unavailable: supply a compatible bundle or run authorized online initialization."; status=1; continue; fi
        fi
        log OK "$name: $ENGINE_VERSION ($ENGINE_LINKAGE)"
    done
    candidate="$ROOT/.vmbench/toolchains/${GO_VERSION:-go1.23.12}/bin/go"
    [[ -x $candidate ]] || candidate=$(command -v go || true)
    if [[ -n $candidate ]]; then version=$(cd /; go_inspect "$candidate" version); log INFO "$version";
    else log INFO 'Go unavailable (not needed for existing binaries; supply it for source rebuilds).'; fi
    if ! find_compiler >/dev/null; then log INFO 'No C compiler (required only for source rebuilds).'; fi
    if installation_ready; then log OK 'Installation state and source fingerprints valid'; else log INFO 'Installation is not current; source initialization or offline prebuilt validation is required.'; fi
    if (( status == 0 )); then timeout 30 "$ROOT/vmbench" doctor || status=$?; fi
    return "$status"
}

platform_key() {
    printf '%s\n' "$OS_ID" "$OS_VERSION" "$GO_ARCH" "$LIBC" "$LIBC_VERSION" | sha256sum | cut -d ' ' -f1
}

# Go 1.23+ telemetry can otherwise upload even with GOPROXY=off. The pinned
# toolchain's per-process test-directory override reads our shipped off mode,
# without changing the user's global telemetry preferences or writing counters.
verify_go_policy() {
    if [[ ! -r $ROOT/scripts/go-telemetry/mode ]] || [[ $(tr -d '\r\n' < "$ROOT/scripts/go-telemetry/mode") != off ]]; then
        fail "Missing or invalid Go telemetry-off policy: $ROOT/scripts/go-telemetry/mode (must contain off)"
        return 1
    fi
}
go_inspect() {
    verify_go_policy || return
    env TEST_TELEMETRY_DIR="$ROOT/scripts/go-telemetry" GOTOOLCHAIN=local "$@"
}
