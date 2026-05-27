# Endpoint Service Map Model

This patch changes the synthetic APM service map from site nodes to conversation endpoint nodes.

Recommended POC setting:

```yaml
connectors:
  flow2span:
    service_map:
      enabled: true
      node_type: endpoint_dns
      emit_peer_service_links: true
      service_namespace: flow2span.network_endpoint
```

Supported `node_type` values:

- `endpoint_dns`: prefer reverse DNS, fall back to IP. Example: `db.curtis.com -> app.curtis.com`.
- `endpoint_ip`: always use IP. Example: `10.0.0.1 -> 10.0.0.2`.
- `site_display`: aggregate nodes by site display name.
- `community`: aggregate nodes by client/server community.
- `region`: aggregate nodes by region.
- `service`: aggregate nodes by logical app/service.

The represented trace still carries site, link, community, dedupe, and DNS attributes, so dashboards can group endpoint conversations back to sites and links.

## Cardinality note

Endpoint mode intentionally creates more APM service nodes than site mode. It is useful for the POC and for bounded top-talker conversations, but for 72-site production you should cap represented traces/top talkers or switch back to `site_display`, `community`, or `region` when viewing broad topology.
