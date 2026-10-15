#!/usr/bin/env bash
# shellcheck disable=SC2153
# PRIVILEGE is the platform module output; privilege is the local command array.
# Translate logical requirements without enabling repositories or upgrading the OS.
package_for() {
    case "$FAMILY:$1" in
        debian:compiler) echo build-essential;; rhel:compiler|suse:compiler|arch:compiler) echo gcc;; alpine:compiler) echo build-base;;
        debian:libaio) echo libaio-dev;; rhel:libaio|suse:libaio) echo libaio-devel;; arch:libaio) echo libaio;; alpine:libaio) echo libaio-dev;;
        debian:pkg-config) echo pkg-config;; rhel:pkg-config|alpine:pkg-config) echo pkgconf;; suse:pkg-config) echo pkg-config;; arch:pkg-config) echo pkgconf;;
        *) printf '%s\n' "$1";;
    esac
}
add_package() { PACKAGES+=("$(package_for "$1")"); }
install_packages() {
    ((${#PACKAGES[@]})) || return 0
    log INFO "Missing packages ($PACKAGE_MANAGER): ${PACKAGES[*]}"
    (( ! OFFLINE )) || { fail "Offline initialization cannot install: ${PACKAGES[*]}. Supply local tools/engines or a complete release."; return 1; }
    require_install_family
    command -v "$PACKAGE_MANAGER" >/dev/null || { fail "Missing package manager $PACKAGE_MANAGER for $OS_PRETTY. Supply dependencies manually; no alternate repositories are enabled."; return 1; }
    local answer privilege=()
    if (( ! YES )); then
        [[ -t 0 ]] || { fail 'Package installation needs confirmation; rerun interactively or with --yes.'; return 1; }
        read -r -p "Install only these packages using $PACKAGE_MANAGER? [y/N] " answer
        [[ $answer == y || $answer == Y ]] || { fail 'Package installation declined.'; return 1; }
    fi
    if (( EUID != 0 )); then
        case $PRIVILEGE in sudo|doas) privilege=("$PRIVILEGE");; *) fail 'No sudo/doas available. Ask an administrator to install the listed packages.'; return 1;; esac
    fi
    local status=0
    case $PACKAGE_MANAGER in
        apt-get)
            "${privilege[@]}" apt-get update || return
            if [[ " ${PACKAGES[*]} " == *' iperf3 '* ]] && command -v debconf-set-selections >/dev/null; then
                printf 'iperf3 iperf3/start_daemon boolean false\n' | "${privilege[@]}" debconf-set-selections
            fi
            "${privilege[@]}" env DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=l apt-get install -y --no-install-recommends "${PACKAGES[@]}" || status=$?;;
        dnf) "${privilege[@]}" dnf install -y --setopt=install_weak_deps=False "${PACKAGES[@]}" || status=$?;;
        zypper) "${privilege[@]}" zypper --non-interactive install --no-recommends "${PACKAGES[@]}" || status=$?;;
        # Do not refresh Arch databases without a full upgrade. Use existing synchronized metadata.
        pacman) "${privilege[@]}" pacman -S --needed --noconfirm "${PACKAGES[@]}" || status=$?;;
        apk) "${privilege[@]}" apk add --no-cache "${PACKAGES[@]}" || status=$?;;
        *) fail "No safe installation adapter for $PACKAGE_MANAGER"; return 1;;
    esac
    (( status == 0 )) || { fail "Could not install requested components: ${PACKAGES[*]} (exit $status). Check enabled repositories/package names with your administrator, or supply verified bundled engines. No extra repositories, signature bypass, or OS upgrades were requested."; return "$status"; }
}
