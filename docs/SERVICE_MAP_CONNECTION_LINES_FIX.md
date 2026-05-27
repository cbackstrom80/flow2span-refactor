# Service Map Connection Lines Fix

## Problem

Traces were arriving in Splunk APM, but the Service Map showed isolated nodes without connecting dependency lines.

That can happen with synthetic traces because the spans do not come from real app instrumentation. Parent/child spans alone are not always enough for APM to infer a service dependency edge between two synthetic services.

## Fix

The POC now emits site-oriented service-map traces with explicit peer-service attributes on the client span.

Config:

```yaml
connectors:
  flow2span:
    service_map:
      enabled: true
      node_type: site_display
      include_application_span: false
      emit_peer_service_links: true
      service_namespace: flow2span.network_site
```

The resource `service.name` becomes the customer-facing site node:

```text
US BRANCH 1
EUROPE 1
DATACENTER US
external
```

The client span includes dependency-link hints:

```text
peer.service = DATACENTER US
server.address = DATACENTER US
server.port = 443
net.peer.name = DATACENTER US
rpc.system = flow2span
rpc.service = payment-api
```

The logical application/service name is preserved as:

```text
flow2span.logical_service.name = payment-services
application.name = payment-api
server.community = payment-services
```

The span-level `service.name` attribute was removed to avoid confusing the site-oriented resource `service.name` used by Splunk APM.

## Expected Service Map

Instead of disconnected application nodes like:

```text
payment-services
postgres
external
```

The map should show site dependencies:

```text
US BRANCH 1  -> DATACENTER US
EUROPE 1     -> DATACENTER US
US BRANCH 1  -> external
```

## Validation

Run:

```bash
docker compose build --no-cache collector
docker compose --profile test up --build
```

Then in Splunk APM, filter by:

```text
deployment.environment=netflowpoc
telemetry.source=flow2spanconnector
service.namespace=flow2span.network_site
```

You should see service-map nodes using site display names. If lines still do not appear immediately, give APM a few minutes and make sure the trace detail for a site span contains `peer.service`.
