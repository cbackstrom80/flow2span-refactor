#!/usr/bin/env bash
set -euo pipefail

EXPORTER_IPS=(
  "10.1.0.5"
  "10.1.0.1"
  "10.2.0.5"
  "10.2.0.1"
  "10.3.0.1"
)

run_privileged() {
  if [[ "$(id -u)" == "0" ]]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    echo "ERROR: must run as root or install sudo." >&2
    exit 1
  fi
}

case "$(uname -s)" in
  Linux)
    for ip_addr in "${EXPORTER_IPS[@]}"; do
      if ip addr show dev lo | grep -q "${ip_addr}/32"; then
        echo "removing loopback alias: ${ip_addr}"
        run_privileged ip addr del "${ip_addr}/32" dev lo || true
      else
        echo "loopback alias not present: ${ip_addr}"
      fi
    done
    ;;
  Darwin)
    for ip_addr in "${EXPORTER_IPS[@]}"; do
      echo "removing macOS loopback alias: ${ip_addr}"
      run_privileged ifconfig lo0 -alias "${ip_addr}" || true
    done
    ;;
  *)
    echo "Unsupported OS: $(uname -s)" >&2
    exit 1
    ;;
esac
