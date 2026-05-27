# Build Fix: OCB v0.149.0 Requires Go 1.25+

If the collector image fails with:

```text
go.opentelemetry.io/collector/cmd/builder@v0.149.0 requires go >= 1.25.0
running go 1.24.13; GOTOOLCHAIN=local
```

Use the updated Dockerfile in this bundle. It now starts from:

```dockerfile
ARG GO_IMAGE=golang:1.25-bookworm
FROM ${GO_IMAGE} AS builder
```

Then rebuild cleanly:

```bash
docker compose build --no-cache collector
docker compose up collector
```

Or explicitly pass the image:

```bash
docker compose build --no-cache --build-arg GO_IMAGE=golang:1.25-bookworm collector
```

Fallback only if Go 1.25 cannot be pulled:

```bash
docker compose build --no-cache --build-arg OCB_VERSION=v0.148.0 collector
```

The preferred POC path is Go 1.25 + OCB v0.149.0.
