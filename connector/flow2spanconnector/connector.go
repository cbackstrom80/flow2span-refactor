package flow2spanconnector

import (
	"context"
	"crypto/sha1"
	"fmt"
	"math"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type normalizedFlow struct {
	srcIP, dstIP        string
	srcDNS, dstDNS      string
	srcPort, dstPort    int64
	transport           string
	bytes, packets      int64
	flows               int64
	start, end          pcommon.Timestamp
	tcpFlagsOR          int64
	exporter            exporterMatch
	srcSite, dstSite    siteMatch
	client, server      communityMatch
	link                linkMatch
	application         string
	serviceName         string
	direction, path     string
	crossSite           bool
	dscp                int64
	dscpClass           string
	qosClass            string
	ecn                 int64
	tos                 int64
	dedupeDuplicateCnt  int64
	dedupeSuppBytes     int64
	dedupeSuppPackets   int64
	dedupeReason        string
	suppressedExporters []string
}

type agg struct {
	flow                  normalizedFlow
	bytes, packets, flows int64
	minTs, maxTs          pcommon.Timestamp
	tcpFlagsOR            int64
	dedupeDuplicateCnt    int64
	dedupeSuppBytes       int64
	dedupeSuppPackets     int64
	dedupeReason          string
	suppressedExporters   map[string]struct{}
}

type cidrRule struct {
	prefix netip.Prefix
	name   string
}
type siteRule struct {
	prefix                                          netip.Prefix
	name, displayName, region, role, directionClass string
	labels                                          map[string]string
}
type siteMatch struct {
	name, displayName, region, role, directionClass string
	labels                                          map[string]string
}
type communityRule struct {
	prefix                         netip.Prefix
	name, site, region, role, kind string
	services                       []string
	labels                         map[string]string
}
type communityMatch struct {
	name, site, region, role, kind string
	services                       []string
	labels                         map[string]string
}
type exporterRule struct {
	ip, name, role, site, vendor string
	priority, trustLevel         int
}
type exporterMatch struct {
	ip, name, role, site, vendor string
	priority, trustLevel         int
}
type linkRule struct {
	name, site, direction, provider, circuitID, router, iface, exporter string
	speedBps                                                            uint64
	warn, crit                                                          float64
	labels                                                              map[string]string
}
type linkMatch struct {
	name, site, direction, provider, circuitID, router, iface, exporter string
	speedBps                                                            uint64
	warn, crit                                                          float64
	labels                                                              map[string]string
}

type dnsEntry struct {
	name, status string
	expires      time.Time
}

type flow2SpanConnector struct {
	cfg               *Config
	nextTraces        consumer.Traces
	nextMetrics       consumer.Metrics
	mu                sync.Mutex
	buckets           map[int64]map[string]*agg
	serviceRules      []cidrRule
	sites             []siteRule
	clientCommunities []communityRule
	serverCommunities []communityRule
	exporters         map[string]exporterRule
	links             []linkRule
	dnsMu             sync.Mutex
	dnsCache          map[string]dnsEntry
	dnsSem            chan struct{}
}

func newConnector(cfg *Config, nextTraces consumer.Traces, nextMetrics consumer.Metrics) (*flow2SpanConnector, error) {
	c := &flow2SpanConnector{cfg: cfg, nextTraces: nextTraces, nextMetrics: nextMetrics, buckets: map[int64]map[string]*agg{}, exporters: map[string]exporterRule{}, dnsCache: map[string]dnsEntry{}}
	if cfg.DNS.MaxConcurrentLookups <= 0 {
		cfg.DNS.MaxConcurrentLookups = 50
	}
	c.dnsSem = make(chan struct{}, cfg.DNS.MaxConcurrentLookups)
	for _, r := range cfg.ServiceNameRules {
		if p, err := netip.ParsePrefix(r.CIDR); err == nil {
			c.serviceRules = append(c.serviceRules, cidrRule{p, r.Name})
		}
	}
	for _, s := range cfg.Sites {
		for _, raw := range s.CIDRs {
			if p, err := netip.ParsePrefix(raw); err == nil {
				c.sites = append(c.sites, siteRule{p, s.Name, s.DisplayName, s.Region, s.Role, s.DirectionClass, cloneLabels(s.Labels)})
			}
		}
	}
	sort.Slice(c.sites, func(i, j int) bool { return c.sites[i].prefix.Bits() > c.sites[j].prefix.Bits() })
	for _, cm := range cfg.ClientCommunities {
		c.addCommunity(cm, "client")
	}
	for _, cm := range cfg.ServerCommunities {
		c.addCommunity(cm, "server")
	}
	sort.Slice(c.clientCommunities, func(i, j int) bool {
		return c.clientCommunities[i].prefix.Bits() > c.clientCommunities[j].prefix.Bits()
	})
	sort.Slice(c.serverCommunities, func(i, j int) bool {
		return c.serverCommunities[i].prefix.Bits() > c.serverCommunities[j].prefix.Bits()
	})
	for _, e := range cfg.Exporters {
		c.exporters[e.IP] = exporterRule{ip: e.IP, name: e.Name, role: e.Role, site: e.Site, vendor: e.Vendor, priority: e.Priority, trustLevel: e.TrustLevel}
	}
	for _, l := range cfg.Links {
		bps, err := parseSpeedBps(l.Speed)
		if err != nil {
			return nil, fmt.Errorf("invalid link speed for %s: %w", l.Name, err)
		}
		c.links = append(c.links, linkRule{name: l.Name, site: l.Site, direction: l.Direction, provider: l.Provider, circuitID: l.CircuitID, router: l.Router, iface: l.Interface, exporter: l.Exporter, speedBps: bps, warn: l.WarningUtilizationPercent, crit: l.CriticalUtilizationPercent, labels: cloneLabels(l.Labels)})
	}
	return c, nil
}

func (c *flow2SpanConnector) addCommunity(cm CommunityConfig, kind string) {
	for _, raw := range cm.CIDRs {
		if p, err := netip.ParsePrefix(raw); err == nil {
			r := communityRule{prefix: p, name: cm.Name, site: cm.Site, region: cm.Region, role: cm.Role, kind: kind, services: cm.Services, labels: cloneLabels(cm.Labels)}
			if kind == "client" {
				c.clientCommunities = append(c.clientCommunities, r)
			} else {
				c.serverCommunities = append(c.serverCommunities, r)
			}
		}
	}
}
func (c *flow2SpanConnector) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}
func (c *flow2SpanConnector) Start(context.Context, component.Host) error { return nil }
func (c *flow2SpanConnector) Shutdown(context.Context) error              { return nil }
func (c *flow2SpanConnector) ConsumeLogs(ctx context.Context, logs plog.Logs) error {
	c.consume(logs)
	return c.flushReady(ctx, time.Now().Unix())
}

