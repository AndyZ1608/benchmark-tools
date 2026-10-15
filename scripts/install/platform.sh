#!/usr/bin/env bash
# shellcheck disable=SC2034
# GO_ARCH and ELF_MACHINE are the shared platform API.
# OS metadata is data, never executable shell. Shared by initialization/startup.
os_value() {
    local key=$1 line value
    [[ -r $OS_RELEASE ]] || return 0
    while IFS= read -r line || [[ -n $line ]]; do
        [[ $line == "$key="* ]] || continue
        value=${line#*=}; value=${value%$'\r'}
        if [[ $value == \"*\" || $value == \'*\' ]]; then value=${value:1:${#value}-2}; fi
        printf '%s' "$value" | LC_ALL=C tr -cd ' -~'
        return
    done < "$OS_RELEASE"
}
detect_linux() {
    [[ $(uname -s) == Linux ]] || { fail 'VMBench requires Linux.'; return 1; }
    OS_RELEASE=${VMBENCH_OS_RELEASE:-/etc/os-release}
    OS_ID=$(os_value ID); OS_LIKE=$(os_value ID_LIKE); OS_VERSION=$(os_value VERSION_ID)
    OS_PRETTY=$(os_value PRETTY_NAME); OS_PRETTY=${OS_PRETTY:-${OS_ID:-unknown} ${OS_VERSION:-unknown}}
    ARCH=$(uname -m); KERNEL=$(uname -r)
    case $ARCH in x86_64) GO_ARCH=amd64; ELF_MACHINE=62;; aarch64|arm64) GO_ARCH=arm64; ELF_MACHINE=183;; *) fail "Unsupported architecture: $ARCH; targets are amd64 and arm64 (experimental)."; return 1;; esac
    FAMILY=unknown PACKAGE_MANAGER=none
    case $OS_ID in
        ubuntu|debian) FAMILY=debian; PACKAGE_MANAGER=apt-get;;
        rocky|almalinux|rhel|centos|fedora) FAMILY=rhel; PACKAGE_MANAGER=dnf;;
        opensuse*|sles|sled) FAMILY=suse; PACKAGE_MANAGER=zypper;;
        arch) FAMILY=arch; PACKAGE_MANAGER=pacman;;
        alpine) FAMILY=alpine; PACKAGE_MANAGER=apk;;
    esac
    # ID_LIKE is diagnostic only: untested derivatives must not inherit install permission.
    LIBC=unknown LIBC_VERSION=unknown
    local output
    output=$(getconf GNU_LIBC_VERSION 2>/dev/null || true)
    if [[ $output == glibc* ]]; then LIBC=glibc; LIBC_VERSION=${output#glibc };
    else
        output=$(ldd --version 2>&1 || true)
        if [[ $output == *musl* ]]; then LIBC=musl; LIBC_VERSION=$(printf '%s\n' "$output" | sed -n 's/^Version //p');
        elif [[ $output == *GLIBC* || $output == *'GNU libc'* || $output == *'GNU C Library'* ]]; then LIBC=glibc; LIBC_VERSION=$(printf '%s\n' "$output" | head -n1 | awk '{print $NF}'); fi
        LIBC_VERSION=${LIBC_VERSION:-unknown}
    fi
    EXECUTION_ENV=host-or-vm
    [[ ! -e /.dockerenv && ! -e /run/.containerenv ]] || EXECUTION_ENV=container
    CPU_FLAGS=unknown
    if [[ -r /proc/cpuinfo ]]; then
        CPU_FLAGS=$(awk -F: '/^(flags|Features)[ \t]*:/{sub(/^[ \t]+/,"",$2);print $2;exit}' /proc/cpuinfo)
        CPU_FLAGS=${CPU_FLAGS:-unknown}
    fi
    PRIVILEGE=unprivileged
    if (( EUID == 0 )); then PRIVILEGE=root; elif command -v sudo >/dev/null; then PRIVILEGE=sudo; elif command -v doas >/dev/null; then PRIVILEGE=doas; fi
}
platform_diagnostics() {
    log INFO "Operating System : $OS_PRETTY (ID=${OS_ID:-unknown}; ID_LIKE=${OS_LIKE:-none})"
    log INFO "Architecture     : $ARCH; kernel $KERNEL"
    log INFO "Package Manager  : $PACKAGE_MANAGER; Runtime: $LIBC $LIBC_VERSION"
    log INFO "Execution        : $EXECUTION_ENV; authorization $PRIVILEGE"
    if (( ${VERBOSE:-0} )); then log INFO "CPU features     : $CPU_FLAGS"; fi
    log INFO 'Qualification    : candidate target; runtime checks below apply only to this host. See docs/COMPATIBILITY.md.'
}
require_install_family() {
    [[ $FAMILY != unknown ]] || { fail "Unsupported OS identity '${OS_ID:-missing}' in $OS_RELEASE. No package installation attempted. Use a validated offline release or qualify a new adapter; ID_LIKE alone is insufficient."; return 1; }
}
