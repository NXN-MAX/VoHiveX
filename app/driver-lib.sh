#!/bin/sh
# Only host-kernel runtime matching is changed. No AT commands or package installs.
set -eu
SYS="${QMI_SYS_ROOT:-/host-sys}"
STATE="${QMI_STATE_ROOT:-/state}"
USB="$SYS/bus/usb"
SERIAL="$SYS/bus/usb-serial/drivers/option1"
MODULES='usbserial usb_wwan usbnet cdc_wdm option qmi_wwan'

log() { printf '[QMI_WWAN] %s\n' "$*"; }
fail() { log "ERROR: $*" >&2; exit 1; }
readf() { cat "$1" 2>/dev/null || true; }
driver_of() { basename "$(readlink "$1/driver" 2>/dev/null || printf none)"; }
is_dji() {
  [ "$(readf "$1/idVendor")" = 2ca3 ] && [ "$(readf "$1/idProduct")" = 4006 ]
}
has_id() { grep -qi '^2ca3 4006\( \|$\)' "$1" 2>/dev/null; }
write_sys() { printf '%s\n' "$2" > "$1"; }

inspect() {
  log "kernel=$(uname -r) architecture=$(uname -m)"
  for m in $MODULES; do
    if [ -d "$SYS/module/$m" ]; then log "$m loaded"; else log "$m not-loaded"; fi
  done
  for p in "$USB"/devices/*; do
    is_dji "$p" || continue
    log "DJI $(basename "$p") configuration=$(readf "$p/bConfigurationValue")"
    for i in "$p":*; do
      [ -r "$i/bInterfaceNumber" ] || continue
      log "$(basename "$i") if=$(readf "$i/bInterfaceNumber") class=$(readf "$i/bInterfaceClass") subclass=$(readf "$i/bInterfaceSubClass") protocol=$(readf "$i/bInterfaceProtocol") endpoints=$(readf "$i/bNumEndpoints") driver=$(driver_of "$i")"
    done
  done
}

validate_devices() {
  for p in "$USB"/devices/*; do
    is_dji "$p" || continue
    [ "$(readf "$p/bConfigurationValue")" = 1 ] || fail 'Unexpected DJI USB configuration'
    for n in 2 4; do
      i="$p:1.$n"
      [ "$(readf "$i/bInterfaceClass")" = ff ] || fail "Unexpected interface: $i"
      case "$(driver_of "$i")" in
        none|option|qmi_wwan) ;;
        *) fail "Another driver owns $i; will not detach it" ;;
      esac
    done
  done
}

init_state() {
  mkdir -p "$STATE"
  boot="$(readf /proc/sys/kernel/random/boot_id)"
  [ -n "$boot" ] || fail 'Cannot read boot ID'
  if [ "$(readf "$STATE/boot-id")" != "$boot" ]; then
    # These are application-owned state files only, never host configuration.
    rm -f "$STATE/owned-modules" "$STATE/option-id" "$STATE/qmi-id" "$STATE/ready"
    printf '%s\n' "$boot" > "$STATE/boot-id"
  fi
}

load_modules() {
  before=' '
  for m in $MODULES; do
    [ ! -d "$SYS/module/$m" ] || before="$before$m "
  done
  # Load in this order so option wins initial serial-interface probing.
  result=0
  modprobe option || result=1
  [ "$result" -ne 0 ] || modprobe qmi_wwan || result=1
  for m in $MODULES; do
    case "$before" in *" $m "*) continue ;; esac
    if [ -d "$SYS/module/$m" ] && ! grep -qx "$m" "$STATE/owned-modules" 2>/dev/null; then
      printf '%s\n' "$m" >> "$STATE/owned-modules"
    fi
  done
  [ "$result" -eq 0 ] || fail 'Loading existing host modules failed; see log and ownership state'
}

register_ids() {
  if has_id "$SERIAL/new_id" && [ ! -f "$STATE/option-id" ]; then
    fail 'An existing DJI option ID is not owned by this project; refusing to replace it'
  fi
  if has_id "$USB/drivers/qmi_wwan/new_id" && [ ! -f "$STATE/qmi-id" ]; then
    fail 'An existing DJI QMI ID is not owned by this project; refusing to replace it'
  fi
  if ! has_id "$SERIAL/new_id"; then
    # The usb-serial core propagates this registration to the USB driver too.
    : > "$STATE/option-id"
    write_sys "$SERIAL/new_id" '2ca3 4006 ff' || fail 'Cannot register option ID'
  elif [ ! -f "$STATE/option-id" ]; then
    fail 'An existing DJI option ID is not owned by this project; refusing to replace it'
  fi
  if ! has_id "$USB/drivers/qmi_wwan/new_id"; then
    : > "$STATE/qmi-id"
    # Borrow EC25 driver_info (DTR quirk); the modem USB ID is unchanged.
    write_sys "$USB/drivers/qmi_wwan/new_id" '2ca3 4006 ff 2c7c 0125' || fail 'Cannot register QMI ID'
  elif [ ! -f "$STATE/qmi-id" ]; then
    fail 'An existing DJI QMI ID is not owned by this project; refusing to replace it'
  fi
}

bind_to() {
  intf="$1"; want="$2"
  [ -d "$intf" ] || return 0
  current="$(driver_of "$intf")"
  [ "$current" != "$want" ] || return 0
  case "$current" in
    option|qmi_wwan)
      log "Reassigning $(basename "$intf"): $current -> $want"
      write_sys "$USB/drivers/$current/unbind" "$(basename "$intf")" || return 1 ;;
    none) ;;
    *) log "Conflict on $(basename "$intf"): $current; left untouched"; return 1 ;;
  esac
  write_sys "$USB/drivers/$want/bind" "$(basename "$intf")" || return 1
  [ "$(driver_of "$intf")" = "$want" ]
}

reconcile() {
  for p in "$USB"/devices/*; do
    is_dji "$p" || continue
    [ "$(readf "$p/bConfigurationValue")" = 1 ] || continue
    # Only the DJI AT and QMI interfaces are actively corrected.
    # Diagnostic/GNSS ports may also be exposed by option and are left alone.
    bind_to "$p:1.2" option || return 1
    bind_to "$p:1.4" qmi_wwan || return 1
  done
}

health() {
  found=0
  for p in "$USB"/devices/*; do
    is_dji "$p" || continue
    found=1
    [ "$(driver_of "$p:1.2")" = option ] || return 1
    [ "$(driver_of "$p:1.4")" = qmi_wwan ] || return 1
    qmi=0
    for node in "$p:1.4"/usbmisc/cdc-wdm*; do [ ! -e "$node" ] || qmi=1; done
    [ "$qmi" -eq 1 ] || return 1
    at=0
    for node in "$p:1.2"/ttyUSB*/tty/ttyUSB* "$p:1.2"/tty/ttyUSB*; do
      [ ! -e "$node" ] || at=1
    done
    [ "$at" -eq 1 ] || return 1
  done
  [ "$found" -eq 1 ]
}