func (c *flow2SpanConnector) consume(logs plog.Logs) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		rl := logs.ResourceLogs().At(i)
		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			for k := 0; k < sl.LogRecords().Len(); k++ {
				lr := sl.LogRecords().At(k)
				attrs := lr.Attributes()
				nf, ok := c.normalize(attrs, lr)
				if !ok {
					continue
				}
				bucketSec := bucketStart(nf.start, c.cfg.Window)
				if _, ok := c.buckets[bucketSec]; !ok {
					c.buckets[bucketSec] = map[string]*agg{}
				}
				key := c.conversationKey(nf, bucketSec)
				existing := c.buckets[bucketSec][key]
				if existing == nil {
					c.buckets[bucketSec][key] = &agg{flow: nf, bytes: nf.bytes, packets: nf.packets, flows: 1, minTs: nf.start, maxTs: nf.end, tcpFlagsOR: nf.tcpFlagsOR, suppressedExporters: map[string]struct{}{}}
					continue
				}
				useNewPrimary, dup := choosePrimary(existing.flow, nf)
				if dup {
					existing.dedupeDuplicateCnt++
					if useNewPrimary {
						if existing.flow.exporter.name != "" {
							existing.suppressedExporters[existing.flow.exporter.name] = struct{}{}
						}
						existing.dedupeSuppBytes += existing.bytes
						existing.dedupeSuppPackets += existing.packets
						existing.flow = nf
						existing.bytes = nf.bytes
						existing.packets = nf.packets
						existing.flows = 1
					} else {
						existing.dedupeSuppBytes += nf.bytes
						existing.dedupeSuppPackets += nf.packets
						if nf.exporter.name != "" {
							existing.suppressedExporters[nf.exporter.name] = struct{}{}
						}
					}
					existing.dedupeReason = "same_conversation_same_window"
					continue
				}
				existing.bytes += nf.bytes
				existing.packets += nf.packets
				existing.flows++
				existing.tcpFlagsOR |= nf.tcpFlagsOR
				if nf.start < existing.minTs {
					existing.minTs = nf.start
				}
				if nf.end > existing.maxTs {
					existing.maxTs = nf.end
				}
			}
		}
	}
	if len(c.buckets) > c.cfg.MaxBucketsInMemory {
		keys := make([]int64, 0, len(c.buckets))
		for k := range c.buckets {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		for _, k := range keys[:len(keys)-c.cfg.MaxBucketsInMemory] {
			delete(c.buckets, k)
		}
	}
}

func (c *flow2SpanConnector) normalize(attrs pcommon.Map, lr plog.LogRecord) (normalizedFlow, bool) {
	src := getString(attrs, c.cfg.SourceAddressKey)
	dst := getString(attrs, c.cfg.DestinationAddressKey)
	dstPort := getInt(attrs, c.cfg.DestinationPortKey)
	srcPort := getInt(attrs, c.cfg.SourcePortKey)
	transport := strings.ToLower(getString(attrs, c.cfg.TransportKey))
	if src == "" || dst == "" || dstPort == 0 || transport == "" {
		return normalizedFlow{}, false
	}
	start := getTimestamp(attrs, c.cfg.StartKey)
	end := getTimestamp(attrs, c.cfg.EndKey)
	fallback := lr.Timestamp()
	if fallback == 0 {
		fallback = lr.ObservedTimestamp()
	}
	if fallback == 0 {
		fallback = pcommon.NewTimestampFromTime(time.Now())
	}
	if start == 0 {
		start = fallback
	}
	if end == 0 {
		end = fallback
	}
	exp := c.matchExporter(getString(attrs, c.cfg.ExporterIPKey))
	srcSite := c.matchSite(src)
	dstSite := c.matchSite(dst)
	client := c.matchCommunity(src, c.clientCommunities)
	server := c.matchCommunity(dst, c.serverCommunities)
	direction := classifyDirection(srcSite, dstSite)
	link := c.matchLink(srcSite, dstSite, exp)
	srcDNS := c.lookupDNS(src)
	dstDNS := c.lookupDNS(dst)
	app := c.applicationName(dst, dstPort, transport, dstDNS)
	service := c.serviceName(dst, dstSite, server, app)
	tos, dscp, ecn, dscpClass, qosClass := c.extractQoS(attrs)
	return normalizedFlow{srcIP: src, dstIP: dst, srcDNS: srcDNS, dstDNS: dstDNS, srcPort: srcPort, dstPort: dstPort, transport: transport, bytes: getInt(attrs, c.cfg.BytesKey), packets: getInt(attrs, c.cfg.PacketsKey), flows: 1, start: start, end: end, tcpFlagsOR: getInt(attrs, c.cfg.TCPFlagsKey), exporter: exp, srcSite: srcSite, dstSite: dstSite, client: client, server: server, link: link, application: app, serviceName: service, direction: direction, path: fmt.Sprintf("%s->%s", srcSite.name, dstSite.name), crossSite: srcSite.name != "" && dstSite.name != "" && srcSite.name != dstSite.name, tos: tos, dscp: dscp, ecn: ecn, dscpClass: dscpClass, qosClass: qosClass}, true
}

