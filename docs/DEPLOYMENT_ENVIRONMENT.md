# Setting `deployment.environment`

The POC connector now supports setting the environment value without editing Go code.

The collector config uses the OTel environment provider:

```yaml
connectors:
  flow2span:
    environment: ${env:DEPLOYMENT_ENVIRONMENT}
    environment_key: ${env:DEPLOYMENT_ENVIRONMENT_KEY}
```

The Docker Compose file provides defaults:

```yaml
environment:
  DEPLOYMENT_ENVIRONMENT: ${DEPLOYMENT_ENVIRONMENT:-poc}
  DEPLOYMENT_ENVIRONMENT_KEY: ${DEPLOYMENT_ENVIRONMENT_KEY:-deployment.environment}
```

## Run with the default POC environment

```bash
docker compose --profile test up --build
```

This emits resource attributes like:

```text
deployment.environment=poc
```

## Override for a customer POC

```bash
DEPLOYMENT_ENVIRONMENT=customer-poc docker compose --profile test up --build
```

## Override for production

```bash
DEPLOYMENT_ENVIRONMENT=prod docker compose up collector
```

## Use a different key name

Most Splunk/OTel dashboards should use the standard key `deployment.environment`. If you need a custom key:

```bash
DEPLOYMENT_ENVIRONMENT=prod \
DEPLOYMENT_ENVIRONMENT_KEY=environment \
docker compose up collector
```

## Where the attribute is emitted

The connector adds the configured environment key/value to resource attributes for:

- represented traces
- connector-generated metrics

That means metrics such as these can be filtered by environment:

```text
flow2span.site.bits_per_second{deployment.environment="poc"}
flow2span.link.utilization_percent{deployment.environment="poc"}
flow2span.top_talker.bits_per_second{deployment.environment="poc"}
```