cleanup() {
  [ -f "$STATE/boot-id" ] || { log 'No project-owned state'; return 0; }
  [ "$(readf "$STATE/boot-id")" = "$(readf /proc/sys/kernel/random/boot_id)" ] || {
    log 'State is from an earlier boot; nothing to undo in this kernel'; return 0;
  }
  # Caller must stop VoHive and this project's driver loop first.
  errors=0
  if [ -f "$STATE/qmi-id" ] && has_id "$USB/drivers/qmi_wwan/new_id"; then
    write_sys "$USB/drivers/qmi_wwan/remove_id" '2ca3 4006' || errors=1
  fi
  if [ -f "$STATE/option-id" ] && has_id "$USB/drivers/option/new_id"; then
    write_sys "$USB/drivers/option/remove_id" '2ca3 4006' || errors=1
  fi
  for p in "$USB"/devices/*; do
    is_dji "$p" || continue
    for i in "$p":*; do
      [ -r "$i/bInterfaceNumber" ] || continue
      driver="$(driver_of "$i")"
      case "$driver" in
        option) [ -f "$STATE/option-id" ] || continue ;;
        qmi_wwan) [ -f "$STATE/qmi-id" ] || continue ;;
        *) continue ;;
      esac
      write_sys "$USB/drivers/$driver/unbind" "$(basename "$i")" || errors=1
    done
  done
  for m in qmi_wwan option cdc_wdm usbnet usb_wwan usbserial; do
    grep -qx "$m" "$STATE/owned-modules" 2>/dev/null || continue
    [ -d "$SYS/module/$m" ] || continue
    # Never force removal; shared/busy modules are retained.
    if ! rmmod "$m"; then log "Retained busy/shared module: $m"; errors=1; fi
  done
  # usb-serial option1 has no remove_id. Its dynamic ID goes away on unload/reboot.
  if [ -f "$STATE/option-id" ] && has_id "$SERIAL/new_id"; then
    log 'option1 dynamic ID remains until option can be safely unloaded or NAS reboots'
    errors=1
  fi
  [ "$errors" -eq 0 ] || return 1
  rm -f "$STATE/owned-modules" "$STATE/option-id" "$STATE/qmi-id" "$STATE/ready"
  log 'Project-owned runtime changes removed'
}