func (c *flow2SpanConnector) flushReady(ctx context.Context, nowSec int64) error {
	cutoff := nowSec - int64(c.cfg.FlushLag.Seconds())
	ready := map[int64]map[string]*agg{}
	c.mu.Lock()
	for k, v := range c.buckets {
		if k <= cutoff {
			ready[k] = v
			delete(c.buckets, k)
		}
	}
	c.mu.Unlock()
	for bucketSec, convs := range ready {
		if c.nextTraces != nil {
			tr := c.buildTraces(bucketSec, convs)
			if tr.SpanCount() > 0 {
				if err := c.nextTraces.ConsumeTraces(ctx, tr); err != nil {
					return err
				}
			}
		}
		if c.nextMetrics != nil {
			md := c.buildMetrics(bucketSec, convs)
			if md.DataPointCount() > 0 {
				if err := c.nextMetrics.ConsumeMetrics(ctx, md); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (c *flow2SpanConnector) buildTraces(bucketSec int64, convs map[string]*agg) ptrace.Traces {
	if c.cfg.ServiceMap.Enabled {
		return c.buildServiceMapTraces(bucketSec, convs)
	}
	tr := ptrace.NewTraces()
	items := rankAggs(convs, c.cfg.ConversationScoreKey)
	items = topPercent(items, c.cfg.ConversationPercent)
	if len(items) > c.cfg.MaxSpansPerFlush {
		items = items[:c.cfg.MaxSpansPerFlush]
	}
	byService := map[string]ptrace.ScopeSpans{}
	for idx, it := range items {
		nf := it.flow
		nf.bytes = it.bytes
		nf.packets = it.packets
		nf.flows = it.flows
		traceID := stableTraceID(it.key, bucketSec)
		rootID := stableSpanID("community", it.key, bucketSec)
		start := it.minTs
		end := it.maxTs
		if end <= start {
			end = start + pcommon.Timestamp(time.Millisecond)
		}
		ss := scopeSpansForService(tr, byService, nf.serviceName, c.cfg, nf.dstSite)
		sp := ss.Spans().AppendEmpty()
		sp.SetTraceID(traceID)
		sp.SetSpanID(rootID)
		sp.SetName(fmt.Sprintf("%s -> %s / %s", nz(nf.client.name, nf.srcSite.name), nz(nf.server.name, nf.dstSite.name), nz(nf.application, nf.transport)))
		sp.SetKind(ptrace.SpanKindInternal)
		sp.SetStartTimestamp(start)
		sp.SetEndTimestamp(end)
		fillCommon(sp.Attributes(), c.cfg, nf, it.agg)
		if c.cfg.TopTalkers.Enabled && c.cfg.TopTalkers.EmitTraces && idx < c.cfg.TopTalkers.Limit {
			sp.Attributes().PutInt("top_talker.rank", int64(idx+1))
			sp.Attributes().PutStr("top_talker.score_key", c.cfg.TopTalkers.RankBy)
		}
	}
	return tr
}

func (c *flow2SpanConnector) buildServiceMapTraces(bucketSec int64, convs map[string]*agg) ptrace.Traces {
	tr := ptrace.NewTraces()
	items := rankAggs(convs, c.cfg.ConversationScoreKey)
	items = topPercent(items, c.cfg.ConversationPercent)
	if len(items) > c.cfg.MaxSpansPerFlush {
		items = items[:c.cfg.MaxSpansPerFlush]
	}
	byService := map[string]ptrace.ScopeSpans{}
	for idx, it := range items {
		nf := it.flow
		nf.bytes = it.bytes
		nf.packets = it.packets
		nf.flows = it.flows

		traceID := stableTraceID(it.key, bucketSec)
		clientSpanID := stableSpanID("client-site", it.key, bucketSec)
		serverSpanID := stableSpanID("server-site", it.key, bucketSec)
		appSpanID := stableSpanID("application", it.key, bucketSec)
		start := it.minTs
		end := it.maxTs
		if end <= start {
			end = start + pcommon.Timestamp(time.Millisecond)
		}

		clientNode := c.serviceMapNodeName(nf, "client")
		serverNode := c.serviceMapNodeName(nf, "server")
		if clientNode == serverNode {
			serverNode = serverNode + " / " + nz(nf.server.name, nf.serviceName)
		}

		clientSS := scopeSpansForServiceMapNode(tr, byService, clientNode, c.cfg, nf.srcSite)
		clientSpan := clientSS.Spans().AppendEmpty()
		clientSpan.SetTraceID(traceID)
		clientSpan.SetSpanID(clientSpanID)
		clientSpan.SetName(c.serviceMapSpanName(nf, clientNode, serverNode))
		clientSpan.SetKind(ptrace.SpanKindClient)
		clientSpan.SetStartTimestamp(start)
		clientSpan.SetEndTimestamp(end)
		fillCommon(clientSpan.Attributes(), c.cfg, nf, it.agg)
		clientSpan.Attributes().PutStr("service_map.node.role", "client_site")
		clientSpan.Attributes().PutStr("service_map.client.node", clientNode)
		clientSpan.Attributes().PutStr("service_map.server.node", serverNode)
		if c.cfg.ServiceMap.EmitPeerServiceLinks {
			applyServiceMapPeerAttrs(clientSpan.Attributes(), nf, clientNode, serverNode)
		}

		serverSS := scopeSpansForServiceMapNode(tr, byService, serverNode, c.cfg, nf.dstSite)
		serverSpan := serverSS.Spans().AppendEmpty()
		serverSpan.SetTraceID(traceID)
		serverSpan.SetSpanID(serverSpanID)
		serverSpan.SetParentSpanID(clientSpanID)
		serverSpan.SetName(c.serviceMapServerSpanName(nf, clientNode, serverNode))
		serverSpan.SetKind(ptrace.SpanKindServer)
		serverSpan.SetStartTimestamp(start)
		serverSpan.SetEndTimestamp(end)
		fillCommon(serverSpan.Attributes(), c.cfg, nf, it.agg)
		serverSpan.Attributes().PutStr("service_map.node.role", "server_site")
		serverSpan.Attributes().PutStr("service_map.client.node", clientNode)
		serverSpan.Attributes().PutStr("service_map.server.node", serverNode)
		serverSpan.Attributes().PutStr("client.address", clientNode)
		serverSpan.Attributes().PutStr("server.address", serverNode)
		serverSpan.Attributes().PutStr("net.host.name", serverNode)

		if c.cfg.ServiceMap.IncludeApplicationSpan {
			appNode := nz(nf.application, nf.serviceName)
			appSS := scopeSpansForServiceMapNode(tr, byService, appNode, c.cfg, nf.dstSite)
			appSpan := appSS.Spans().AppendEmpty()
			appSpan.SetTraceID(traceID)
			appSpan.SetSpanID(appSpanID)
			appSpan.SetParentSpanID(serverSpanID)
			appSpan.SetName("application " + appNode)
			appSpan.SetKind(ptrace.SpanKindInternal)
			appSpan.SetStartTimestamp(start)
			appSpan.SetEndTimestamp(end)
			fillCommon(appSpan.Attributes(), c.cfg, nf, it.agg)
			appSpan.Attributes().PutStr("service_map.node.role", "application")
		}

		if c.cfg.TopTalkers.Enabled && c.cfg.TopTalkers.EmitTraces && idx < c.cfg.TopTalkers.Limit {
			clientSpan.Attributes().PutInt("top_talker.rank", int64(idx+1))
			clientSpan.Attributes().PutStr("top_talker.score_key", c.cfg.TopTalkers.RankBy)
			serverSpan.Attributes().PutInt("top_talker.rank", int64(idx+1))
			serverSpan.Attributes().PutStr("top_talker.score_key", c.cfg.TopTalkers.RankBy)
		}
	}
	return tr
}

func (c *flow2SpanConnector) buildMetrics(bucketSec int64, convs map[string]*agg) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	ra := rm.Resource().Attributes()
	ra.PutStr("service.name", "flow2spanconnector")
	if c.cfg.EnvironmentKey != "" && c.cfg.Environment != "" {
		ra.PutStr(c.cfg.EnvironmentKey, c.cfg.Environment)
	}
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("flow2spanconnector")
	for _, it := range convs {
		nf := it.flow
		nf.bytes = it.bytes
		nf.packets = it.packets
		nf.flows = it.flows
		duration := math.Max(c.cfg.Window.Seconds(), 1)
		bps := float64(it.bytes*8) / duration
		util := 0.0
		if nf.link.speedBps > 0 {
			util = (bps / float64(nf.link.speedBps)) * 100
		}
		addGauge(sm, "flow2span.site.bits_per_second", bps, bucketSec, attrsSite(nf, "egress"))
		addSum(sm, "flow2span.site.bytes", it.bytes, bucketSec, attrsSite(nf, "egress"))
		addSum(sm, "flow2span.site.packets", it.packets, bucketSec, attrsSite(nf, "egress"))
		addGauge(sm, "flow2span.link.bits_per_second", bps, bucketSec, attrsLink(nf))
		addGauge(sm, "flow2span.link.utilization_percent", util, bucketSec, attrsLink(nf))
		if c.cfg.QoS.Enabled && c.cfg.QoS.EmitOnLinkQoSMetrics && nf.qosClass != "" {
			addGauge(sm, "flow2span.link.qos.bits_per_second", bps, bucketSec, attrsLinkQoS(nf))
			addGauge(sm, "flow2span.link.qos.utilization_percent", util, bucketSec, attrsLinkQoS(nf))
		}
		addGauge(sm, "flow2span.community_dependency.bits_per_second", bps, bucketSec, attrsDependency(nf))
		addSum(sm, "flow2span.community_dependency.bytes", it.bytes, bucketSec, attrsDependency(nf))
		addSum(sm, "flow2span.community_dependency.conversations", it.flows, bucketSec, attrsDependency(nf))
		if it.dedupeDuplicateCnt > 0 {
			a := attrsDedupe(nf, it)
			addSum(sm, "flow2span.dedup.duplicate_flows", it.dedupeDuplicateCnt, bucketSec, a)
			addSum(sm, "flow2span.dedup.duplicate_bytes_suppressed", it.dedupeSuppBytes, bucketSec, a)
			addSum(sm, "flow2span.dedup.duplicate_packets_suppressed", it.dedupeSuppPackets, bucketSec, a)
		}
	}
	if c.cfg.TopTalkers.Enabled && c.cfg.TopTalkers.EmitMetrics {
		c.addTopTalkerMetrics(sm, bucketSec, convs)
	}
	return md
}

func (c *flow2SpanConnector) addTopTalkerMetrics(sm pmetric.ScopeMetrics, bucketSec int64, convs map[string]*agg) {
	items := rankAggs(convs, c.cfg.TopTalkers.RankBy)
	limit := c.cfg.TopTalkers.Limit
	if limit <= 0 {
		limit = 20
	}
	if len(items) > limit {
		items = items[:limit]
	}
	for i, it := range items {
		nf := it.flow
		nf.bytes = it.bytes
		nf.packets = it.packets
		nf.flows = it.flows
		duration := math.Max(c.cfg.Window.Seconds(), 1)
		bps := float64(it.bytes*8) / duration
		util := 0.0
		if nf.link.speedBps > 0 {
			util = (bps / float64(nf.link.speedBps)) * 100
		}
		attrs := attrsTopTalker(nf, i+1, c.cfg.TopTalkers.RankBy)
		traceID := stableTraceID(it.key, bucketSec)
		spanID := stableSpanID("community", it.key, bucketSec)
		if c.cfg.TopTalkers.IncludeTraceIDAttribute && !c.cfg.TopTalkers.DropTraceIDDimensionMetrics {
			attrs["trace.id"] = traceID.String()
		}
		if c.cfg.TopTalkers.IncludeSpanIDAttribute {
			attrs["span.id"] = spanID.String()
		}
		addGauge(sm, "flow2span.top_talker.bits_per_second", bps, bucketSec, attrs)
		addSum(sm, "flow2span.top_talker.bytes", it.bytes, bucketSec, attrs)
		addSum(sm, "flow2span.top_talker.packets", it.packets, bucketSec, attrs)
		addGauge(sm, "flow2span.top_talker.utilization_percent", util, bucketSec, attrs)
	}
}

func addGauge(sm pmetric.ScopeMetrics, name string, value float64, bucketSec int64, attrs map[string]string) {
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	dp := m.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Unix(bucketSec, 0)))
	dp.SetDoubleValue(value)
	putAttrs(dp.Attributes(), attrs)
}
func addSum(sm pmetric.ScopeMetrics, name string, value int64, bucketSec int64, attrs map[string]string) {
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	sum := m.SetEmptySum()
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	sum.SetIsMonotonic(false)
	dp := sum.DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Unix(bucketSec, 0)))
	dp.SetIntValue(value)
	putAttrs(dp.Attributes(), attrs)
}
func putAttrs(m pcommon.Map, attrs map[string]string) {
	for k, v := range attrs {
		if v != "" {
			m.PutStr(k, v)
		}
	}
}

