// SPDX-License-Identifier: GPL-3.0-or-later
package model

import (
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	validID       = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	validZone     = regexp.MustCompile(`^[A-Za-z0-9_]{1,32}$`)
	validMAC      = regexp.MustCompile(`(?i)^([0-9a-f]{2}:){5}[0-9a-f]{2}$`)
	validGeo      = regexp.MustCompile(`^[a-z0-9_!.\-]+(@[a-z0-9_!.\-]+)?$`)
	validDuration = regexp.MustCompile(`^[1-9][0-9]*(ms|s|m|h)$`)
)

func Validate(intent Intent) Validation {
	validation := Validation{Errors: []Issue{}, Warnings: []Issue{}}
	err := func(code, objectType, objectID, option, message string) {
		validation.Errors = append(validation.Errors, Issue{Code: code, ObjectType: objectType, ObjectID: objectID, Option: option, Message: message})
	}
	warn := func(code, objectType, objectID, option, message string) {
		validation.Warnings = append(validation.Warnings, Issue{Code: code, ObjectType: objectType, ObjectID: objectID, Option: option, Message: message})
	}

	if intent.Main.SchemaVersion != SchemaVersion {
		err("UNSUPPORTED_SCHEMA", "steer", intent.Main.ID, "schema_version", "only schema 4 is supported")
	}
	if !validID.MatchString(intent.Main.ID) {
		err("INVALID_ID", "steer", intent.Main.ID, "id", "invalid section ID")
	}
	if len(intent.Main.ManagedZones) == 0 && intent.Main.Enabled {
		err("NO_MANAGED_ZONE", "steer", intent.Main.ID, "managed_zone", "enabled Steer requires at least one managed firewall zone")
	}
	for _, zone := range intent.Main.ManagedZones {
		if !validZone.MatchString(zone) {
			err("INVALID_MANAGED_ZONE", "steer", intent.Main.ID, "managed_zone", "invalid firewall zone: "+zone)
		}
	}
	if !oneOf(intent.Main.LogLevel, "error", "warn", "info", "debug") {
		err("INVALID_LOG_LEVEL", "steer", intent.Main.ID, "log_level", "log level must be error, warn, info or debug")
	}
	if intent.Main.DNSCacheCapacity != 0 && (intent.Main.DNSCacheCapacity < 1024 || intent.Main.DNSCacheCapacity > 10_000_000) {
		err("INVALID_CACHE_CAPACITY", "steer", intent.Main.ID, "dns_cache_capacity", "DNS cache capacity must be 1024..10000000")
	}
	if intent.Main.DNSCachePersist {
		err("REQUIRES_SING_BOX_1_14", "steer", intent.Main.ID, "dns_cache_persist", "persistent DNS cache is unavailable on the supported sing-box 1.13 baseline")
	}
	if intent.Main.DNSOptimisticCache {
		err("REQUIRES_SING_BOX_1_14", "steer", intent.Main.ID, "dns_optimistic_cache", "optimistic DNS cache is unavailable on the supported sing-box 1.13 baseline")
	}
	for _, raw := range intent.Main.ProbeURLs {
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
			err("INVALID_PROBE_URL", "steer", intent.Main.ID, "probe_url", "probe must be an HTTPS URL without credentials or fragment: "+raw)
		}
	}
	if len(intent.Main.ProbeURLs) == 0 {
		err("NO_PROBE_URL", "steer", intent.Main.ID, "probe_url", "at least one HTTPS Apply probe is required")
	}

	validateBootstrap(intent.Bootstrap, err)

	globalIDs := map[string]string{}
	register := func(objectType, id string) {
		if !validID.MatchString(id) {
			err("INVALID_ID", objectType, id, "id", "ID must start with lowercase ASCII and contain only lowercase ASCII, digits, _ or -")
		}
		if previous, exists := globalIDs[id]; exists {
			err("GLOBAL_DUPLICATE_ID", objectType, id, "id", "ID is already used by "+previous)
		} else {
			globalIDs[id] = objectType
		}
	}
	register("steer", intent.Main.ID)
	register("bootstrap", intent.Bootstrap.ID)
	for _, value := range intent.Nodes {
		register("node", value.ID)
	}
	for _, value := range intent.Routes {
		register("route", value.ID)
	}
	for _, value := range intent.DNSProfiles {
		register("dns_profile", value.ID)
	}
	for _, value := range intent.LocalProxies {
		register("local_proxy", value.ID)
	}
	for _, value := range intent.Rules {
		register("rule", value.ID)
	}

	nodes := make(map[string]Node, len(intent.Nodes))
	for _, node := range intent.Nodes {
		nodes[node.ID] = node
		if !node.Enabled {
			continue
		}
		validateNode(node, err, warn)
	}
	routes := make(map[string]Route, len(intent.Routes))
	directRouteCount := 0
	for _, route := range intent.Routes {
		routes[route.ID] = route
		if !route.Enabled {
			continue
		}
		if !oneOf(route.Kind, "direct", "block", "single") {
			err("UNSUPPORTED_ROUTE_KIND", "route", route.ID, "kind", "route kind must be direct, block or single")
			continue
		}
		if route.Kind == "direct" {
			directRouteCount++
		}
		if route.Kind == "single" {
			node, exists := nodes[route.Node]
			if !exists {
				err("DANGLING_NODE", "route", route.ID, "node", "referenced node does not exist")
			} else if !node.Enabled {
				err("DISABLED_NODE", "route", route.ID, "node", "referenced node is disabled")
			}
		} else if route.Node != "" {
			err("UNEXPECTED_NODE", "route", route.ID, "node", "only single routes accept a node")
		}
	}
	if directRouteCount != 1 {
		err("DIRECT_ROUTE_COUNT", "route", "", "", "exactly one enabled direct Route is required for bootstrap and core loop prevention")
	}
	dnsProfiles := make(map[string]DNSProfile, len(intent.DNSProfiles))
	for _, profile := range intent.DNSProfiles {
		dnsProfiles[profile.ID] = profile
		if !profile.Enabled {
			continue
		}
		validateDNSProfile(profile, err, warn)
	}
	localProxies := make(map[string]LocalProxy, len(intent.LocalProxies))
	listenPorts := map[string]string{}
	for _, proxy := range intent.LocalProxies {
		localProxies[proxy.ID] = proxy
		if !proxy.Enabled {
			continue
		}
		validateLocalProxy(proxy, err)
		key := net.JoinHostPort(proxy.Listen, strconv.Itoa(proxy.ListenPort))
		if previous, exists := listenPorts[key]; exists {
			err("PORT_COLLISION", "local_proxy", proxy.ID, "listen_port", "listen address collides with "+previous)
		} else {
			listenPorts[key] = proxy.ID
		}
	}

	defaultCount, defaultSeen := 0, false
	for _, rule := range intent.Rules {
		if !rule.Enabled {
			continue
		}
		if defaultSeen && !rule.Default {
			err("RULE_AFTER_DEFAULT", "rule", rule.ID, "", "enabled rule appears after Default")
		}
		if rule.Default {
			defaultCount++
			defaultSeen = true
		}
		validateRule(rule, routes, dnsProfiles, localProxies, err, warn)
	}
	if defaultCount != 1 {
		err("DEFAULT_COUNT", "rule", "", "", "exactly one enabled Default rule is required")
	}
	validation.OK = len(validation.Errors) == 0
	return validation
}

