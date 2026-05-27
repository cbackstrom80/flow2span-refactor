# Service Map Site Model

This patch changes represented traces so Splunk APM Service Map nodes are based on customer-facing network sites instead of only server communities such as `payment-services` or `database-services`.

## Why

The previous trace model emitted one span under the destination service name. Splunk APM therefore rendered isolated nodes like:

```text
payment-services
database-services
dns-services
external
```

For the POC, the customer wants the Service Map to represent network locations / communities, for example:

```text
US BRANCH 1 -> DATACENTER US
EUROPE 1    -> DATACENTER US
external    -> DATACENTER US
```

## Config

```yaml
connectors:
  flow2span:
    service_map:
      enabled: true
      node_type: site_display
      include_application_span: false
```

`node_type` options:

| Value | Service Map `service.name` behavior |
|---|---|
| `site_display` | Uses site `display_name`, such as `DATACENTER US` or `EUROPE 1`. Recommended for the POC. |
| `site_name` | Uses stable site IDs, such as `site-001`. |
| `region` | Groups all traffic by region. |
| `community` | Uses Client/Server Community names. |
| `service` | Uses server/application service names, closer to the earlier behavior. |

## Trace shape

Each represented flow/conversation now creates a two-service trace:

```text
Resource service.name = US BRANCH 1
  SpanKindClient: US BRANCH 1 -> DATACENTER US

Resource service.name = DATACENTER US
  SpanKindServer: receive payment-api / payment-services
```

The server span has the client span as its parent. This gives Splunk APM a real cross-service relationship to draw.

## What stays the same

The spans still include all flow context:

```text
client.community
server.community
client.site
server.site
application.name
link.name
flow.io.bytes
flow.io.packets
dedupe.*
flow.src.dns
flow.dst.dns
```

Metrics are unchanged, so site utilization, link utilization, top talkers, dedupe metrics, and DNS-enriched top talkers continue to work.

## POC display names

The bundled POC config uses:

```text
site-001 display_name: US BRANCH 1
site-002 display_name: EUROPE 1
site-003 display_name: DATACENTER US
```

For production, replace these with the customer’s 72 site names, for example `DATACENTER US`, `DATACENTER EU`, `EUROPE 1`, `AMER STORE 001`, etc.
