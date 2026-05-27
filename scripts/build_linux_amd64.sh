#!/usr/bin/env bash
set -euo pipefail

# Run from the root of the flow2span repo after copying this bundle into it.
# Produces a Linux x86_64 collector binary suitable for most EC2 instances.

if ! command -v go >/dev/null 2>&1; then
  echo "Go is not installed or not on PATH" >&2
  exit 1
fi

if [[ ! -f builder-config.yaml ]]; then
  echo "builder-config.yaml not found. Run this from the project root." >&2
  exit 1
fi

go install go.opentelemetry.io/collector/cmd/builder@v0.149.0
GOOS=linux GOARCH=amd64 "$(go env GOPATH)/bin/builder" --skip-strict-versioning --config=builder-config.yaml

if [[ -f ./dist/netflowotelcol ]]; then
  file ./dist/netflowotelcol
elif [[ -f ./bin/netflowotelcol ]]; then
  file ./bin/netflowotelcol
else
  echo "Build completed, but collector binary was not found in ./dist or ./bin. Check builder-config.yaml output_path." >&2
fi
