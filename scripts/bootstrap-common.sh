#!/usr/bin/env bash
# Shared by init.sh and start.sh. State is data: never source installation metadata.
BOOTSTRAP_SCHEMA=1
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
        cd "$ROOT"
        case "$scope" in
            cli) for item in cmd internal; do [[ ! -d $item ]] || find "$item" -type f -name '*.go' -print0; done; [[ ! -f go.mod ]] || printf '%s\0' go.mod; [[ ! -f go.sum ]] || printf '%s\0' go.sum ;;
            native) [[ ! -d native ]] || find native -type f -print0 ;;
            all) for item in cmd internal native scripts config; do [[ ! -d $item ]] || find "$item" -type f ! -path '*/__pycache__/*' -print0; done
                 for item in go.mod go.sum VERSION init.sh start.sh; do [[ ! -f $item ]] || printf '%s\0' "$item"; done ;;
        esac
    ) | sort -z | (cd "$ROOT"; xargs -0 -r sha256sum) | sha256sum | cut -d ' ' -f1
}
validate_binary() {
    local binary=$1 name=$2 output status=0 header machine linked=unknown
    [[ -f $binary && -x $binary ]] || { fail "$name executable missing or not executable: $binary"; return 1; }
    header=$(od -An -tx1 -N4 "$binary" | tr -d ' \n')
    machine=$(od -An -tu2 -j18 -N2 "$binary" | tr -d ' \n')
    [[ $header == 7f454c46 && $machine == 62 ]] || { fail "$name is not a Linux x86_64 ELF executable"; return 1; }
    output=$(LC_ALL=C ldd "$binary" 2>&1) || status=$?
    if [[ $output == *'not found'* ]]; then fail "$name has missing runtime libraries: $output"; return 1; fi
    if [[ $output == *'not a dynamic executable'* || $output == *'statically linked'* ]]; then
        linked=static
    elif (( status == 0 )); then linked=dynamic
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
    [[ $name != fio ]] || timeout 10 "$binary" --enghelp 2>&1 | grep -qE '(^|[[:space:]])io_uring([[:space:]]|$)' || { fail 'fio does not include the default io_uring engine'; return 1; }
    ENGINE_VERSION=$output
    ENGINE_LINKAGE=$linked
}
installation_ready() {
    local state="$ROOT/.vmbench" binary
    [[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || return 1
    local release=${VMBENCH_OS_RELEASE:-/etc/os-release} os_id os_version
    [[ -r $release ]] || return 1
    os_id=$(sed -n 's/^ID=//p' "$release" | tr -d '"')
    os_version=$(sed -n 's/^VERSION_ID=//p' "$release" | tr -d '"')
    [[ $os_id == ubuntu && ( $os_version == 22.04 || $os_version == 24.04 ) ]] || return 1
    [[ -f $state/install-state.json && -f $state/install-state.sha256 && -f $state/install-files.sha256 && -f $state/source.sha256 ]] || return 1
    (cd "$ROOT"; sha256sum -c --status .vmbench/install-state.sha256 && sha256sum -c --status .vmbench/install-files.sha256) || return 1
    grep -q '"schema": 1' "$state/install-state.json" || return 1
    [[ $(cat "$state/source.sha256") == "$(fingerprint all)" ]] || return 1
    for binary in vmbench engines/compute-bench engines/fio engines/stress-ng engines/iperf3; do
        validate_binary "$ROOT/$binary" "${binary##*/}" >/dev/null 2>&1 || return 1
    done
}

compile_native_kernel() {
    local output=$1 linkage=$2 compiler=$3 flags=()
    [[ $linkage != static ]] || flags=(-static)
    "$compiler" -O2 -fno-fast-math -pthread "${flags[@]}" "$ROOT/native/compute-bench.c" -lm -o "$output"
}