type issueFn func(string, string, string, string, string)

func validateBootstrap(value Bootstrap, err issueFn) {
	if !oneOf(value.Protocol, "udp", "tcp") {
		err("UNSUPPORTED_BOOTSTRAP_PROTOCOL", "bootstrap", value.ID, "protocol", "bootstrap protocol must be udp or tcp")
	}
	if _, parseErr := netip.ParseAddr(value.Server); parseErr != nil {
		err("BOOTSTRAP_NOT_IP", "bootstrap", value.ID, "server", "bootstrap server must be an IP literal")
	}
	validPort(value.ServerPort, "bootstrap", value.ID, "server_port", err)
	if !validStrategy(value.Strategy) {
		err("INVALID_BOOTSTRAP_STRATEGY", "bootstrap", value.ID, "strategy", "invalid bootstrap strategy")
	}
}

func validateNode(value Node, err, warn issueFn) {
	if !oneOf(value.Type, "vless", "hysteria2", "trojan") {
		err("UNSUPPORTED_NODE_TYPE", "node", value.ID, "type", "node type must be vless, hysteria2 or trojan")
		return
	}
	if !validHost(value.Server) {
		err("INVALID_SERVER", "node", value.ID, "server", "invalid node server")
	}
	validPort(value.ServerPort, "node", value.ID, "server_port", err)
	switch value.Type {
	case "vless":
		if value.UUID == "" {
			err("REQUIRED", "node", value.ID, "uuid", "VLESS UUID is required")
		}
		if value.Flow != "" && value.Flow != "xtls-rprx-vision" {
			err("UNSUPPORTED_VLESS_FLOW", "node", value.ID, "flow", "unsupported VLESS flow")
		}
		if value.PacketEncoding != "" && !oneOf(value.PacketEncoding, "xudp", "packetaddr") {
			err("INVALID_PACKET_ENCODING", "node", value.ID, "packet_encoding", "packet encoding must be xudp or packetaddr")
		}
		if value.RealityPublicKey != "" && (value.TLSServerName == "" || value.RealityShortID == "") {
			err("INCOMPLETE_REALITY", "node", value.ID, "reality_public_key", "Reality requires TLS server name and short ID")
		}
	case "hysteria2":
		if value.Password == "" {
			err("REQUIRED", "node", value.ID, "password", "Hysteria2 password is required")
		}
		if value.TLSServerName == "" {
			err("REQUIRED", "node", value.ID, "tls_server_name", "Hysteria2 TLS server name is required")
		}
		for _, portRange := range value.ServerPorts {
			if !validPortRange(portRange) {
				err("INVALID_PORT_RANGE", "node", value.ID, "server_ports", "invalid Hysteria2 port range: "+portRange)
			}
		}
		if value.HopInterval != "" && !validDuration.MatchString(value.HopInterval) {
			err("INVALID_DURATION", "node", value.ID, "hop_interval", "invalid Hysteria2 hop interval")
		}
		if value.ObfsType != "" && value.ObfsType != "salamander" {
			err("UNSUPPORTED_HYSTERIA2_OBFS", "node", value.ID, "obfs_type", "only salamander obfuscation is supported")
		}
		if value.ObfsType != "" && value.ObfsPassword == "" {
			err("REQUIRED", "node", value.ID, "obfs_password", "obfuscation password is required")
		}
		if value.UpMbps < 0 || value.DownMbps < 0 {
			err("INVALID_BANDWIDTH", "node", value.ID, "up_mbps", "bandwidth cannot be negative")
		}
	case "trojan":
		if value.Password == "" {
			err("REQUIRED", "node", value.ID, "password", "Trojan password is required")
		}
		if value.TLSServerName == "" {
			err("REQUIRED", "node", value.ID, "tls_server_name", "Trojan TLS server name is required")
		}
	}
	if value.Insecure {
		warn("INSECURE_TLS", "node", value.ID, "insecure", "TLS certificate verification is disabled")
	}
}

