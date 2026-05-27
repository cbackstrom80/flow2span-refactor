#!/usr/bin/env bash
set -euo pipefail

cd /opt/flow2span-poc

if [[ "${USE_LOOPBACK_EXPORTERS:-true}" == "true" ]]; then
  echo "[generator] Adding simulated exporter IPs to loopback. This requires NET_ADMIN capability."
  ./scripts/setup_loopback_exporters.sh || {
    echo "[generator] WARNING: could not add loopback aliases. Exporter source IP simulation may fail." >&2
  }
fi

echo "[generator] Sending NetFlow v5 traffic to ${TARGET_HOST}:${TARGET_PORT} for ${DURATION}s at rate ${RATE}."
exec python3 scripts/generate_netflow_v5.py \
  --target "${TARGET_HOST}" \
  --port "${TARGET_PORT}" \
  --duration "${DURATION}" \
  --rate "${RATE}"
