#!/bin/sh
set -eu

CONFIG="${CONFIG_PATH:-/app/config/config.yaml}"

# 首次运行且未挂载配置时，生成最小可用配置
if [ ! -f "${CONFIG}" ]; then
  mkdir -p "$(dirname "${CONFIG}")"
  cat >"${CONFIG}" <<'CFG'
server:
  port: ":7575"

web:
  username: "admin"
  password: "admin"
CFG
  echo "[vohive] 已生成默认配置: ${CONFIG} (Web 账号密码 admin/admin)"
fi

mkdir -p /app/data /app/logs

exec /app/vohive -c "${CONFIG}" "$@"
