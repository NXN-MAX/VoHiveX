#!/bin/sh
set -eu
case "$(uname -m)" in
  x86_64) ;;
  *) echo 'This release supports Linux amd64 only' >&2; exit 1 ;;
esac
binary="/opt/vohive-release/vohive-dji-amd64"
config="${CONFIG_PATH:-/app/config/config.yaml}"
[ -s "$binary" ] || { echo "Missing release: $binary" >&2; exit 1; }
# NAS ZIP extraction may remove executable bits. Use the image copy only
# after verifying that it is byte-for-byte identical to the mounted release.
if [ ! -x "$binary" ]; then
  cmp -s "$binary" /app/vohive || {
    echo 'Image binary differs from mounted release; refusing to start' >&2
    exit 1
  }
  binary=/app/vohive
fi
[ -s "$config" ] || { echo 'Missing prepared config; refusing default credentials' >&2; exit 1; }
cd /app
exec "$binary" -c "$config"
