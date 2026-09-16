#!/bin/sh
set -eu
driver_pid=''; app_pid=''; scheduler_pid=''
stop_children() {
  trap - TERM INT
  [ -z "$scheduler_pid" ] || kill -TERM "$scheduler_pid" 2>/dev/null || true
  [ -z "$app_pid" ] || kill -TERM "$app_pid" 2>/dev/null || true
  [ -z "$driver_pid" ] || kill -TERM "$driver_pid" 2>/dev/null || true
  wait || true
  scheduler_pid=''; app_pid=''; driver_pid=''
}
trap 'stop_children; exit 0' TERM INT
trap stop_children EXIT

python3 -B /opt/vohivex/init-config.py
/bin/sh /opt/vohivex/driver.sh run & driver_pid=$!
# Allow driver initialization before existing device workers start. The UI still
# starts without a plugged-in modem; the driver loop handles later enumeration.
count=0
while ! /bin/sh /opt/vohivex/driver.sh health; do
  kill -0 "$driver_pid" 2>/dev/null || { wait "$driver_pid"; exit 1; }
  count=$((count + 1))
  [ "$count" -lt 15 ] || break
  sleep 1 & wait $! || true
done

config="${CONFIG_PATH:-/app/config/config.yaml}"
[ -s "$config" ] || { echo 'Missing prepared configuration' >&2; exit 1; }
cd /app
# Keep kernel-module capability in the driver loop only. Exec drops it from the
# Go application's bounding/permitted/effective sets, along with SETPCAP.
setpriv --bounding-set=-sys_module,-setpcap --inh-caps=-all --ambient-caps=-all \
  --no-new-privs /app/vohive -c "$config" & app_pid=$!
# The gateway and durable scheduler use the same restricted capabilities.
setpriv --bounding-set=-sys_module,-setpcap --inh-caps=-all --ambient-caps=-all \
  --no-new-privs python3 -B /opt/vohivex/scheduler/server.py & scheduler_pid=$!
while kill -0 "$scheduler_pid" 2>/dev/null && kill -0 "$driver_pid" 2>/dev/null && kill -0 "$app_pid" 2>/dev/null; do
  sleep 2 & wait $! || true
done
echo 'A required child process exited; restarting the container' >&2
exit 1