func validateDNSProfile(value DNSProfile, err, warn issueFn) {
	if !oneOf(value.Protocol, "udp", "tcp", "tls", "https", "quic", "h3") {
		err("UNSUPPORTED_DNS_PROTOCOL", "dns_profile", value.ID, "protocol", "DNS protocol must be udp, tcp, tls, https, quic or h3")
	}
	if !validHost(value.Server) {
		err("INVALID_DNS_SERVER", "dns_profile", value.ID, "server", "invalid DNS server")
	}
	validPort(value.ServerPort, "dns_profile", value.ID, "server_port", err)
	if oneOf(value.Protocol, "tls", "https", "quic", "h3") && value.TLSServerName == "" {
		err("REQUIRED", "dns_profile", value.ID, "tls_server_name", "encrypted DNS requires a TLS server name")
	}
	if oneOf(value.Protocol, "https", "h3") && value.Path != "" && !strings.HasPrefix(value.Path, "/") {
		err("INVALID_DNS_PATH", "dns_profile", value.ID, "path", "DoH path must start with /")
	}
	if !validStrategy(value.Strategy) {
		err("INVALID_DNS_STRATEGY", "dns_profile", value.ID, "strategy", "invalid DNS strategy")
	}
	if value.CachePersist {
		err("REQUIRES_SING_BOX_1_14", "dns_profile", value.ID, "cache_persist", "persistent DNS cache is unavailable on sing-box 1.13")
	}
	if value.OptimisticCache {
		err("REQUIRES_SING_BOX_1_14", "dns_profile", value.ID, "optimistic_cache", "optimistic DNS cache is unavailable on sing-box 1.13")
	}
	if value.Insecure {
		warn("INSECURE_TLS", "dns_profile", value.ID, "insecure", "DNS TLS certificate verification is disabled")
	}
}

