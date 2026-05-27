# Flow2Span Site/Community POC Bundle

This bundle packages the Flow2Span POC around the way the customer wants to consume the data:

- **Sites**: POC has 3 sites, production has 72 sites using the same schema.
- **Client Communities**: users, POS, IoT, VPN, branch users, etc.
- **Server Communities**: payment services, database services, DNS services, SaaS, etc.
- **Link speed and utilization**: configured per site/link.
- **Top Talkers**: emitted as metrics with `trace.id` and `span.id` for drillback.
- **Represented traces**: trace narrative for top talkers and community dependencies.
- **Dedupe**: suppresses duplicate flow records when SD-WAN routers and firewalls both export the same conversation.
- **DNS enrichment**: reverse DNS / host name enrichment for top talkers and traces.

## Contents

```text
connector/flow2spanconnector/    POC connector scaffold files
config/collector-config-poc.yaml Main inline POC collector config
config/sites.yaml                External inventory example for sites
config/links.yaml                External inventory example for link speeds
config/exporters.yaml            External inventory example for SD-WAN/firewall exporters
config/communities.yaml          Client/Server Community inventory
config/applications.yaml         Application classification rules
config/dns-hosts.yaml            DNS host hints for human-readable test expectations
scripts/generate_netflow_v5.py   Synthetic NetFlow v5 traffic generator
scripts/setup_loopback_exporters.sh
scripts/remove_loopback_exporters.sh
scripts/build_linux_amd64.sh
scripts/run_collector_debug.sh
scripts/install_systemd_service.sh
docs/METRIC_AND_TRACE_MODEL.md
docs/POC_TEST_PLAN.md
```

## POC topology

```text
site-001 / Seattle Branch / 10.1.0.0/16
  Client Communities:
    site-001-users / 10.1.10.0/24
    site-001-pos   / 10.1.50.0/24
  Exporters:
    fw-site-001    / 10.1.0.5 / priority 100
    sdwan-site-001 / 10.1.0.1 / priority 80
  Links:
    site-001-wan-primary / 500 Mbps
    site-001-lte-backup  / 50 Mbps

site-002 / Calgary Branch / 10.2.0.0/16
  Client Communities:
    site-002-users / 10.2.10.0/24
    site-002-iot   / 10.2.60.0/24
  Exporters:
    fw-site-002    / 10.2.0.5 / priority 100
    sdwan-site-002 / 10.2.0.1 / priority 80
  Links:
    site-002-wan-primary / 1 Gbps

site-003 / Denver DC / 10.3.0.0/16
  Server Communities:
    payment-services  / 10.3.30.0/24
    database-services / 10.3.40.0/24
    dns-services      / 10.3.53.0/24
  Exporters:
    dc-edge-003 / 10.3.0.1 / priority 90
  Links:
    site-003-dc-internet / 10 Gbps
```

## Copy into the repo

From a checkout of `https://github.com/cbackstrom80/flow2span`, copy this bundle into the repo root:

```bash
cp -R connector/flow2spanconnector/* ./connector/flow2spanconnector/
cp -R config ./config
cp -R scripts ./scripts
cp -R docs ./docs
```

The `collector-config-poc.yaml` is self-contained and inline for easier POC testing. The separate inventory files show how to split config for production inventory management.

## Build the collector on Debian/EC2

```bash
sudo apt update
sudo apt install -y golang-go git make build-essential python3

go install go.opentelemetry.io/collector/cmd/builder@v0.149.0
$(go env GOPATH)/bin/builder --skip-strict-versioning --config=builder-config.yaml
```

If building on a Mac for Linux EC2, compile for Linux:

```bash
GOOS=linux GOARCH=amd64 $(go env GOPATH)/bin/builder --skip-strict-versioning --config=builder-config.yaml
file ./dist/netflowotelcol
```

You want to see an ELF Linux binary, not Mach-O:

```text
ELF 64-bit LSB executable, x86-64
```

You can also use the helper:

```bash
./scripts/build_linux_amd64.sh
```

## Run the collector in debug mode

From the repo root:

```bash
./scripts/run_collector_debug.sh config/collector-config-poc.yaml
```

Or directly:

```bash
./dist/netflowotelcol --config config/collector-config-poc.yaml
```

## Run the NetFlow v5 site test

In another shell, add loopback aliases so the generator can bind to the simulated exporter IPs:

```bash
./scripts/setup_loopback_exporters.sh
```

Then generate traffic:

```bash
python3 scripts/generate_netflow_v5.py --target 127.0.0.1 --port 2055 --duration 120 --rate 8
```