func scopeSpansForService(tr ptrace.Traces, cache map[string]ptrace.ScopeSpans, service string, cfg *Config, site siteMatch) ptrace.ScopeSpans {
	key := service + "|" + site.name
	if ss, ok := cache[key]; ok {
		return ss
	}
	rl := tr.ResourceSpans().AppendEmpty()
	ra := rl.Resource().Attributes()
	ra.PutStr("service.name", service)
	if cfg.ServiceMap.ServiceNamespace != "" {
		ra.PutStr("service.namespace", cfg.ServiceMap.ServiceNamespace)
	}
	if cfg.EnvironmentKey != "" && cfg.Environment != "" {
		ra.PutStr(cfg.EnvironmentKey, cfg.Environment)
	}
	ra.PutStr("telemetry.source", "flow2spanconnector")
	applySiteResourceAttrs(ra, site)
	ss := rl.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("flow2spanconnector")
	ss.Scope().SetVersion("0.3.0-poc")
	cache[key] = ss
	return ss
}
func scopeSpansForServiceMapNode(tr ptrace.Traces, cache map[string]ptrace.ScopeSpans, service string, cfg *Config, site siteMatch) ptrace.ScopeSpans {
	if service == "" {
		service = "unknown-network-node"
	}
	key := service + "|" + site.name
	if ss, ok := cache[key]; ok {
		return ss
	}
	rl := tr.ResourceSpans().AppendEmpty()
	ra := rl.Resource().Attributes()
	ra.PutStr("service.name", service)
	if cfg.ServiceMap.ServiceNamespace != "" {
		ra.PutStr("service.namespace", cfg.ServiceMap.ServiceNamespace)
	}
	if cfg.EnvironmentKey != "" && cfg.Environment != "" {
		ra.PutStr(cfg.EnvironmentKey, cfg.Environment)
	}
	ra.PutStr("telemetry.source", "flow2spanconnector")
	ra.PutStr("flow2span.service_map.mode", "site_dependency")
	applySiteResourceAttrs(ra, site)
	ss := rl.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("flow2spanconnector")
	ss.Scope().SetVersion("0.4.0-poc-service-map")
	cache[key] = ss
	return ss
}

