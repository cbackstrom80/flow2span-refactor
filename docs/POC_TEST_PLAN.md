# POC Test Plan

## Goal

Demonstrate that Flow2Span can model 3 POC sites in the same way production will model 72 sites.

The customer should be able to consume:

1. Overall traffic per site.
2. Link utilization by configured link speed.
3. Client Community to Server Community dependencies.
4. Top Talkers by site/link/dependency.
5. DNS names for important IPs.
6. Dedupe between firewall and SD-WAN exporters.
7. Metric rows that link back to represented traces.

## Test setup

1. Build the custom collector.
2. Run the collector with `config/collector-config-poc.yaml`.
3. Add loopback aliases for exporter simulation.
4. Run `scripts/generate_netflow_v5.py`.
5. Watch debug exporter output or Splunk Observability.

## Expected behavior

### Dedupe

For duplicate conversations exported by both devices:

- `fw-site-001` wins over `sdwan-site-001`.
- `fw-site-002` wins over `sdwan-site-002`.
- Suppressed bytes should appear in `flow2span.dedup.duplicate_bytes_suppressed`.

### Site traffic

`flow2span.site.bits_per_second` should show at least:

- `site.name=site-001`
- `site.name=site-002`
- `site.name=site-003`

### Link utilization

`flow2span.link.utilization_percent` should use configured speeds:

- `site-001-wan-primary` = 500 Mbps
- `site-001-lte-backup` = 50 Mbps
- `site-002-wan-primary` = 1 Gbps
- `site-003-dc-internet` = 10 Gbps

### Top talkers

Top talker metrics should identify:

- `10.1.50.44 -> 10.3.30.15:443`
- `client.community=site-001-pos`
- `server.community=payment-services`
- `application.name=payment-api`
- `trace.id` and `span.id` populated

### Represented trace

The trace should show:

- Client community
- Server community
- Source and destination DNS/IP
- Link and utilization context
- Dedupe context

## Scale gate before 72-site production

Before production rollout, validate:

- 72-site inventory file can be loaded.
- CIDRs do not overlap unexpectedly.
- Exporters are mapped to sites and priorities.
- Every production site has at least one link speed configured.
- Top talker max series limits are acceptable.
- DNS cache hit rate is healthy.
- Dedupe ratio is plausible and not suppressing unique flows.
