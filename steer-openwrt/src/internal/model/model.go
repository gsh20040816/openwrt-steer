// SPDX-License-Identifier: GPL-3.0-or-later

// Package model defines Steer's platform-neutral canonical intent.
package model

const SchemaVersion = 5

type Intent struct {
	Main         Main         `json:"main"`
	Bootstrap    Bootstrap    `json:"bootstrap"`
	Nodes        []Node       `json:"nodes"`
	Routes       []Route      `json:"routes"`
	DNSProfiles  []DNSProfile `json:"dns_profiles"`
	LocalProxies []LocalProxy `json:"local_proxies"`
	Rules        []Rule       `json:"rules"`
}

type Main struct {
	ID                 string   `json:"id"`
	SchemaVersion      int      `json:"schema_version"`
	Enabled            bool     `json:"enabled"`
	LogLevel           string   `json:"log_level"`
	ProbeDirectURLs    []string `json:"probe_direct_urls"`
	ProbeProxyURLs     []string `json:"probe_proxy_urls"`
	SpeedtestProxyURLs []string `json:"speedtest_proxy_urls"`
	DNSCacheCapacity   int      `json:"dns_cache_capacity,omitempty"`
	DNSCachePersist    bool     `json:"dns_cache_persist,omitempty"`
	DNSOptimisticCache bool     `json:"dns_optimistic_cache,omitempty"`
}

type Bootstrap struct {
	ID         string `json:"id"`
	Protocol   string `json:"protocol"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	Strategy   string `json:"strategy"`
}

type Node struct {
	ID               string   `json:"id"`
	Enabled          bool     `json:"enabled"`
	Name             string   `json:"name,omitempty"`
	Type             string   `json:"type"`
	Server           string   `json:"server"`
	ServerPort       int      `json:"server_port"`
	UUID             string   `json:"uuid,omitempty"`
	Flow             string   `json:"flow,omitempty"`
	PacketEncoding   string   `json:"packet_encoding,omitempty"`
	Password         string   `json:"password,omitempty"`
	ServerPorts      []string `json:"server_ports,omitempty"`
	HopInterval      string   `json:"hop_interval,omitempty"`
	ObfsType         string   `json:"obfs_type,omitempty"`
	ObfsPassword     string   `json:"obfs_password,omitempty"`
	UpMbps           int      `json:"up_mbps,omitempty"`
	DownMbps         int      `json:"down_mbps,omitempty"`
	TLSServerName    string   `json:"tls_server_name,omitempty"`
	Insecure         bool     `json:"insecure,omitempty"`
	RealityPublicKey string   `json:"reality_public_key,omitempty"`
	RealityShortID   string   `json:"reality_short_id,omitempty"`
	UTLSFingerprint  string   `json:"utls_fingerprint,omitempty"`
}

type Route struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	Name    string `json:"name,omitempty"`
	Kind    string `json:"kind"`
	Node    string `json:"node,omitempty"`
}

type DNSProfile struct {
	ID              string `json:"id"`
	Enabled         bool   `json:"enabled"`
	Name            string `json:"name,omitempty"`
	Protocol        string `json:"protocol"`
	Server          string `json:"server"`
	ServerPort      int    `json:"server_port"`
	TLSServerName   string `json:"tls_server_name,omitempty"`
	Path            string `json:"path,omitempty"`
	Insecure        bool   `json:"insecure,omitempty"`
	Strategy        string `json:"strategy"`
	CachePersist    bool   `json:"cache_persist,omitempty"`
	OptimisticCache bool   `json:"optimistic_cache,omitempty"`
}

type LocalProxy struct {
	ID         string `json:"id"`
	Enabled    bool   `json:"enabled"`
	Name       string `json:"name,omitempty"`
	Protocol   string `json:"protocol"`
	Listen     string `json:"listen"`
	ListenPort int    `json:"listen_port"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
}

type Rule struct {
	ID               string   `json:"id"`
	Enabled          bool     `json:"enabled"`
	Default          bool     `json:"default"`
	Name             string   `json:"name,omitempty"`
	DNSProfile       string   `json:"dns_profile"`
	Route            string   `json:"route"`
	Inbound          []string `json:"inbound,omitempty"`
	DomainMatch      []string `json:"domain_match,omitempty"`
	IPMatch          []string `json:"ip_match,omitempty"`
	SourceIPCIDR     []string `json:"source_ip_cidr,omitempty"`
	SourceMACAddress []string `json:"source_mac_address,omitempty"`
	Network          []string `json:"network,omitempty"`
	Protocol         []string `json:"protocol,omitempty"`
	Port             []int    `json:"port,omitempty"`
}

type Issue struct {
	Code       string `json:"code"`
	ObjectType string `json:"object_type,omitempty"`
	ObjectID   string `json:"object_id,omitempty"`
	Option     string `json:"option,omitempty"`
	Message    string `json:"message"`
}

type Validation struct {
	OK       bool    `json:"ok"`
	Errors   []Issue `json:"errors"`
	Warnings []Issue `json:"warnings"`
}