func validateLocalProxy(value LocalProxy, err issueFn) {
	if !oneOf(value.Protocol, "mixed", "socks", "http") {
		err("UNSUPPORTED_LOCAL_PROXY_PROTOCOL", "local_proxy", value.ID, "protocol", "local proxy protocol must be mixed, socks or http")
	}
	address, parseErr := netip.ParseAddr(value.Listen)
	if parseErr != nil {
		err("INVALID_LISTEN_ADDRESS", "local_proxy", value.ID, "listen", "listen address must be an IP literal")
	}
	validPort(value.ListenPort, "local_proxy", value.ID, "listen_port", err)
	if (value.Username == "") != (value.Password == "") {
		err("INCOMPLETE_LOCAL_PROXY_AUTH", "local_proxy", value.ID, "username", "username and password must be set together")
	}
	if parseErr == nil && !address.IsLoopback() && value.Username == "" {
		err("LOCAL_PROXY_AUTH_REQUIRED", "local_proxy", value.ID, "username", "non-loopback local proxy requires authentication")
	}
}

func validateRule(value Rule, routes map[string]Route, dnsProfiles map[string]DNSProfile, localProxies map[string]LocalProxy, err, warn issueFn) {
	hasMatch := len(value.Inbound)+len(value.DomainMatch)+len(value.IPMatch)+len(value.SourceIPCIDR)+len(value.SourceMACAddress)+len(value.Network)+len(value.Protocol)+len(value.Port) > 0
	if value.Default && hasMatch {
		err("DEFAULT_HAS_MATCH", "rule", value.ID, "", "Default cannot have match conditions")
	}
	if !value.Default && !hasMatch {
		err("EMPTY_MATCH", "rule", value.ID, "", "non-Default rule requires a match condition")
	}
	profile, exists := dnsProfiles[value.DNSProfile]
	if !exists {
		err("DANGLING_DNS_PROFILE", "rule", value.ID, "dns_profile", "referenced DNS Profile does not exist")
	} else if !profile.Enabled {
		err("DISABLED_DNS_PROFILE", "rule", value.ID, "dns_profile", "referenced DNS Profile is disabled")
	}
	route, exists := routes[value.Route]
	if !exists {
		err("DANGLING_ROUTE", "rule", value.ID, "route", "referenced Route does not exist")
	} else if !route.Enabled {
		err("DISABLED_ROUTE", "rule", value.ID, "route", "referenced Route is disabled")
	}
	for _, inbound := range value.Inbound {
		proxy, exists := localProxies[inbound]
		if !exists {
			err("DANGLING_LOCAL_PROXY", "rule", value.ID, "inbound", "local proxy does not exist: "+inbound)
		} else if !proxy.Enabled {
			err("DISABLED_LOCAL_PROXY", "rule", value.ID, "inbound", "local proxy is disabled: "+inbound)
		}
	}
	if len(value.Inbound) > 0 && len(value.SourceMACAddress) > 0 {
		err("INCOMPATIBLE_SOURCE_SELECTORS", "rule", value.ID, "source_mac_address", "local proxy and source MAC cannot be combined")
	}
	for _, expression := range value.DomainMatch {
		validateDomainExpression(expression, value.ID, err)
	}
	for _, expression := range value.IPMatch {
		validateIPExpression(expression, value.ID, err)
	}
	for _, prefix := range value.SourceIPCIDR {
		if _, parseErr := netip.ParsePrefix(prefix); parseErr != nil {
			err("INVALID_IP_CIDR", "rule", value.ID, "source_ip_cidr", "invalid source prefix: "+prefix)
		}
	}
	for _, address := range value.SourceMACAddress {
		if !validMAC.MatchString(address) {
			err("INVALID_SOURCE_MAC_ADDRESS", "rule", value.ID, "source_mac_address", "invalid MAC address: "+address)
		}
	}
	for _, network := range value.Network {
		if !oneOf(network, "tcp", "udp") {
			err("INVALID_RULE_NETWORK", "rule", value.ID, "network", "network must be tcp or udp")
		}
	}
	for _, protocol := range value.Protocol {
		if !oneOf(protocol, "tls", "http", "quic", "dns", "stun", "bittorrent", "dtls", "ssh", "rdp", "ntp") {
			err("INVALID_RULE_PROTOCOL", "rule", value.ID, "protocol", "unsupported sniffed protocol: "+protocol)
		}
	}
	for _, port := range value.Port {
		validPort(port, "rule", value.ID, "port", err)
	}
	if !value.Default && len(value.Inbound)+len(value.DomainMatch)+len(value.SourceIPCIDR)+len(value.SourceMACAddress) == 0 {
		warn("DNS_PROJECTION_EMPTY", "rule", value.ID, "dns_profile", "rule has only connection-stage conditions and produces no DNS projection")
	}
}

