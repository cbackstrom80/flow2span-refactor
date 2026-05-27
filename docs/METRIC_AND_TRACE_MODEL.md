# Flow2Span POC Metric and Trace Model

This POC models how the customer wants to consume flow telemetry: by site, link, client community, server community, top talker, and represented trace.

## Site rollup metrics

Always-on, low-cardinality metrics:

- `flow2span.site.bytes`
- `flow2span.site.packets`
- `flow2span.site.bits_per_second`
- `flow2span.site.conversations`
- `flow2span.site.unique_clients`
- `flow2span.site.unique_servers`

Recommended dimensions:

- `site.name`
- `site.region`
- `site.role`
- `traffic.direction`
- `application.name`

## Link utilization metrics

Always-on capacity metrics:

- `flow2span.link.bytes`
- `flow2span.link.packets`
- `flow2span.link.bits_per_second`
- `flow2span.link.utilization_percent`
- `flow2span.link.conversations`

Recommended dimensions:

- `site.name`
- `link.name`
- `link.provider`
- `link.circuit_id`
- `link.interface`
- `link.direction`
- `application.name`

Utilization formula:

```text
bits_per_second = bytes * 8 / aggregation_window_seconds
utilization_percent = bits_per_second / link.speed_bps * 100
```

## Client and Server Communities

NETSCOUT-inspired community metrics:

- `flow2span.client_community.bits_per_second`
- `flow2span.client_community.bytes`
- `flow2span.client_community.conversations`
- `flow2span.server_community.bits_per_second`
- `flow2span.server_community.bytes`
- `flow2span.server_community.conversations`
- `flow2span.community_dependency.bits_per_second`
- `flow2span.community_dependency.bytes`
- `flow2span.community_dependency.conversations`

Recommended dimensions:

- `client.community`
- `server.community`
- `client.site`
- `server.site`
- `application.name`
- `service.name`
- `link.name`

## Top Talker metrics

High-cardinality diagnostic metrics, bounded by top-N settings:

- `flow2span.top_talker.bytes`
- `flow2span.top_talker.packets`
- `flow2span.top_talker.bits_per_second`
- `flow2span.top_talker.conversations`
- `flow2span.top_talker.utilization_percent`
- `flow2span.top_talker.rank`

Important dimensions:

- `top_talker.rank`
- `top_talker.scope`
- `top_talker.score_key`
- `flow.src.ip`
- `flow.dst.ip`
- `flow.src.dns`
- `flow.dst.dns`
- `flow.src.port`
- `flow.dst.port`
- `network.transport`
- `client.community`
- `server.community`
- `client.site`
- `server.site`
- `application.name`
- `link.name`
- `trace.id`
- `span.id`

## Dedupe metrics

Used to prove that firewall and SD-WAN duplicate records are not double-counting traffic:

- `flow2span.dedup.input_flows`
- `flow2span.dedup.output_flows`
- `flow2span.dedup.duplicate_flows`
- `flow2span.dedup.duplicate_bytes_suppressed`
- `flow2span.dedup.duplicate_packets_suppressed`
- `flow2span.dedup.dedup_ratio`

Useful dimensions:

- `site.name`
- `exporter.primary`
- `exporter.duplicate`
- `exporter.role.primary`
- `exporter.role.duplicate`
- `dedupe.strategy`
- `dedupe.reason`

## DNS metrics

- `flow2span.dns.lookup.requests`
- `flow2span.dns.lookup.success`
- `flow2span.dns.lookup.failure`
- `flow2span.dns.lookup.timeout`
- `flow2span.dns.cache.hit`
- `flow2span.dns.cache.miss`
- `flow2span.dns.cache.size`
- `flow2span.dns.lookup.latency_ms`

## Represented traces

Top talker metrics include `trace.id` and `span.id`. The represented trace should be named one of:

- `network.top_talker`
- `network.community_dependency`

Root span attributes should include:

```yaml
client.community: site-001-pos
server.community: payment-services
client.site: site-001
server.site: site-003
flow.src.ip: 10.1.50.44
flow.src.dns: pos-044.site001.example.local
flow.dst.ip: 10.3.30.15
flow.dst.dns: payment-api-01.site003.example.local
flow.dst.port: 443
application.name: payment-api
link.name: site-001-wan-primary
link.speed_bps: 500000000
link.utilization_percent: 83.6
dedupe.enabled: true
dedupe.primary_exporter: fw-site-001
dedupe.suppressed_exporters: sdwan-site-001
dedupe.duplicate_count: 1
```
