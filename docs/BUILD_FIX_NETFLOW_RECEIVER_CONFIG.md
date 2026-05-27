# Fix: netflowreceiver.Config invalid key: endpoint

The OpenTelemetry contrib `netflowreceiver` does not use `endpoint`. It expects separate `hostname` and `port` fields.

Old invalid config:

```yaml
receivers:
  netflow:
    scheme: netflow
    endpoint: 0.0.0.0:2055
```

Fixed config:

```yaml
receivers:
  netflow:
    scheme: netflow
    hostname: 0.0.0.0
    port: 2055
    sockets: 4
    workers: 8
    queue_size: 10000
```

Also note that the current receiver emits the exporter/sampler IP as `flow.sampler_address`, so the POC config now uses:

```yaml
exporter_ip_key: flow.sampler_address
```

This lets the dedupe layer identify the firewall versus SD-WAN exporter based on the UDP source/sampler address.