func (c *flow2SpanConnector) serviceMapNodeName(nf normalizedFlow, side string) string {
	nodeType := strings.ToLower(strings.TrimSpace(c.cfg.ServiceMap.NodeType))
	if nodeType == "" {
		nodeType = "site_display"
	}
	if side == "client" {
		switch nodeType {
		case "endpoint", "endpoint_dns":
			return endpointDisplayName(nf.srcDNS, nf.srcIP)
		case "endpoint_ip":
			return nf.srcIP
		case "community":
			return nz(nf.client.name, nz(nf.srcSite.displayName, nf.srcSite.name))
		case "region":
			return nz(nf.srcSite.region, nf.srcSite.name)
		case "site_name":
			return nz(nf.srcSite.name, nf.client.name)
		case "service":
			return nz(nf.client.name, nf.srcSite.name)
		default:
			return nz(nf.srcSite.displayName, nz(nf.srcSite.name, nf.client.name))
		}
	}
	switch nodeType {
	case "endpoint", "endpoint_dns":
		return endpointDisplayName(nf.dstDNS, nf.dstIP)
	case "endpoint_ip":
		return nf.dstIP
	case "community":
		return nz(nf.server.name, nz(nf.dstSite.displayName, nf.dstSite.name))
	case "region":
		return nz(nf.dstSite.region, nf.dstSite.name)
	case "site_name":
		return nz(nf.dstSite.name, nf.server.name)
	case "service":
		return nz(nf.serviceName, nf.server.name)
	default:
		return nz(nf.dstSite.displayName, nz(nf.dstSite.name, nf.server.name))
	}
}

func endpointDisplayName(dnsName, ip string) string {
	dnsName = strings.TrimSpace(strings.TrimSuffix(dnsName, "."))
	if dnsName != "" && dnsName != "unresolved" {
		return dnsName
	}
	return ip
}

func (c *flow2SpanConnector) serviceMapSpanName(nf normalizedFlow, clientNode, serverNode string) string {
	base := fmt.Sprintf("%s -> %s", clientNode, serverNode)
	if app := nz(nf.application, nf.serviceName); app != "" {
		return base + " / " + app
	}
	return base
}

func (c *flow2SpanConnector) serviceMapServerSpanName(nf normalizedFlow, clientNode, serverNode string) string {
	return fmt.Sprintf("receive %s from %s", nz(nf.application, nf.transport), clientNode)
}

func applyServiceMapPeerAttrs(attrs pcommon.Map, nf normalizedFlow, clientNode, serverNode string) {
	// These attributes help Splunk APM infer edges between synthetic site services.
	// The resource service.name is the client-side site/community, while peer.service
	// points at the destination site/community node.
	attrs.PutStr("peer.service", serverNode)
	attrs.PutStr("server.address", serverNode)
	attrs.PutInt("server.port", nf.dstPort)
	attrs.PutStr("net.peer.name", serverNode)
	attrs.PutStr("net.peer.ip", nf.dstIP)
	attrs.PutStr("rpc.system", "flow2span")
	attrs.PutStr("rpc.service", nz(nf.application, nf.serviceName))
	attrs.PutStr("span.kind.synthetic", "client_to_server_site_dependency")
}

func fillCommon(attrs pcommon.Map, cfg *Config, nf normalizedFlow, a *agg) {
	attrs.PutStr("flow2span.logical_service.name", nf.serviceName)
	attrs.PutStr("network.transport", nf.transport)
	if cfg.QoS.Enabled && cfg.QoS.EmitOnTraces && nf.dscpClass != "" {
		attrs.PutInt("network.tos", nf.tos)
		attrs.PutInt("network.dscp", nf.dscp)
		attrs.PutStr("network.dscp.class", nf.dscpClass)
		attrs.PutStr("qos.class", nf.qosClass)
		attrs.PutInt("network.ecn", nf.ecn)
	}
	attrs.PutStr("source.address", nf.srcIP)
	attrs.PutStr("destination.address", nf.dstIP)
	attrs.PutStr("source.endpoint.name", endpointDisplayName(nf.srcDNS, nf.srcIP))
	attrs.PutStr("destination.endpoint.name", endpointDisplayName(nf.dstDNS, nf.dstIP))
	attrs.PutInt("source.port", nf.srcPort)
	attrs.PutInt("destination.port", nf.dstPort)
	attrs.PutStr("flow.direction", nf.direction)
	attrs.PutStr("flow.path", nf.path)
	attrs.PutBool("flow.cross_site", nf.crossSite)
	attrs.PutInt("flow.io.bytes", a.bytes)
	attrs.PutInt("flow.io.packets", a.packets)
	attrs.PutInt("flow.count", a.flows)
	attrs.PutStr("application.name", nf.application)
	attrs.PutStr("client.community", nf.client.name)
	attrs.PutStr("server.community", nf.server.name)
	attrs.PutStr("client.site", nf.srcSite.name)
	attrs.PutStr("server.site", nf.dstSite.name)
	attrs.PutStr("link.name", nf.link.name)
	attrs.PutInt("link.speed_bps", int64(nf.link.speedBps))
	if cfg.DNS.EmitOnRepresentedTraces {
		attrs.PutStr("flow.src.dns", nf.srcDNS)
		attrs.PutStr("flow.dst.dns", nf.dstDNS)
	}
	if a.dedupeDuplicateCnt > 0 {
		attrs.PutBool("dedupe.enabled", true)
		attrs.PutInt("dedupe.duplicate_count", a.dedupeDuplicateCnt)
		attrs.PutInt("dedupe.bytes_suppressed", a.dedupeSuppBytes)
		attrs.PutStr("dedupe.reason", a.dedupeReason)
		attrs.PutStr("dedupe.primary_exporter", nf.exporter.name)
	}
	applyEndpointAttrs(attrs, "source", nf.srcSite)
	applyEndpointAttrs(attrs, "destination", nf.dstSite)
}

