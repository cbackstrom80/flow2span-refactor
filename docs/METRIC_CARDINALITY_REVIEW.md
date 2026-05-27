# Metric Cardinality Review and Trace-Linking Fix

## Summary

The previous POC emitted `trace.id` and `span.id` as dimensions on `flow2span.top_talker.*` metrics. That is useful for quick debugging, but it is not safe as a default because every represented trace/window can create new dimension values. In Splunk Observability, those IDs can quickly become high-cardinality metric time series.

This patch changes the default behavior:

```yaml
top_talkers:
  include_trace_id_attribute: false
  include_span_id_attribute: false
  drop_trace_id_dimension_for_metrics: true
```

The represented traces are still emitted. The metrics keep stable dimensions that are useful for dashboards and filtering.

## Keep as metric dimensions

These are acceptable for the POC and usually acceptable in production when bounded:

```text
deployment.environment
telemetry.source
site.name
site.region
site.role
link.name
link.provider
link.direction
client.community
server.community
application.name
service.name
traffic.class
top_talker.rank
top_talker.score_key
network.transport
```

## Use carefully

These are intentionally diagnostic/high-cardinality. Keep them only on bounded Top Talker metrics, not broad rollup metrics:

```text
flow.src.ip
flow.dst.ip
flow.dst.port
flow.src.dns
flow.dst.dns
```

For the POC they are useful because Top Talkers are capped. For production, keep the cap low, for example 10 per site/link, and avoid emitting endpoint dimensions on always-on site/link rollups.

## Do not use as metric dimensions by default

```text
trace.id
span.id
trace.url
flow.src.port
full 5-tuple on all rollup metrics
raw exporter sequence ids
```

These should stay on traces/events/logs, not high-volume metrics.

## How to link from metrics to traces now

Use a dashboard drilldown that carries stable filters and the time range into APM trace search:

```text
deployment.environment
client.community
server.community
flow.src.ip or flow.src.dns
flow.dst.ip or flow.dst.dns
application.name
link.name
```

The represented trace includes the same attributes, so searching traces over the same time window will return the corresponding network conversation without making `trace.id` a metric dimension.

## Production recommendation for 72 sites

Recommended production settings:

```yaml
top_talkers:
  enabled: true
  limit: 10
  scopes:
    - global
    - site
    - link
    - community_dependency
  emit_metrics: true
  emit_traces: true
  include_trace_id_attribute: false
  include_span_id_attribute: false
  drop_trace_id_dimension_for_metrics: true
  max_series_per_window: 10000

dns:
  emit_on_top_talker_metrics: true
  emit_on_represented_traces: true
```

If metric cardinality becomes too high, first disable DNS dimensions on top-talker metrics, then reduce top-talker scope count or limit.