Clean up loopback aliases afterward:

```bash
./scripts/remove_loopback_exporters.sh
```

## Why the loopback aliases matter

The NetFlow receiver typically identifies the exporter from the UDP source address. To model duplicate records from both firewall and SD-WAN devices, the generator binds packets to these fake exporter addresses:

```text
10.1.0.5 fw-site-001
10.1.0.1 sdwan-site-001
10.2.0.5 fw-site-002
10.2.0.1 sdwan-site-002
10.3.0.1 dc-edge-003
```

That lets the dedupe logic prove that the higher-priority firewall copy wins over the SD-WAN copy.

## Expected POC outputs

### Site traffic

Look for:

```text
flow2span.site.bits_per_second{site.name="site-001"}
flow2span.site.bits_per_second{site.name="site-002"}
flow2span.site.bits_per_second{site.name="site-003"}
```

### Link utilization

Look for:

```text
flow2span.link.utilization_percent{link.name="site-001-wan-primary"}
flow2span.link.utilization_percent{link.name="site-002-wan-primary"}
flow2span.link.utilization_percent{link.name="site-003-dc-internet"}
```

### Dedupe

Look for:

```text
flow2span.dedup.duplicate_flows
flow2span.dedup.duplicate_bytes_suppressed
flow2span.dedup.duplicate_packets_suppressed
```

Expected dedupe behavior:

```text
fw-site-001 wins over sdwan-site-001
fw-site-002 wins over sdwan-site-002
```

### Top talkers with trace linking

Look for top talker metric rows with:

```text
flow2span.top_talker.bits_per_second
flow.src.ip=10.1.50.44
flow.dst.ip=10.3.30.15
client.community=site-001-pos
server.community=payment-services
application.name=payment-api
trace.id=<represented trace id>
span.id=<represented root span id>
```

### DNS names

Top talker metrics and represented traces should include DNS attributes when reverse lookup or cache enrichment resolves them:

```text
flow.src.dns=pos-044.site001.example.local
flow.dst.dns=payment-api-01.site003.example.local
```

## Production scaling notes

For production with 72 sites:

- Keep the same `sites`, `links`, `exporters`, and `communities` schemas.
- Use external inventory files or generated config.
- Keep site and link metrics always-on.
- Keep top talkers bounded: 10 per site/link/dependency is usually safer than unlimited IP-level metrics.
- Dedupe before aggregation, otherwise traffic and utilization will be inflated.
- DNS must be async and cache-backed; never block the flow pipeline on DNS.

Recommended production settings:

```yaml
aggregation:
  window: 60s

top_talkers:
  enabled: true
  limit: 10
  scopes: [global, site, link, community_dependency]
  max_series_per_window: 10000

deduplication:
  enabled: true
  window: 60s
  strategy: prefer_highest_priority
  match_tolerance_percent: 15

dns:
  enabled: true
  timeout: 100ms
  max_concurrent_lookups: 200
  cache_ttl: 24h
  negative_cache_ttl: 30m
```

## Docker Compose POC

This bundle also includes a Docker Compose scaffold.

Files:

```text
docker-compose.yml
docker-compose.debug-exporters.yml
docker/Dockerfile.collector
docker/Dockerfile.generator
docker/generator-entrypoint.sh
docs/DOCKER_README.md
```

Run from the Flow2Span repo root after copying the bundle files into the repo:

```bash
docker compose up --build collector
```

In another terminal, run the NetFlow generator:

```bash
docker compose --profile test up --build generator
```

Or run both together:

```bash
docker compose --profile test up --build
```

See `docs/DOCKER_README.md` for the full workflow and troubleshooting notes.

## Docker build note

OTel Collector Builder `v0.149.0` requires Go `>=1.25.0`. The Docker scaffold uses `golang:1.25-bookworm`. If you hit a Go 1.24 error, rebuild cleanly:

```bash
docker compose build --no-cache collector
```

See `docs/BUILD_FIX_GO125.md`.

## Docker build note

This bundle includes `builder-config.yaml` at the bundle root. Run Docker Compose from this directory:

```bash
cd flow2span-site-poc-bundle
docker compose build --no-cache collector
docker compose --profile test up --build
```

If you see `open builder-config.yaml: no such file or directory`, you are building from the wrong directory or did not copy `builder-config.yaml` into the Docker build context.

## Docker generator loopback exporter note

The generator simulates firewall and SD-WAN exporters by adding fake exporter IPs to the container loopback interface and binding UDP packets to those source IPs. This requires Docker `NET_ADMIN` capability, already included in `docker-compose.yml` for the `generator` service.