func (c *flow2SpanConnector) extractQoS(attrs pcommon.Map) (int64, int64, int64, string, string) {
	if !c.cfg.QoS.Enabled {
		return 0, 0, 0, "", ""
	}
	keys := []string{}
	if c.cfg.QoS.SourceAttribute != "" {
		keys = append(keys, c.cfg.QoS.SourceAttribute)
	}
	keys = append(keys, c.cfg.QoS.FallbackAttributes...)
	var raw int64
	var found bool
	var sourceKey string
	for _, k := range keys {
		if k == "" {
			continue
		}
		if v, ok := getOptionalInt(attrs, k); ok {
			raw = v
			found = true
			sourceKey = k
			break
		}
	}
	if !found {
		return 0, 0, 0, "", ""
	}
	// NetFlow v5 carries the IPv4 ToS byte. DSCP is upper 6 bits and ECN is lower 2 bits.
	// If a receiver already emits a dscp-like attribute, use it as DSCP directly.
	isDSCPAttr := strings.Contains(strings.ToLower(sourceKey), "dscp")
	var tos, dscp, ecn int64
	if isDSCPAttr && !c.cfg.QoS.DeriveDSCPFromToS {
		dscp = raw
		tos = dscp << 2
		ecn = 0
	} else if isDSCPAttr && raw <= 63 {
		dscp = raw
		tos = dscp << 2
		ecn = 0
	} else {
		tos = raw
		dscp = (tos >> 2) & 0x3f
		ecn = tos & 0x03
	}
	class := dscpClassName(dscp)
	qos := qosMediaClass(dscp)
	return tos, dscp, ecn, class, qos
}

func dscpClassName(dscp int64) string {
	switch dscp {
	case 0:
		return "CS0"
	case 8:
		return "CS1"
	case 10:
		return "AF11"
	case 12:
		return "AF12"
	case 14:
		return "AF13"
	case 16:
		return "CS2"
	case 18:
		return "AF21"
	case 20:
		return "AF22"
	case 22:
		return "AF23"
	case 24:
		return "CS3"
	case 26:
		return "AF31"
	case 28:
		return "AF32"
	case 30:
		return "AF33"
	case 32:
		return "CS4"
	case 34:
		return "AF41"
	case 36:
		return "AF42"
	case 38:
		return "AF43"
	case 40:
		return "CS5"
	case 46:
		return "EF"
	case 48:
		return "CS6"
	case 56:
		return "CS7"
	default:
		return fmt.Sprintf("DSCP_%d", dscp)
	}
}

func qosMediaClass(dscp int64) string {
	switch dscp {
	case 46:
		return "voice"
	case 40, 24:
		return "voice_signaling"
	case 32, 34, 36, 38:
		return "video"
	case 26, 28, 30:
		return "business_critical"
	case 0:
		return "best_effort"
	case 8:
		return "scavenger"
	default:
		return "other"
	}
}

func (c *flow2SpanConnector) conversationKey(nf normalizedFlow, bucket int64) string {
	if c.cfg.Deduplication.Enabled && c.cfg.Deduplication.Bidirectional {
		clientIP, serverIP, clientPort, serverPort := canonicalEndpoint(nf.srcIP, nf.dstIP, nf.srcPort, nf.dstPort)
		return fmt.Sprintf("%s:%d->%s:%d/%s|%s|%d", clientIP, clientPort, serverIP, serverPort, nf.transport, nf.application, bucket)
	}
	return fmt.Sprintf("%s:%d->%s:%d/%s|%s|%d", nf.srcIP, nf.srcPort, nf.dstIP, nf.dstPort, nf.transport, nf.application, bucket)
}
func canonicalEndpoint(src, dst string, sp, dp int64) (string, string, int64, int64) {
	if isWellKnown(dp) && !isWellKnown(sp) {
		return src, dst, sp, dp
	}
	if isWellKnown(sp) && !isWellKnown(dp) {
		return dst, src, dp, sp
	}
	if src < dst {
		return src, dst, sp, dp
	}
	return dst, src, dp, sp
}
func isWellKnown(p int64) bool { return p > 0 && p < 1024 || p == 8080 || p == 8443 || p == 3389 }
func choosePrimary(a, b normalizedFlow) (bool, bool) {
	if a.exporter.name == "" || b.exporter.name == "" {
		return false, false
	}
	if a.exporter.name == b.exporter.name {
		return false, false
	}
	if a.exporter.priority == b.exporter.priority {
		return false, true
	}
	if b.exporter.priority > a.exporter.priority {
		return true, true
	}
	return false, true
}

