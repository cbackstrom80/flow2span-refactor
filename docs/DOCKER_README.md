# Docker Compose POC Instructions

This compose scaffold builds and runs the Flow2Span POC collector plus a synthetic NetFlow v5 generator.

It is intended to be run from the **Flow2Span repo root** after this bundle has been copied into the repo.

## Important: Go version for OTel Collector Builder

OTel Collector Builder `v0.149.0` requires **Go >= 1.25.0**. The collector Dockerfile now uses:

```dockerfile
ARG GO_IMAGE=golang:1.25-bookworm
```

If Docker still shows Go 1.24.x, rebuild without cache:

```bash
docker compose build --no-cache collector
```

Or override explicitly:

```bash
docker compose build --no-cache --build-arg GO_IMAGE=golang:1.25-bookworm collector
```

If your environment cannot pull a Go 1.25 image, temporarily build with an older OCB version that supports your installed Go version:

```bash
docker compose build --no-cache --build-arg OCB_VERSION=v0.148.0 collector
```

For this POC, the preferred fix is to stay on OCB `v0.149.0` and use Go 1.25.

## Files added

```text
docker-compose.yml
docker-compose.debug-exporters.yml
docker/Dockerfile.collector
docker/Dockerfile.generator
docker/generator-entrypoint.sh
```

## 1. Copy the bundle into the repo

From the extracted POC bundle directory:

```bash
cp -R connector/flow2spanconnector/* ./connector/flow2spanconnector/
cp -R config ./config
cp -R scripts ./scripts
cp -R docs ./docs
cp -R docker ./docker
cp docker-compose.yml ./docker-compose.yml
cp docker-compose.debug-exporters.yml ./docker-compose.debug-exporters.yml
```

## 2. Build and start the collector

```bash
docker compose up --build collector
```

If you previously built it with an older image, use:

```bash
docker compose build --no-cache collector
docker compose up collector
```

The collector listens on:

```text
2055/udp  NetFlow
4739/udp  IPFIX placeholder
6343/udp  sFlow placeholder
4317      OTLP gRPC placeholder
4318      OTLP HTTP placeholder
8888      collector telemetry placeholder
```

## 3. Run the synthetic NetFlow generator

In another terminal:

```bash
docker compose --profile test up --build generator
```

Or run both collector and generator together:

```bash
docker compose --profile test up --build
```

The generator models:

```text
site-001 Seattle Branch
site-002 Calgary Branch
site-003 Denver DC
firewall and SD-WAN duplicate exporters
client communities
server communities
payment, database, DNS, web, and SaaS-style flows
top talkers
site/link utilization
```

## 4. Why the generator needs NET_ADMIN

The generator tries to bind NetFlow packets to simulated exporter source IPs:

```text
10.1.0.5 fw-site-001
10.1.0.1 sdwan-site-001
10.2.0.5 fw-site-002
10.2.0.1 sdwan-site-002
10.3.0.1 dc-edge-003
```

That requires adding addresses to the container loopback interface, so the compose file grants:

```yaml
cap_add:
  - NET_ADMIN
```

This lets the collector see exporter identity from packet source IP and exercise deduplication.

## 5. Validate dedupe behavior

Expected behavior:

```text
fw-site-001 wins over sdwan-site-001
fw-site-002 wins over sdwan-site-002
SD-WAN duplicate bytes/packets are suppressed for site traffic and top talkers
```

Look in collector logs for metrics like:

```text
flow2span.dedup.duplicate_flows
flow2span.dedup.duplicate_bytes_suppressed
flow2span.dedup.duplicate_packets_suppressed
```

## 6. Validate site/link utilization

Expected metrics:

```text
flow2span.site.bits_per_second{site.name="site-001"}
flow2span.site.bits_per_second{site.name="site-002"}
flow2span.link.utilization_percent{link.name="site-001-wan-primary"}
flow2span.link.utilization_percent{link.name="site-002-wan-primary"}
```

## 7. Validate top talker trace linking

Top talker metrics should include:

```text
flow2span.top_talker.bits_per_second
trace.id
span.id
client.community
server.community
flow.src.ip
flow.dst.ip
flow.src.dns
flow.dst.dns
```

The represented traces should include:

```text
dedupe.primary_exporter
dedupe.suppressed_exporters
link.utilization_percent
client.community
server.community
application.name
```

## 8. Common issue: builder output path

The collector Dockerfile assumes the generated binary is:

```text
./dist/netflowotelcol
```

If your `builder-config.yaml` outputs somewhere else, update this line in `docker/Dockerfile.collector`:

```dockerfile
COPY --from=builder /src/dist/netflowotelcol /opt/flow2span/bin/netflowotelcol
```

## 9. Common issue: Docker Desktop networking

The duplicate-exporter simulation depends on Linux networking behavior. It is most reliable on a Linux Docker host or EC2 instance.

On Docker Desktop for macOS, the generator still runs, but exporter source-IP simulation may not behave exactly like Linux host networking. For the most realistic POC, run compose on the Debian/EC2 host where the collector will be tested.

## 10. Tear down

```bash
docker compose --profile test down --remove-orphans
```

To rebuild cleanly:

```bash
docker compose --profile test build --no-cache
```
