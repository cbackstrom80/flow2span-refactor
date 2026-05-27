# Trace export fix

If metrics are visible but traces are not, the connector path is usually working, but the trace exporter path is not.
This bundle sends metrics through the `signalfx` exporter and traces through an explicit OTLP/HTTP trace endpoint:

```yaml
exporters:
  signalfx:
    access_token: ${env:SPLUNK_ACCESS_TOKEN}
    realm: ${env:SPLUNK_REALM}

  otlphttp/splunktraces:
    traces_endpoint: https://ingest.${env:SPLUNK_REALM}.signalfx.com/v2/trace/otlp
    headers:
      X-SF-TOKEN: ${env:SPLUNK_ACCESS_TOKEN}

service:
  pipelines:
    traces:
      receivers: [flow2span]
      processors: [batch]
      exporters: [debug, otlphttp/splunktraces]
    metrics:
      receivers: [flow2span]
      processors: [batch]
      exporters: [debug, signalfx]
```

## Validate locally

Run:

```bash
docker compose logs -f collector | grep -E "ResourceSpans|Span #|network|payment-services|site-003"
```

If the debug exporter prints spans, trace generation is working and the remaining issue is export/auth/realm/APM search.

## Search in Splunk APM

Look for services such as:

- `payment-services`
- `database-services`
- `dns-services`
- `site-003`

Filter by:

```text
deployment.environment=netflowpoc
telemetry.source=flow2spanconnector
```

## Required env file

Make sure the file is named `.env` and contains:

```bash
SPLUNK_ACCESS_TOKEN=REDACTED
SPLUNK_REALM=us1
DEPLOYMENT_ENVIRONMENT=netflowpoc
DEPLOYMENT_ENVIRONMENT_KEY=deployment.environment
```
