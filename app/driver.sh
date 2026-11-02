#!/bin/sh
set -eu
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/driver-lib.sh"

case "${1:-inspect}" in
  inspect) inspect ;;
  health) health ;;
  container-health)
    # No plugged-in modem is a normal server state. Keep strict binding
    # checks when supported hardware is present; the runtime loop still uses
    # strict health so it reports 'waiting' while unplugged.
    for p in "$USB"/devices/*; do
      is_dji "$p" || continue
      health
      exit $?
    done
    exit 0 ;;
  cleanup) cleanup ;;
  run)
    inspect
    validate_devices
    init_state
    load_modules
    register_ids
    trap 'log "Loop stopped; bindings retained. Use cleanup after stopping VoHive to undo."; exit 0' TERM INT
    log 'Driver loop started for DJI 2ca3:4006; no dialling or modem commands'
    last=waiting
    while :; do
      if reconcile && health; then now=ready; else now=waiting; fi
      if [ "$now" != "$last" ]; then log "Device $now"; last="$now"; fi
      sleep 2 & wait $! || true
    done ;;
  *) fail 'Usage: driver.sh inspect|run|health|cleanup' ;;
esac