func validateDomainExpression(value, id string, err issueFn) {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\t ") {
		err("INVALID_DOMAIN_MATCH", "rule", id, "domain_match", "invalid domain expression: "+value)
		return
	}
	prefix, payload := "keyword", value
	for _, candidate := range []string{"full:", "domain:", "regexp:", "geosite:"} {
		if strings.HasPrefix(value, candidate) {
			prefix, payload = strings.TrimSuffix(candidate, ":"), strings.TrimPrefix(value, candidate)
			break
		}
	}
	if payload == "" {
		err("INVALID_DOMAIN_MATCH", "rule", id, "domain_match", "empty domain expression: "+value)
		return
	}
	switch prefix {
	case "full", "domain":
		if !validDomain(payload) {
			err("INVALID_DOMAIN_MATCH", "rule", id, "domain_match", "invalid domain: "+payload)
		}
	case "regexp":
		if _, compileErr := regexp.Compile(payload); compileErr != nil {
			err("INVALID_DOMAIN_MATCH", "rule", id, "domain_match", "invalid regular expression: "+payload)
		}
	case "geosite":
		if !validGeo.MatchString(payload) {
			err("INVALID_GEO_CATEGORY", "rule", id, "domain_match", "invalid GeoSite category: "+payload)
		}
	case "keyword":
		if strings.Contains(value, ":") {
			err("INVALID_DOMAIN_MATCH", "rule", id, "domain_match", "unknown domain expression prefix: "+value)
		}
	}
}

func validateIPExpression(value, id string, err issueFn) {
	if strings.HasPrefix(value, "geoip:") {
		if !validGeo.MatchString(strings.TrimPrefix(value, "geoip:")) {
			err("INVALID_GEO_CATEGORY", "rule", id, "ip_match", "invalid GeoIP category: "+value)
		}
		return
	}
	if _, parseErr := netip.ParsePrefix(value); parseErr == nil {
		return
	}
	if _, parseErr := netip.ParseAddr(value); parseErr != nil {
		err("INVALID_IP_CIDR", "rule", id, "ip_match", "invalid destination IP or prefix: "+value)
	}
}

func validPort(value int, objectType, objectID, option string, err issueFn) {
	if value < 1 || value > 65535 {
		err("INVALID_PORT", objectType, objectID, option, "port must be 1..65535")
	}
}
func validStrategy(value string) bool {
	return oneOf(value, "prefer_ipv4", "prefer_ipv6", "ipv4_only", "ipv6_only")
}
func oneOf(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}
func validHost(value string) bool {
	if _, err := netip.ParseAddr(value); err == nil {
		return true
	}
	return validDomain(value)
}
func validDomain(value string) bool {
	if value == "" || len(value) > 253 {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}
func validPortRange(value string) bool {
	parts := strings.Split(value, ":")
	if len(parts) > 2 {
		return false
	}
	first, firstErr := strconv.Atoi(parts[0])
	last := first
	lastErr := firstErr
	if len(parts) == 2 {
		last, lastErr = strconv.Atoi(parts[1])
	}
	return firstErr == nil && lastErr == nil && first >= 1 && last <= 65535 && first <= last
}
