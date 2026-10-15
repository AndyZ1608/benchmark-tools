#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
source "$ROOT/scripts/bootstrap-common.sh"
reinit=0 mode_change=0 requested_mode='' init_args=()
# Only a leading option prefix belongs to startup. Everything after the first CLI argument is forwarded verbatim.
while (($#)); do
    case $1 in
        --reinit) reinit=1; init_args+=(--rebuild); shift;;
        --verbose) init_args+=(--verbose); shift;;
        --offline) init_args+=(--offline); shift;;
        --yes) init_args+=(--yes); shift;;
        --engine-mode) [[ $# -ge 2 ]] || { fail '--engine-mode needs system or bundled'; exit 1; }; requested_mode=$2; init_args+=(--engine-mode "$2"); shift 2;;
        --) shift; break;;
        *) break;;
    esac
done
if [[ -n $requested_mode ]]; then
    [[ $requested_mode == system || $requested_mode == bundled ]] || { fail '--engine-mode expects system or bundled'; exit 1; }
    if [[ ! -f $ROOT/.vmbench/install-state.json ]] || ! grep -q "\"engine_mode\": \"$requested_mode\"" "$ROOT/.vmbench/install-state.json"; then mode_change=1; fi
fi
# installation_ready verifies the same supported --version interface as init.sh.
if (( reinit || mode_change )) || ! installation_ready; then
    log INFO 'Installation missing, changed, or invalid; initializing VMBench...'
    bash "$ROOT/init.sh" "${init_args[@]}"
    installation_ready || { fail 'Post-initialization validation failed; application will not be launched.'; exit 1; }
fi
log INFO 'Starting VMBench...'
exec "$ROOT/vmbench" "$@"