If you see `Cannot assign requested address`, rebuild the generator and confirm loopback aliases are being added:

```bash
docker compose build --no-cache generator
docker compose --profile test up --build generator
```

The generator image should print `adding loopback alias: 10.1.0.5` and similar entries before sending NetFlow.


## Setting `deployment.environment`

The bundle now supports setting the standard OTel/Splunk resource attribute `deployment.environment` from an environment variable. Docker Compose defaults it to `poc`:

```bash
docker compose --profile test up --build
```

Override it at runtime:

```bash
DEPLOYMENT_ENVIRONMENT=customer-poc docker compose --profile test up --build
```

For production:

```bash
DEPLOYMENT_ENVIRONMENT=prod docker compose up collector
```

The attribute key also defaults to `deployment.environment`, but can be changed if needed:

```bash
DEPLOYMENT_ENVIRONMENT=prod DEPLOYMENT_ENVIRONMENT_KEY=environment docker compose up collector
```

See `docs/DEPLOYMENT_ENVIRONMENT.md` for details.


## Splunk Observability export

This bundle includes the native `signalfx` exporter in `builder-config.yaml`. The default runtime config exports generated Flow2Span metrics and represented traces to both `debug` and `signalfx`.

Create a `.env` file beside `docker-compose.yml`:

```bash
SPLUNK_ACCESS_TOKEN=REDACTED
SPLUNK_REALM=us1
DEPLOYMENT_ENVIRONMENT=netflowpoc
```

Then rebuild the collector so the `signalfx` exporter is included in the custom binary:

```bash
docker compose build --no-cache collector
docker compose --profile test up --build
```

If you see `unknown type: signalfx`, the collector image was built from an older `builder-config.yaml`; rebuild with `--no-cache`.

## Trace export note

This bundle sends metrics with the `signalfx` exporter and traces with `otlphttp/splunktraces` to `https://ingest.${SPLUNK_REALM}.signalfx.com/v2/trace/otlp`. If metrics appear but traces do not, check `docs/TRACE_EXPORT_FIX.md` and verify the debug exporter is printing `ResourceSpans`.



## Service Map Site Model

This bundle includes a site-oriented Service Map trace model. Set `connectors.flow2span.service_map.enabled: true` and `node_type: site_display` to render Splunk APM nodes as customer-facing sites such as `US BRANCH 1`, `EUROPE 1`, and `DATACENTER US` instead of only `payment-services` or `database-services`. See `docs/SERVICE_MAP_SITE_MODEL.md`.

## Service Map connection-line patch

This bundle includes an explicit Service Map edge fix for synthetic network traces. The connector now sets `peer.service`, `server.address`, `net.peer.name`, and `rpc.service` on client spans so Splunk APM can infer dependency lines between site nodes.

Recommended POC config:

```yaml
service_map:
  enabled: true
  node_type: site_display
  include_application_span: false
  emit_peer_service_links: true
  service_namespace: flow2span.network_site
```

See `docs/SERVICE_MAP_CONNECTION_LINES_FIX.md`.


## Endpoint-oriented APM Service Map

This bundle defaults to:

```yaml
service_map:
  node_type: endpoint_dns
```

That makes represented APM service map nodes look like conversation endpoints, for example:

```text
pos-044.site001.example.local -> payment-api-01.site003.example.local
10.1.50.44 -> 10.3.30.15
```

Use `endpoint_ip` if the customer wants IP-only nodes. Use `site_display` if the APM map becomes too busy during a larger rollout.


## Metric cardinality review

This bundle now disables `trace.id` and `span.id` as metric dimensions by default. Represented traces are still emitted, and dashboards should pivot to traces using stable attributes like `deployment.environment`, `client.community`, `server.community`, `flow.src.ip`/`flow.src.dns`, `flow.dst.ip`/`flow.dst.dns`, `application.name`, and the selected time range. See `docs/METRIC_CARDINALITY_REVIEW.md`.


## QoS / DSCP voice and video

Voice and video are modeled as priority traffic in the POC. The connector derives DSCP from NetFlow ToS and emits `qos.class`, `network.dscp`, and `network.dscp.class` on represented traces, top talkers, community dependencies, and QoS-specific link utilization metrics.

Important metrics:

```text
flow2span.link.qos.bits_per_second
flow2span.link.qos.utilization_percent
flow2span.top_talker.bits_per_second{qos.class="voice"}
flow2span.top_talker.bits_per_second{qos.class="video"}
```

See `docs/QOS_DSCP_VOICE_VIDEO.md`.
