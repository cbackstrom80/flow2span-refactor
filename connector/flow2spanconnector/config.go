package flow2spanconnector

import "time"

type Config struct {
	Window             time.Duration `mapstructure:"window"`
	FlushLag           time.Duration `mapstructure:"flush_lag"`
	MaxSpansPerFlush   int           `mapstructure:"max_spans_per_flush"`
	MaxBucketsInMemory int           `mapstructure:"max_buckets_in_memory"`

	Environment    string `mapstructure:"environment"`
	EnvironmentKey string `mapstructure:"environment_key"`

	UseServiceNameRules bool                `mapstructure:"use_service_name_rules"`
	ServiceNameRules    []ServiceRuleConfig `mapstructure:"service_name_rules"`

	SourceAddressKey      string `mapstructure:"source_address_key"`
	DestinationAddressKey string `mapstructure:"destination_address_key"`
	SourcePortKey         string `mapstructure:"source_port_key"`
	DestinationPortKey    string `mapstructure:"destination_port_key"`
	TransportKey          string `mapstructure:"transport_key"`
	BytesKey              string `mapstructure:"bytes_key"`
	PacketsKey            string `mapstructure:"packets_key"`
	StartKey              string `mapstructure:"start_key"`
	EndKey                string `mapstructure:"end_key"`
	TCPFlagsKey           string `mapstructure:"tcp_flags_key"`
	ExporterIPKey         string `mapstructure:"exporter_ip_key"`

	ConversationPercent  float64 `mapstructure:"conversation_percent"`
	ConversationScoreKey string  `mapstructure:"conversation_score_key"`

	Sites             []SiteConfig      `mapstructure:"sites"`
	Links             []LinkConfig      `mapstructure:"links"`
	Exporters         []ExporterConfig  `mapstructure:"exporters"`
	ClientCommunities []CommunityConfig `mapstructure:"client_communities"`
	ServerCommunities []CommunityConfig `mapstructure:"server_communities"`
	Applications      []AppRuleConfig   `mapstructure:"application_rules"`

	Deduplication DedupConfig      `mapstructure:"deduplication"`
	DNS           DNSConfig        `mapstructure:"dns"`
	QoS           QoSConfig        `mapstructure:"qos"`
	TopTalkers    TopTalkerConfig  `mapstructure:"top_talkers"`
	ServiceMap    ServiceMapConfig `mapstructure:"service_map"`
}

type ServiceRuleConfig struct {
	CIDR string `mapstructure:"cidr"`
	Name string `mapstructure:"name"`
}

type SiteConfig struct {
	Name           string            `mapstructure:"name"`
	DisplayName    string            `mapstructure:"display_name"`
	Region         string            `mapstructure:"region"`
	Role           string            `mapstructure:"role"`
	DirectionClass string            `mapstructure:"direction_class"`
	CIDRs          []string          `mapstructure:"cidrs"`
	Labels         map[string]string `mapstructure:"labels"`
}

type LinkConfig struct {
	Name                       string            `mapstructure:"name"`
	Site                       string            `mapstructure:"site"`
	Direction                  string            `mapstructure:"direction"`
	Provider                   string            `mapstructure:"provider"`
	CircuitID                  string            `mapstructure:"circuit_id"`
	Router                     string            `mapstructure:"router"`
	Interface                  string            `mapstructure:"interface"`
	Exporter                   string            `mapstructure:"exporter"`
	Speed                      string            `mapstructure:"speed"`
	WarningUtilizationPercent  float64           `mapstructure:"warning_utilization_percent"`
	CriticalUtilizationPercent float64           `mapstructure:"critical_utilization_percent"`
	Labels                     map[string]string `mapstructure:"labels"`
}

type ExporterConfig struct {
	Name       string `mapstructure:"name"`
	IP         string `mapstructure:"ip"`
	Role       string `mapstructure:"role"`
	Site       string `mapstructure:"site"`
	Vendor     string `mapstructure:"vendor"`
	Priority   int    `mapstructure:"priority"`
	TrustLevel int    `mapstructure:"trust_level"`
}

type CommunityConfig struct {
	Name     string            `mapstructure:"name"`
	Site     string            `mapstructure:"site"`
	Region   string            `mapstructure:"region"`
	Role     string            `mapstructure:"role"`
	CIDRs    []string          `mapstructure:"cidrs"`
	Services []string          `mapstructure:"services"`
	Labels   map[string]string `mapstructure:"labels"`
}

type AppRuleConfig struct {
	Name        string   `mapstructure:"name"`
	DstPorts    []int    `mapstructure:"dst_ports"`
	Protocols   []string `mapstructure:"protocols"`
	DstCIDRs    []string `mapstructure:"dst_cidrs"`
	DNSContains []string `mapstructure:"dns_contains"`
	DNSSuffixes []string `mapstructure:"dns_suffixes"`
}

