#!/usr/bin/env bash
set -euo pipefail

# Run from the root of the flow2span repo after building the collector.
CONFIG="${1:-config/collector-config-poc.yaml}"
BIN="${FLOW2SPAN_BIN:-}"

if [[ -z "$BIN" ]]; then
  if [[ -x ./dist/netflowotelcol ]]; then
    BIN=./dist/netflowotelcol
  elif [[ -x ./bin/netflowotelcol ]]; then
    BIN=./bin/netflowotelcol
  else
    echo "Could not find collector binary. Set FLOW2SPAN_BIN=/path/to/netflowotelcol" >&2
    exit 1
  fi
fi

exec "$BIN" --config "$CONFIG"
