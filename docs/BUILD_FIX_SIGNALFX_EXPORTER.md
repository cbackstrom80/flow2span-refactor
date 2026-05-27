# Build fix: unknown exporter type "signalfx"

## Symptom

```text
'exporters' unknown type: "signalfx" for id: "signalfx"
valid values: [debug otlp_grpc otlp otlp_http otlphttp]
```

## Cause

The collector runtime config includes:

```yaml
exporters:
  signalfx:
```

But the custom collector binary was built without the SignalFx exporter component in `builder-config.yaml`.

## Fix

This bundle adds the exporter to `builder-config.yaml`:

```yaml
exporters:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/exporter/signalfxexporter v0.149.0
```

The default POC runtime config now exports generated Flow2Span traces and metrics to both `debug` and `signalfx`:

```yaml
service:
  pipelines:
    traces:
      exporters: [debug, signalfx]
    metrics:
      exporters: [debug, signalfx]
```

## Rebuild

```bash
docker compose build --no-cache collector
docker compose --profile test up --build
```

## Environment

Create `.env` next to `docker-compose.yml`:

```bash
SPLUNK_ACCESS_TOKEN=REDACTED
SPLUNK_REALM=us1
DEPLOYMENT_ENVIRONMENT=netflowpoc
```