func (c *flow2SpanConnector) matchSite(ip string) siteMatch {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return siteMatch{name: "unknown", directionClass: "unknown", labels: map[string]string{}}
	}
	for _, s := range c.sites {
		if s.prefix.Contains(addr) {
			return siteMatch{s.name, s.displayName, s.region, s.role, s.directionClass, s.labels}
		}
	}
	return siteMatch{name: "external", directionClass: "external", labels: map[string]string{}}
}
func (c *flow2SpanConnector) matchCommunity(ip string, rules []communityRule) communityMatch {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return communityMatch{name: "unknown"}
	}
	for _, r := range rules {
		if r.prefix.Contains(addr) {
			return communityMatch{name: r.name, site: r.site, region: r.region, role: r.role, kind: r.kind, services: r.services, labels: r.labels}
		}
	}
	return communityMatch{name: "unknown"}
}
func (c *flow2SpanConnector) matchExporter(ip string) exporterMatch {
	if r, ok := c.exporters[ip]; ok {
		return exporterMatch(r)
	}
	return exporterMatch{ip: ip, name: "unknown", role: "unknown", priority: 10}
}
func (c *flow2SpanConnector) matchLink(src, dst siteMatch, exp exporterMatch) linkMatch {
	for _, l := range c.links {
		if l.exporter != "" && l.exporter == exp.name {
			return linkMatch(l)
		}
	}
	for _, l := range c.links {
		if l.site == exp.site && l.direction != "" {
			return linkMatch(l)
		}
	}
	for _, l := range c.links {
		if l.site == src.name {
			return linkMatch(l)
		}
	}
	return linkMatch{name: "unknown"}
}
func (c *flow2SpanConnector) applicationName(dst string, port int64, transport, dns string) string {
	for _, r := range c.cfg.Applications {
		if len(r.DstPorts) > 0 && !containsInt(r.DstPorts, int(port)) {
			continue
		}
		if len(r.Protocols) > 0 && !containsStrFold(r.Protocols, transport) {
			continue
		}
		if matchDNSRule(dns, r) {
			return r.Name
		}
		if len(r.DNSContains) == 0 && len(r.DNSSuffixes) == 0 {
			return r.Name
		}
	}
	switch port {
	case 53:
		return "dns"
	case 80:
		return "http"
	case 443:
		return "https"
	case 22:
		return "ssh"
	default:
		return transport + "/" + strconv.FormatInt(port, 10)
	}
}
func (c *flow2SpanConnector) serviceName(ip string, site siteMatch, server communityMatch, app string) string {
	if server.name != "" && server.name != "unknown" {
		return server.name
	}
	if c.cfg.UseServiceNameRules {
		if addr, err := netip.ParseAddr(ip); err == nil {
			for _, r := range c.serviceRules {
				if r.prefix.Contains(addr) {
					return r.name
				}
			}
		}
	}
	if site.name != "" {
		return site.name
	}
	return app
}
func classifyDirection(src, dst siteMatch) string {
	if src.directionClass == "external" || dst.directionClass == "external" {
		return "north_south"
	}
	if src.name == "unknown" || dst.name == "unknown" {
		return "unknown"
	}
	if src.name == dst.name {
		return "east_west"
	}
	return "east_west_cross_site"
}

