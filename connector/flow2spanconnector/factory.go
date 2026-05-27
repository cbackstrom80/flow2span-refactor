package flow2spanconnector

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/consumer"
)

var Type = component.MustNewType("flow2span")

func NewFactory() connector.Factory {
	return connector.NewFactory(
		Type,
		createDefaultConfig,
		connector.WithLogsToTraces(createLogsToTracesConnector, component.StabilityLevelAlpha),
		connector.WithLogsToMetrics(createLogsToMetricsConnector, component.StabilityLevelAlpha),
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		Window:             30 * time.Second,
		FlushLag:           5 * time.Second,
		MaxSpansPerFlush:   5000,
		MaxBucketsInMemory: 240,
		Environment:        "prod",
		EnvironmentKey:     "deployment.environment",

		UseServiceNameRules:   true,
		SourceAddressKey:      "source.address",
		DestinationAddressKey: "destination.address",
		SourcePortKey:         "source.port",
		DestinationPortKey:    "destination.port",
		TransportKey:          "network.transport",
		BytesKey:              "flow.io.bytes",
		PacketsKey:            "flow.io.packets",
		StartKey:              "flow.start",
		EndKey:                "flow.end",
		TCPFlagsKey:           "flow.tcp_flags",
		ExporterIPKey:         "netflow.exporter.address",

		ConversationPercent:  100,
		ConversationScoreKey: "bytes",
		ServiceNameRules:     []ServiceRuleConfig{},
		Sites:                []SiteConfig{},
		Links:                []LinkConfig{},
		Exporters:            []ExporterConfig{},
		ClientCommunities:    []CommunityConfig{},
		ServerCommunities:    []CommunityConfig{},
		Applications:         []AppRuleConfig{},

		Deduplication: DedupConfig{
			Enabled:                 true,
			Window:                  30 * time.Second,
			Strategy:                "prefer_highest_priority",
			Bidirectional:           true,
			MatchTolerancePercent:   15,
			PacketTolerancePercent:  20,
			IncludeDedupeAttributes: true,
			PriorityRoles: map[string]int{
				"firewall":     100,
				"edge_router":  90,
				"sdwan_router": 80,
				"core_router":  70,
				"switch":       50,
				"unknown":      10,
			},
		},
		DNS: DNSConfig{
			Enabled:                 true,
			LookupMode:              "reverse",
			Timeout:                 250 * time.Millisecond,
			MaxConcurrentLookups:    50,
			CacheTTL:                24 * time.Hour,
			NegativeCacheTTL:        10 * time.Minute,
			LookupPrivateIPs:        true,
			LookupPublicIPs:         false,
			IncludeDNSAttributes:    true,
			IncludeUnresolved:       true,
			SanitizeNames:           true,
			StripTrailingDot:        true,
			BlockOnLookup:           false,
			EmitOnTopTalkerMetrics:  true,
			EmitOnRepresentedTraces: true,
		},

		QoS: QoSConfig{
			Enabled:                true,
			SourceAttribute:        "flow.tos",
			FallbackAttributes:     []string{"network.tos", "ip.tos", "netflow.tos", "flow.dscp", "network.dscp", "dscp"},
			DeriveDSCPFromToS:      true,
			EmitOnMetrics:          true,
			EmitOnTopTalkerMetrics: true,
			EmitOnLinkQoSMetrics:   true,
			EmitOnTraces:           true,
			FocusClasses:           []string{"voice", "video"},
		},
		TopTalkers: TopTalkerConfig{
			Enabled:                 true,
			Limit:                   20,
			RankBy:                  "bytes",
			Scopes:                  []string{"global", "site", "link", "community_dependency"},
			EmitMetrics:             true,
			EmitTraces:              true,
			IncludeTraceIDAttribute: false,
			IncludeSpanIDAttribute:  false,
			IncludeDedupeDimensions: true,
			MaxSeriesPerWindow:      10000,
		},
	}
}

func createLogsToTracesConnector(_ context.Context, _ connector.Settings, cfg component.Config, next consumer.Traces) (connector.Logs, error) {
	return newConnector(cfg.(*Config), next, nil)
}

func createLogsToMetricsConnector(_ context.Context, _ connector.Settings, cfg component.Config, next consumer.Metrics) (connector.Logs, error) {
	return newConnector(cfg.(*Config), nil, next)
}
