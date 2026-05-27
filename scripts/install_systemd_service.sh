#!/usr/bin/env bash
set -euo pipefail

# Installs a systemd service for a prebuilt Flow2Span collector on Debian/EC2.
# Usage: sudo ./scripts/install_systemd_service.sh /path/to/netflowotelcol config/collector-config-poc.yaml

BIN_SRC="${1:-}"
CFG_SRC="${2:-config/collector-config-poc.yaml}"
INSTALL_DIR="/opt/flow2span-otelcol"
SERVICE_NAME="flow2spanotelcol.service"

if [[ $EUID -ne 0 ]]; then
  echo "Run as root: sudo $0 /path/to/netflowotelcol config/collector-config-poc.yaml" >&2
  exit 1
fi

if [[ -z "$BIN_SRC" || ! -f "$BIN_SRC" ]]; then
  echo "Binary not found: $BIN_SRC" >&2
  exit 1
fi
if [[ ! -f "$CFG_SRC" ]]; then
  echo "Config not found: $CFG_SRC" >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR/bin" "$INSTALL_DIR/config"
cp "$BIN_SRC" "$INSTALL_DIR/bin/netflowotelcol"
cp "$CFG_SRC" "$INSTALL_DIR/config/collector-config.yaml"
chmod +x "$INSTALL_DIR/bin/netflowotelcol"

cat > "/etc/systemd/system/$SERVICE_NAME" <<UNIT
[Unit]
Description=Flow2Span OpenTelemetry Collector
After=network-online.target
Wants=network-online.target

[Service]
User=root
Group=root
ExecStart=$INSTALL_DIR/bin/netflowotelcol --config=$INSTALL_DIR/config/collector-config.yaml
Restart=always
RestartSec=5
LimitNOFILE=1048576
WorkingDirectory=$INSTALL_DIR

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"
systemctl status "$SERVICE_NAME" --no-pager