type DedupConfig struct {
	Enabled                 bool              `mapstructure:"enabled"`
	Window                  time.Duration     `mapstructure:"window"`
	Strategy                string            `mapstructure:"strategy"`
	Bidirectional           bool              `mapstructure:"bidirectional"`
	NormalizeNAT            bool              `mapstructure:"normalize_nat"`
	MatchTolerancePercent   float64           `mapstructure:"match_tolerance_percent"`
	PacketTolerancePercent  float64           `mapstructure:"packet_tolerance_percent"`
	IncludeDedupeAttributes bool              `mapstructure:"include_dedupe_attributes"`
	PriorityRoles           map[string]int    `mapstructure:"priority_roles"`
	Scopes                  map[string]string `mapstructure:"scopes"`
}

type DNSConfig struct {
	Enabled                 bool          `mapstructure:"enabled"`
	LookupMode              string        `mapstructure:"lookup_mode"`
	Timeout                 time.Duration `mapstructure:"timeout"`
	MaxConcurrentLookups    int           `mapstructure:"max_concurrent_lookups"`
	CacheTTL                time.Duration `mapstructure:"cache_ttl"`
	NegativeCacheTTL        time.Duration `mapstructure:"negative_cache_ttl"`
	Nameservers             []string      `mapstructure:"nameservers"`
	LookupPrivateIPs        bool          `mapstructure:"lookup_private_ips"`
	LookupPublicIPs         bool          `mapstructure:"lookup_public_ips"`
	IncludeDNSAttributes    bool          `mapstructure:"include_dns_attributes"`
	IncludeUnresolved       bool          `mapstructure:"include_unresolved_attribute"`
	SanitizeNames           bool          `mapstructure:"sanitize_names"`
	StripTrailingDot        bool          `mapstructure:"strip_trailing_dot"`
	BlockOnLookup           bool          `mapstructure:"block_on_lookup"`
	EmitOnTopTalkerMetrics  bool          `mapstructure:"emit_on_top_talker_metrics"`
	EmitOnRepresentedTraces bool          `mapstructure:"emit_on_represented_traces"`
}

type QoSConfig struct {
	Enabled                bool     `mapstructure:"enabled"`
	SourceAttribute        string   `mapstructure:"source_attribute"`
	FallbackAttributes     []string `mapstructure:"fallback_attributes"`
	DeriveDSCPFromToS      bool     `mapstructure:"derive_dscp_from_tos"`
	EmitOnMetrics          bool     `mapstructure:"emit_on_metrics"`
	EmitOnTopTalkerMetrics bool     `mapstructure:"emit_on_top_talker_metrics"`
	EmitOnLinkQoSMetrics   bool     `mapstructure:"emit_on_link_qos_metrics"`
	EmitOnTraces           bool     `mapstructure:"emit_on_traces"`
	FocusClasses           []string `mapstructure:"focus_classes"`
}

type TopTalkerConfig struct {
	Enabled                     bool     `mapstructure:"enabled"`
	Limit                       int      `mapstructure:"limit"`
	RankBy                      string   `mapstructure:"rank_by"`
	Scopes                      []string `mapstructure:"scopes"`
	EmitMetrics                 bool     `mapstructure:"emit_metrics"`
	EmitTraces                  bool     `mapstructure:"emit_traces"`
	IncludeTraceIDAttribute     bool     `mapstructure:"include_trace_id_attribute"`
	IncludeSpanIDAttribute      bool     `mapstructure:"include_span_id_attribute"`
	IncludeDedupeDimensions     bool     `mapstructure:"include_dedupe_dimensions"`
	MaxSeriesPerWindow          int      `mapstructure:"max_series_per_window"`
	DropTraceIDDimensionMetrics bool     `mapstructure:"drop_trace_id_dimension_for_metrics"`
}

type ServiceMapConfig struct {
	Enabled                bool   `mapstructure:"enabled"`
	NodeType               string `mapstructure:"node_type"` // site_display, site_name, region, community, service, endpoint, endpoint_dns, endpoint_ip
	IncludeApplicationSpan bool   `mapstructure:"include_application_span"`
	// EmitPeerServiceLinks adds peer.service/server.address attributes to client spans.
	// Splunk APM uses these attributes to infer service-map dependencies when spans
	// represent synthetic network conversations rather than instrumented app RPCs.
	EmitPeerServiceLinks bool   `mapstructure:"emit_peer_service_links"`
	ServiceNamespace     string `mapstructure:"service_namespace"`
}
