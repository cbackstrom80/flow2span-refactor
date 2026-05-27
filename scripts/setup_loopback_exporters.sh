#!/usr/bin/env bash
set -euo pipefail

# Adds loopback aliases so the NetFlow v5 generator can bind UDP packets to exporter IPs.
# This lets the collector see packets as if they came from SD-WAN routers/firewalls.
# Safe to rerun.
#
# Docker: run as root with cap_add: [NET_ADMIN]. No sudo required.
# Linux host: run directly as root or with sudo.
# macOS host: uses sudo if not root.

EXPORTER_IPS=(
  "10.1.0.5"   # fw-site-001
  "10.1.0.1"   # sdwan-site-001
  "10.2.0.5"   # fw-site-002
  "10.2.0.1"   # sdwan-site-002
  "10.3.0.1"   # dc-edge-003
)

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "ERROR: required command '$1' not found" >&2
    exit 1
  }
}

run_privileged() {
  if [[ "$(id -u)" == "0" ]]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    echo "ERROR: must run as root or install sudo. In Docker, add cap_add: [NET_ADMIN] and run as root." >&2
    exit 1
  fi
}

case "$(uname -s)" in
  Linux)
    need_cmd ip
    for ip_addr in "${EXPORTER_IPS[@]}"; do
      if ip addr show dev lo | grep -q "${ip_addr}/32"; then
        echo "loopback alias exists: ${ip_addr}"
      else
        echo "adding loopback alias: ${ip_addr}"
        run_privileged ip addr add "${ip_addr}/32" dev lo
      fi
    done
    echo "Done. Current loopback aliases:"
    ip addr show dev lo | grep -E '10\.[123]\.0\.[151]' || true
    ;;
  Darwin)
    need_cmd ifconfig
    for ip_addr in "${EXPORTER_IPS[@]}"; do
      echo "adding loopback alias on macOS: ${ip_addr}"
      run_privileged ifconfig lo0 alias "${ip_addr}" 255.255.255.255 || true
    done
    ifconfig lo0 | grep '10\.' || true
    ;;
  *)
    echo "Unsupported OS: $(uname -s)" >&2
    exit 1
    ;;
esac