func (c *flow2SpanConnector) lookupDNS(ip string) string {
	if !c.cfg.DNS.Enabled {
		return ""
	}
	addr := net.ParseIP(ip)
	if addr == nil {
		return ""
	}
	private := addr.IsPrivate()
	if private && !c.cfg.DNS.LookupPrivateIPs {
		return ""
	}
	if !private && !c.cfg.DNS.LookupPublicIPs {
		return ""
	}
	now := time.Now()
	c.dnsMu.Lock()
	if e, ok := c.dnsCache[ip]; ok && now.Before(e.expires) {
		c.dnsMu.Unlock()
		return e.name
	}
	c.dnsMu.Unlock()
	if c.cfg.DNS.BlockOnLookup {
		return c.resolveDNS(ip)
	}
	select {
	case c.dnsSem <- struct{}{}:
		go func() { defer func() { <-c.dnsSem }(); c.resolveDNS(ip) }()
	default:
	}
	if c.cfg.DNS.IncludeUnresolved {
		return "unresolved"
	}
	return ""
}
func (c *flow2SpanConnector) resolveDNS(ip string) string {
	timeout := c.cfg.DNS.Timeout
	if timeout <= 0 {
		timeout = 250 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(ctx, ip)
	status := "resolved"
	name := ""
	ttl := c.cfg.DNS.CacheTTL
	if err != nil || len(names) == 0 {
		status = "unresolved"
		if ttl = c.cfg.DNS.NegativeCacheTTL; ttl <= 0 {
			ttl = 10 * time.Minute
		}
		name = "unresolved"
	} else {
		name = names[0]
		if c.cfg.DNS.StripTrailingDot {
			name = strings.TrimSuffix(name, ".")
		}
		if c.cfg.DNS.SanitizeNames {
			name = strings.ToLower(strings.TrimSpace(name))
		}
		if ttl <= 0 {
			ttl = 24 * time.Hour
		}
	}
	c.dnsMu.Lock()
	c.dnsCache[ip] = dnsEntry{name: name, status: status, expires: time.Now().Add(ttl)}
	c.dnsMu.Unlock()
	return name
}

func addQoSMetricAttrs(m map[string]string, nf normalizedFlow) {
	if nf.dscpClass == "" {
		return
	}
	m["network.dscp"] = strconv.FormatInt(nf.dscp, 10)
	m["network.dscp.class"] = nf.dscpClass
	m["qos.class"] = nf.qosClass
}

func attrsSite(nf normalizedFlow, dir string) map[string]string {
	return map[string]string{"site.name": nf.srcSite.name, "site.region": nf.srcSite.region, "site.role": nf.srcSite.role, "traffic.direction": dir, "application.name": nf.application}
}
func attrsLink(nf normalizedFlow) map[string]string {
	return map[string]string{"site.name": nf.link.site, "link.name": nf.link.name, "link.provider": nf.link.provider, "link.circuit_id": nf.link.circuitID, "link.interface": nf.link.iface, "link.direction": nf.link.direction, "application.name": nf.application, "client.community": nf.client.name, "server.community": nf.server.name}
}
func attrsDependency(nf normalizedFlow) map[string]string {
	m := map[string]string{"client.community": nf.client.name, "server.community": nf.server.name, "client.site": nf.srcSite.name, "server.site": nf.dstSite.name, "application.name": nf.application, "service.name": nf.serviceName, "link.name": nf.link.name, "traffic.class": nf.direction}
	addQoSMetricAttrs(m, nf)
	return m
}

func attrsLinkQoS(nf normalizedFlow) map[string]string {
	m := attrsLink(nf)
	addQoSMetricAttrs(m, nf)
	return m
}
func attrsTopTalker(nf normalizedFlow, rank int, key string) map[string]string {
	m := attrsDependency(nf)
	m["top_talker.rank"] = strconv.Itoa(rank)
	m["top_talker.score_key"] = key
	m["flow.src.ip"] = nf.srcIP
	m["flow.dst.ip"] = nf.dstIP
	m["flow.dst.port"] = strconv.FormatInt(nf.dstPort, 10)
	m["network.transport"] = nf.transport
	if nf.srcDNS != "" {
		m["flow.src.dns"] = nf.srcDNS
	}
	if nf.dstDNS != "" {
		m["flow.dst.dns"] = nf.dstDNS
	}
	return m
}
func attrsDedupe(nf normalizedFlow, a *agg) map[string]string {
	return map[string]string{"site.name": nf.srcSite.name, "exporter.primary": nf.exporter.name, "exporter.role.primary": nf.exporter.role, "dedupe.strategy": nf.exporter.role, "dedupe.reason": a.dedupeReason}
}

type rankedAgg struct {
	key string
	*agg
}

func rankAggs(convs map[string]*agg, key string) []rankedAgg {
	out := make([]rankedAgg, 0, len(convs))
	for k, a := range convs {
		out = append(out, rankedAgg{k, a})
	}
	sort.Slice(out, func(i, j int) bool { return scoreValue(key, out[i].agg) > scoreValue(key, out[j].agg) })
	return out
}
func scoreValue(kind string, a *agg) int64 {
	switch strings.ToLower(kind) {
	case "packets":
		return a.packets
	case "flows", "conversations":
		return a.flows
	default:
		return a.bytes
	}
}
func topPercent[T any](items []T, percent float64) []T {
	if len(items) == 0 || percent <= 0 || percent >= 100 {
		return items
	}
	keep := int(math.Ceil(float64(len(items)) * percent / 100))
	if keep < 1 {
		keep = 1
	}
	if keep > len(items) {
		keep = len(items)
	}
	return items[:keep]
}
func parseSpeedBps(s string) (uint64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	mult := float64(1)
	switch {
	case strings.HasSuffix(s, "gbps"):
		mult = 1e9
		s = strings.TrimSuffix(s, "gbps")
	case strings.HasSuffix(s, "mbps"):
		mult = 1e6
		s = strings.TrimSuffix(s, "mbps")
	case strings.HasSuffix(s, "kbps"):
		mult = 1e3
		s = strings.TrimSuffix(s, "kbps")
	case strings.HasSuffix(s, "bps"):
		mult = 1
		s = strings.TrimSuffix(s, "bps")
	default:
		return 0, fmt.Errorf("expected speed suffix kbps/mbps/gbps/bps")
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, err
	}
	return uint64(v * mult), nil
}
func bucketStart(ts pcommon.Timestamp, window time.Duration) int64 {
	sec := int64(ts) / int64(time.Second)
	w := int64(window / time.Second)
	if w <= 0 {
		w = 1
	}
	return sec - (sec % w)
}
func stableTraceID(key string, bucket int64) pcommon.TraceID {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d", key, bucket)))
	var out pcommon.TraceID
	copy(out[:], sum[:16])
	return out
}
func stableSpanID(prefix, key string, bucket int64) pcommon.SpanID {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%s|%d", prefix, key, bucket)))
	var out pcommon.SpanID
	copy(out[:], sum[:8])
	return out
}
func getString(m pcommon.Map, key string) string {
	if v, ok := m.Get(key); ok {
		return v.AsString()
	}
	return ""
}
func getOptionalInt(m pcommon.Map, key string) (int64, bool) {
	v, ok := m.Get(key)
	if !ok {
		return 0, false
	}
	switch v.Type() {
	case pcommon.ValueTypeInt:
		return v.Int(), true
	case pcommon.ValueTypeDouble:
		return int64(v.Double()), true
	case pcommon.ValueTypeStr:
		n, err := strconv.ParseInt(v.Str(), 10, 64)
		if err == nil {
			return n, true
		}
	}
	return 0, false
}
func getInt(m pcommon.Map, key string) int64 {
	if v, ok := m.Get(key); ok {
		switch v.Type() {
		case pcommon.ValueTypeInt:
			return v.Int()
		case pcommon.ValueTypeDouble:
			return int64(v.Double())
		case pcommon.ValueTypeStr:
			i, _ := strconv.ParseInt(v.Str(), 10, 64)
			return i
		}
	}
	return 0
}
func getTimestamp(m pcommon.Map, key string) pcommon.Timestamp {
	if v, ok := m.Get(key); ok {
		switch v.Type() {
		case pcommon.ValueTypeInt:
			return pcommon.Timestamp(v.Int())
		case pcommon.ValueTypeStr:
			i, _ := strconv.ParseInt(v.Str(), 10, 64)
			return pcommon.Timestamp(i)
		}
	}
	return 0
}
func cloneLabels(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
func applySiteResourceAttrs(attrs pcommon.Map, site siteMatch) {
	if site.name != "" {
		attrs.PutStr("net.site.name", site.name)
	}
	if site.region != "" {
		attrs.PutStr("net.site.region", site.region)
	}
	if site.role != "" {
		attrs.PutStr("net.site.role", site.role)
	}
	for k, v := range site.labels {
		attrs.PutStr(k, v)
	}
}
func applyEndpointAttrs(attrs pcommon.Map, prefix string, site siteMatch) {
	attrs.PutStr(prefix+".site.name", nz(site.name, "unknown"))
	if site.region != "" {
		attrs.PutStr(prefix+".site.region", site.region)
	}
	if site.role != "" {
		attrs.PutStr(prefix+".site.role", site.role)
	}
	if site.directionClass != "" {
		attrs.PutStr(prefix+".site.direction_class", site.directionClass)
	}
}
func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func containsStrFold(xs []string, v string) bool {
	for _, x := range xs {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}
func matchDNSRule(dns string, r AppRuleConfig) bool {
	if dns == "" {
		return len(r.DNSContains) == 0 && len(r.DNSSuffixes) == 0
	}
	for _, x := range r.DNSContains {
		if strings.Contains(strings.ToLower(dns), strings.ToLower(x)) {
			return true
		}
	}
	for _, x := range r.DNSSuffixes {
		if strings.HasSuffix(strings.ToLower(dns), strings.ToLower(x)) {
			return true
		}
	}
	return len(r.DNSContains) == 0 && len(r.DNSSuffixes) == 0
}
func nz(v, fallback string) string {
	if v != "" && v != "unknown" {
		return v
	}
	return fallback
}
