// SPDX-License-Identifier: GPL-3.0-or-later

// Package compiler lowers validated Canonical Intent into one deterministic
// sing-box configuration. Platform adapters provide native inbound fragments
// and generate their operating-system configuration separately.
package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"sort"
	"strings"

	model "github.com/gsh20040816/steer/go/internal/intent"
)

type Output struct {
	IntentDigest         string         `json:"intent_digest"`
	RequiredCapabilities []string       `json:"required_capabilities"`
	GeoRuleSets          []GeoRuleSet   `json:"geo_rule_sets"`
	SingBox              map[string]any `json:"sing_box"`
}

type Options struct {
	StateDirectory string
	Target         Target
}

// Target contains sing-box-native fragments selected by one platform adapter.
// It deliberately contains no nftables, routing-table or service-manager plan.
type Target struct {
	Inbounds             []any        `json:"inbounds"`
	DNSCapture           DNSCapture   `json:"dns_capture"`
	SniffInboundTags     []string     `json:"sniff_inbound_tags"`
	MACBindings          []MACBinding `json:"mac_bindings"`
	RequiredCapabilities []string     `json:"required_capabilities"`
}

// DNSCaptureMode describes which component has already identified DNS
// traffic before the sing-box route rules see it. Keeping this explicit
// prevents a packet-oriented runtime from treating arbitrary UDP sessions as
// DNS.
type DNSCaptureMode string

const (
	DNSCaptureNone          DNSCaptureMode = "none"
	DNSCaptureInboundHijack DNSCaptureMode = "inbound_hijack"
)

// DNSCapture contains only sing-box-facing DNS inbound tags. Platform code
// remains responsible for the first capture layer. The inbound_hijack mode is
// safe only when those tags refer to dedicated DNS flows, never to a packet
// runtime's general UDP sessions.
type DNSCapture struct {
	Mode        DNSCaptureMode `json:"mode"`
	InboundTags []string       `json:"inbound_tags"`
}

type MACBinding struct {
	Address       string `json:"address"`
	TProxyTag     string `json:"tproxy_tag"`
	DNSInboundTag string `json:"dns_inbound_tag"`
}

type DNSPath struct {
	Profile string `json:"profile"`
	Route   string `json:"route"`
	Tag     string `json:"tag"`
}

type GeoRuleSet struct {
	Kind     string `json:"kind"`
	Category string `json:"category"`
	Tag      string `json:"tag"`
	Path     string `json:"path"`
}

func Compile(intent model.Intent, options Options) (Output, error) {
	if options.StateDirectory == "" && requiresGeoStateDirectory(intent) {
		return Output{}, errors.New("compiler state directory is required for Geo rule sets")
	}
	geoSets := collectGeoRuleSets(intent, options.StateDirectory)
	dnsPaths := collectDNSPaths(intent)
	output := Output{
		IntentDigest:         digestIntent(intent),
		RequiredCapabilities: requiredCapabilities(intent, options.Target),
		GeoRuleSets:          geoSets,
	}
	output.SingBox = compileSingBox(intent, options.Target, dnsPaths, geoSets)
	return output, nil
}

func requiresGeoStateDirectory(intent model.Intent) bool {
	for _, rule := range intent.Rules {
		if !rule.Enabled || rule.Default {
			continue
		}
		for _, expression := range rule.DomainMatch {
			if strings.HasPrefix(expression, "geosite:") {
				return true
			}
		}
		for _, expression := range rule.IPMatch {
			if strings.HasPrefix(expression, "geoip:") {
				return true
			}
		}
	}
	return false
}

func digestIntent(intent model.Intent) string {
	encoded, _ := json.Marshal(intent)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func requiredCapabilities(intent model.Intent, target Target) []string {
	capabilities := append([]string{}, target.RequiredCapabilities...)
	for _, node := range intent.Nodes {
		if !node.Enabled {
			continue
		}
		if node.Type == "hysteria" || node.Type == "hysteria2" || node.Type == "tuic" || node.Transport == "quic" || node.QUIC {
			capabilities = append(capabilities, "with_quic")
		}
		if node.UTLSFingerprint != "" {
			capabilities = append(capabilities, "with_utls")
		}
	}
	for _, profile := range intent.DNSProfiles {
		if !profile.Enabled {
			continue
		}
		if profile.Protocol == "quic" || profile.Protocol == "h3" {
			capabilities = append(capabilities, "dns_quic")
		}
	}
	sort.Strings(capabilities)
	return unique(capabilities)
}

func collectDNSPaths(intent model.Intent) []DNSPath {
	seen := map[string]bool{}
	routes := indexRoutes(intent.Routes)
	var paths []DNSPath
	for _, rule := range intent.Rules {
		if !rule.Enabled {
			continue
		}
		if routes[rule.Route].Kind == "block" {
			continue
		}
		key := rule.DNSProfile + "\x00" + rule.Route
		if seen[key] {
			continue
		}
		seen[key] = true
		paths = append(paths, DNSPath{Profile: rule.DNSProfile, Route: rule.Route, Tag: dnsPathTag(rule.DNSProfile, rule.Route)})
	}
	return paths
}

func collectGeoRuleSets(intent model.Intent, stateDirectory string) []GeoRuleSet {
	seen := map[string]bool{}
	var result []GeoRuleSet
	for _, rule := range intent.Rules {
		if !rule.Enabled || rule.Default {
			continue
		}
		for _, expression := range rule.DomainMatch {
			if strings.HasPrefix(expression, "geosite:") {
				addGeoSet(&result, seen, stateDirectory, "geosite", strings.TrimPrefix(expression, "geosite:"))
			}
		}
		for _, expression := range rule.IPMatch {
			if strings.HasPrefix(expression, "geoip:") {
				addGeoSet(&result, seen, stateDirectory, "geoip", strings.TrimPrefix(expression, "geoip:"))
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Tag < result[j].Tag })
	return result
}

func addGeoSet(result *[]GeoRuleSet, seen map[string]bool, stateDirectory, kind, category string) {
	tag := "steer-" + kind + "-" + category
	if seen[tag] {
		return
	}
	seen[tag] = true
	*result = append(*result, GeoRuleSet{Kind: kind, Category: category, Tag: tag, Path: strings.TrimRight(stateDirectory, "/") + "/geodata/current/rules/" + kind + "-" + category + ".srs"})
}

func compileSingBox(intent model.Intent, target Target, dnsPaths []DNSPath, geoRuleSets []GeoRuleSet) map[string]any {
	profiles := indexDNSProfiles(intent.DNSProfiles)
	routes := indexRoutes(intent.Routes)
	macIndex := map[string]MACBinding{}
	for _, binding := range target.MACBindings {
		macIndex[binding.Address] = binding
	}

	inbounds := append([]any{}, target.Inbounds...)
	dnsInboundTags := append([]string{}, target.DNSCapture.InboundTags...)
	sniffInboundTags := append([]string{}, target.SniffInboundTags...)
	for _, proxy := range intent.LocalProxies {
		if !proxy.Enabled {
			continue
		}
		inbound := map[string]any{"type": proxy.Protocol, "tag": localProxyTag(proxy.ID), "listen": proxy.Listen, "listen_port": proxy.ListenPort}
		if proxy.Username != "" {
			inbound["users"] = []any{map[string]any{"username": proxy.Username, "password": proxy.Password}}
		}
		inbounds = append(inbounds, inbound)
		sniffInboundTags = append(sniffInboundTags, localProxyTag(proxy.ID))
	}

	nodes := indexNodes(intent.Nodes)
	outbounds := make([]any, 0, len(intent.Routes))
	for _, route := range intent.Routes {
		if !route.Enabled {
			continue
		}
		switch route.Kind {
		case "direct":
			outbounds = append(outbounds, map[string]any{"type": "direct", "tag": routeTag(route.ID)})
		case "block":
			outbounds = append(outbounds, map[string]any{"type": "block", "tag": routeTag(route.ID)})
		case "single":
			outbounds = append(outbounds, compileRouteOutbound(route, nodes))
		}
	}

	dnsServers := []any{map[string]any{
		"type": intent.Bootstrap.Protocol, "tag": "steer-dns-bootstrap", "server": intent.Bootstrap.Server,
		"server_port": intent.Bootstrap.ServerPort,
	}}
	for _, path := range dnsPaths {
		dnsServers = append(dnsServers, compileDNSPath(profiles[path.Profile], routes[path.Route], path))
	}

	routeRules := make([]any, 0, 2+len(intent.Rules))
	if target.DNSCapture.Mode == DNSCaptureInboundHijack && len(dnsInboundTags) > 0 {
		routeRules = append(routeRules, map[string]any{"inbound": dnsInboundTags, "action": "hijack-dns"})
	}
	routeRules = append(routeRules, map[string]any{"inbound": sniffInboundTags, "action": "sniff", "timeout": "300ms"})
	dnsRules := []any{}
	var defaultRule model.Rule
	for _, rule := range intent.Rules {
		if !rule.Enabled {
			continue
		}
		if rule.Default {
			defaultRule = rule
			continue
		}
		routeMatch := compileRuleMatch(rule, macIndex, false)
		routeMatch["action"] = "route"
		routeMatch["outbound"] = routeTag(rule.Route)
		routeRules = append(routeRules, routeMatch)
		if dnsMatch := compileDNSMatch(rule, macIndex); len(dnsMatch) > 0 {
			if routes[rule.Route].Kind == "block" {
				dnsMatch["action"] = "reject"
			} else {
				dnsMatch["action"] = "route"
				dnsMatch["server"] = dnsPathTag(rule.DNSProfile, rule.Route)
				dnsMatch["strategy"] = profiles[rule.DNSProfile].Strategy
			}
			dnsRules = append(dnsRules, dnsMatch)
		}
	}
	finalDNS := "steer-dns-bootstrap"
	if routes[defaultRule.Route].Kind == "block" {
		dnsRules = append(dnsRules, map[string]any{"action": "reject"})
	} else {
		finalDNS = dnsPathTag(defaultRule.DNSProfile, defaultRule.Route)
	}
	ruleSets := make([]any, 0, len(geoRuleSets))
	for _, ruleSet := range geoRuleSets {
		ruleSets = append(ruleSets, map[string]any{"type": "local", "tag": ruleSet.Tag, "format": "binary", "path": ruleSet.Path})
	}
	return map[string]any{
		"log": map[string]any{"level": intent.Main.LogLevel, "timestamp": true},
		"dns": map[string]any{"servers": dnsServers, "rules": dnsRules, "final": finalDNS, "strategy": profiles[defaultRule.DNSProfile].Strategy,
			"independent_cache": true, "cache_capacity": intent.Main.DNSCacheCapacity, "reverse_mapping": true},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"route": map[string]any{"rules": routeRules, "rule_set": ruleSets, "final": routeTag(defaultRule.Route), "auto_detect_interface": true,
			"default_domain_resolver": map[string]any{"server": "steer-dns-bootstrap", "strategy": intent.Bootstrap.Strategy}},
	}
}

func compileNode(node model.Node) map[string]any {
	result := map[string]any{"type": node.Type, "tag": nodeTag(node.ID)}
	if node.Type != "tor" {
		result["server"] = node.Server
		if len(node.ServerPorts) > 0 {
			result["server_ports"] = node.ServerPorts
		} else {
			result["server_port"] = node.ServerPort
		}
	}
	switch node.Type {
	case "socks":
		if node.Username != "" {
			result["username"] = node.Username
		}
		if node.Password != "" {
			result["password"] = node.Password
		}
	case "http":
		result["username"], result["password"] = node.Username, node.Password
		if node.TLSServerName != "" || node.Insecure {
			result["tls"] = compileTLS(node.TLSServerName, node.Insecure, node.UTLSFingerprint, "", "")
		}
	case "shadowsocks":
		result["method"], result["password"] = node.Method, node.Password
		if node.Plugin != "" {
			result["plugin"], result["plugin_opts"] = node.Plugin, node.PluginOptions
		}
	case "vmess":
		result["uuid"] = node.UUID
		if node.Security != "" {
			result["security"] = node.Security
		}
		if node.AlterID != 0 {
			result["alter_id"] = node.AlterID
		}
		if node.Network != "" {
			result["network"] = node.Network
		}
		if node.PacketEncoding != "" {
			result["packet_encoding"] = node.PacketEncoding
		}
		if tls := compileTLSIfConfigured(node); tls != nil {
			result["tls"] = tls
		}
		if transport := compileTransport(node); transport != nil {
			result["transport"] = transport
		}
	case "vless":
		result["uuid"], result["flow"], result["packet_encoding"] = node.UUID, node.Flow, node.PacketEncoding
		if tls := compileTLSIfConfigured(node); tls != nil {
			result["tls"] = tls
		}
		if transport := compileTransport(node); transport != nil {
			result["transport"] = transport
		}
	case "trojan":
		result["password"] = node.Password
		if tls := compileTLSIfConfigured(node); tls != nil {
			result["tls"] = tls
		}
		if transport := compileTransport(node); transport != nil {
			result["transport"] = transport
		}
	case "hysteria":
		result["auth_str"], result["hop_interval"], result["up_mbps"], result["down_mbps"] = node.Password, node.HopInterval, node.UpMbps, node.DownMbps
		result["tls"] = compileTLS(node.TLSServerName, node.Insecure, node.UTLSFingerprint, "", "")
		if node.ObfsPassword != "" {
			result["obfs"] = node.ObfsPassword
		}
	case "shadowtls":
		result["version"], result["password"] = node.Version, node.Password
		result["tls"] = compileTLS(node.TLSServerName, node.Insecure, node.UTLSFingerprint, "", "")
	case "tuic":
		result["uuid"], result["password"] = node.UUID, node.Password
		result["congestion_control"] = node.CongestionControl
		if node.UDPRelayMode != "" {
			result["udp_relay_mode"] = node.UDPRelayMode
		} else if node.UDPOverStream {
			result["udp_over_stream"] = true
		}
		result["zero_rtt_handshake"], result["heartbeat"] = node.ZeroRTTHandshake, node.Heartbeat
		result["tls"] = compileTLS(node.TLSServerName, node.Insecure, node.UTLSFingerprint, "", "")
	case "hysteria2":
		result["password"], result["hop_interval"], result["up_mbps"], result["down_mbps"] = node.Password, node.HopInterval, node.UpMbps, node.DownMbps
		result["tls"] = compileTLS(node.TLSServerName, node.Insecure, node.UTLSFingerprint, "", "")
		if node.ObfsType != "" {
			result["obfs"] = map[string]any{"type": node.ObfsType, "password": node.ObfsPassword}
		}
	case "anytls":
		result["password"] = node.Password
		result["tls"] = compileTLS(node.TLSServerName, node.Insecure, node.UTLSFingerprint, "", "")
	case "naive":
		result["username"], result["password"] = node.Username, node.Password
		if node.InsecureConcurrency != 0 {
			result["insecure_concurrency"] = node.InsecureConcurrency
		}
		if node.QUIC {
			result["quic"] = true
		}
		if node.QUICCongestionControl != "" {
			result["quic_congestion_control"] = node.QUICCongestionControl
		}
		result["tls"] = compileTLS(node.TLSServerName, node.Insecure, node.UTLSFingerprint, "", "")
	case "ssh":
		result["user"], result["password"] = node.Username, node.Password
		if node.PrivateKey != "" {
			result["private_key"] = node.PrivateKey
		}
		if len(node.HostKeyAlgorithms) > 0 {
			result["host_key_algorithms"] = node.HostKeyAlgorithms
		}
		if node.HostKey != "" {
			result["host_key"] = []string{node.HostKey}
		}
	case "tor":
		if node.ExecutablePath != "" {
			result["executable_path"] = node.ExecutablePath
		}
		if len(node.ExtraArgs) > 0 {
			result["extra_args"] = node.ExtraArgs
		}
		if node.DataDirectory != "" {
			result["data_directory"] = node.DataDirectory
		}
	}
	return clean(result)
}

// CompileNodeOutbound exposes the same typed lowering used by the final config
// for temporary node diagnostics such as speed tests.
func CompileNodeOutbound(node model.Node) map[string]any { return compileNode(node) }

func NodeOutboundTag(id string) string { return nodeTag(id) }

func RouteOutboundTag(id string) string { return routeTag(id) }

// CompileRouteChainOutbounds returns only the target single-node Route and its
// detour ancestry. Callers must validate the Intent before using the result.
func CompileRouteChainOutbounds(intent model.Intent, routeID string) []any {
	routes := indexRoutes(intent.Routes)
	included := map[string]bool{}
	for current := routeID; current != "" && !included[current]; {
		route, exists := routes[current]
		if !exists || !route.Enabled || route.Kind != "single" {
			return nil
		}
		included[current] = true
		current = route.Detour
	}
	nodes := indexNodes(intent.Nodes)
	outbounds := make([]any, 0, len(included))
	for _, route := range intent.Routes {
		if included[route.ID] {
			outbounds = append(outbounds, compileRouteOutbound(route, nodes))
		}
	}
	return outbounds
}

func compileRouteOutbound(route model.Route, nodes map[string]model.Node) map[string]any {
	outbound := compileNode(nodes[route.Node])
	outbound["tag"] = routeTag(route.ID)
	if route.Detour != "" {
		outbound["detour"] = routeTag(route.Detour)
	}
	return outbound
}

func compileTLSIfConfigured(node model.Node) map[string]any {
	if node.TLSServerName == "" && node.RealityPublicKey == "" && !node.Insecure && node.UTLSFingerprint == "" {
		return nil
	}
	return compileTLS(node.TLSServerName, node.Insecure, node.UTLSFingerprint, node.RealityPublicKey, node.RealityShortID)
}

func compileTransport(node model.Node) map[string]any {
	switch node.Transport {
	case "", "tcp", "raw":
		return nil
	case "ws":
		result := map[string]any{"type": "ws", "path": node.TransportPath}
		if node.TransportHost != "" {
			result["headers"] = map[string]any{"Host": []string{node.TransportHost}}
		}
		return result
	case "grpc":
		return map[string]any{"type": "grpc", "service_name": node.ServiceName}
	case "http":
		result := map[string]any{"type": "http", "path": node.TransportPath}
		if node.TransportHost != "" {
			result["host"] = []string{node.TransportHost}
		}
		return result
	case "quic":
		return map[string]any{"type": "quic"}
	default:
		return nil
	}
}

func compileTLS(serverName string, insecure bool, fingerprint, publicKey, shortID string) map[string]any {
	result := map[string]any{"enabled": true, "server_name": serverName, "insecure": insecure}
	if fingerprint != "" {
		result["utls"] = map[string]any{"enabled": true, "fingerprint": fingerprint}
	}
	if publicKey != "" {
		result["reality"] = map[string]any{"enabled": true, "public_key": publicKey, "short_id": shortID}
	}
	return clean(result)
}

func compileDNSPath(profile model.DNSProfile, route model.Route, path DNSPath) map[string]any {
	result := map[string]any{"type": profile.Protocol, "tag": path.Tag, "server": profile.Server, "server_port": profile.ServerPort}
	if route.Kind == "single" {
		result["detour"] = routeTag(path.Route)
	}
	if _, err := netip.ParseAddr(profile.Server); err != nil {
		result["domain_resolver"] = map[string]any{"server": "steer-dns-bootstrap", "strategy": profile.Strategy}
	}
	if profile.Protocol == "tls" || profile.Protocol == "https" || profile.Protocol == "quic" || profile.Protocol == "h3" {
		result["tls"] = map[string]any{"enabled": true, "server_name": profile.TLSServerName, "insecure": profile.Insecure}
	}
	if profile.Protocol == "https" || profile.Protocol == "h3" {
		result["path"] = profile.Path
	}
	return clean(result)
}

func compileRuleMatch(rule model.Rule, macIndex map[string]MACBinding, dns bool) map[string]any {
	groups := []map[string]any{}
	if len(rule.Inbound) > 0 {
		values := make([]string, len(rule.Inbound))
		for i, value := range rule.Inbound {
			values[i] = localProxyTag(value)
		}
		groups = append(groups, map[string]any{"inbound": values})
	}
	if len(rule.SourceMACAddress) > 0 {
		values := []string{}
		for _, address := range rule.SourceMACAddress {
			binding := macIndex[strings.ToLower(address)]
			if dns {
				values = append(values, binding.DNSInboundTag)
			} else {
				values = append(values, binding.TProxyTag)
			}
		}
		groups = append(groups, map[string]any{"inbound": values})
	}
	if len(rule.DomainMatch) > 0 {
		groups = append(groups, compileDomainGroup(rule.DomainMatch))
	}
	if !dns && len(rule.IPMatch) > 0 {
		groups = append(groups, compileIPGroup(rule.IPMatch))
	}
	if len(rule.SourceIPCIDR) > 0 {
		groups = append(groups, map[string]any{"source_ip_cidr": rule.SourceIPCIDR})
	}
	if !dns {
		if len(rule.Network) > 0 {
			groups = append(groups, map[string]any{"network": rule.Network})
		}
		if len(rule.Protocol) > 0 {
			groups = append(groups, map[string]any{"protocol": rule.Protocol})
		}
		if len(rule.Port) > 0 {
			groups = append(groups, map[string]any{"port": rule.Port})
		}
	}
	return combine(groups, "and")
}

func compileDNSMatch(rule model.Rule, macIndex map[string]MACBinding) map[string]any {
	return compileRuleMatch(rule, macIndex, true)
}

func compileDomainGroup(expressions []string) map[string]any {
	groups := []map[string]any{}
	fields := map[string][]string{}
	for _, expression := range expressions {
		field, value := "domain_keyword", expression
		switch {
		case strings.HasPrefix(expression, "full:"):
			field, value = "domain", strings.TrimPrefix(expression, "full:")
		case strings.HasPrefix(expression, "domain:"):
			field, value = "domain_suffix", strings.TrimPrefix(expression, "domain:")
		case strings.HasPrefix(expression, "regexp:"):
			field, value = "domain_regex", strings.TrimPrefix(expression, "regexp:")
		case strings.HasPrefix(expression, "geosite:"):
			field, value = "rule_set", "steer-geosite-"+strings.TrimPrefix(expression, "geosite:")
		}
		fields[field] = append(fields[field], value)
	}
	keys := sortedKeys(fields)
	for _, field := range keys {
		groups = append(groups, map[string]any{field: fields[field]})
	}
	return combine(groups, "or")
}

func compileIPGroup(expressions []string) map[string]any {
	fields := map[string][]string{}
	for _, expression := range expressions {
		if strings.HasPrefix(expression, "geoip:") {
			fields["rule_set"] = append(fields["rule_set"], "steer-geoip-"+strings.TrimPrefix(expression, "geoip:"))
		} else {
			fields["ip_cidr"] = append(fields["ip_cidr"], expression)
		}
	}
	groups := []map[string]any{}
	for _, field := range sortedKeys(fields) {
		groups = append(groups, map[string]any{field: fields[field]})
	}
	return combine(groups, "or")
}

func combine(groups []map[string]any, mode string) map[string]any {
	if len(groups) == 0 {
		return map[string]any{}
	}
	if len(groups) == 1 {
		return groups[0]
	}
	rules := make([]any, len(groups))
	for index, group := range groups {
		rules[index] = group
	}
	return map[string]any{"type": "logical", "mode": mode, "rules": rules}
}

func clean(value map[string]any) map[string]any {
	for key, item := range value {
		switch typed := item.(type) {
		case string:
			if typed == "" {
				delete(value, key)
			}
		case bool:
			if !typed {
				delete(value, key)
			}
		case int:
			if typed == 0 {
				delete(value, key)
			}
		case []string:
			if len(typed) == 0 {
				delete(value, key)
			}
		case map[string]any:
			value[key] = clean(typed)
			if len(value[key].(map[string]any)) == 0 {
				delete(value, key)
			}
		}
	}
	return value
}

func indexDNSProfiles(values []model.DNSProfile) map[string]model.DNSProfile {
	result := map[string]model.DNSProfile{}
	for _, value := range values {
		result[value.ID] = value
	}
	return result
}
func indexRoutes(values []model.Route) map[string]model.Route {
	result := map[string]model.Route{}
	for _, value := range values {
		result[value.ID] = value
	}
	return result
}
func indexNodes(values []model.Node) map[string]model.Node {
	result := map[string]model.Node{}
	for _, value := range values {
		result[value.ID] = value
	}
	return result
}
func routeTag(id string) string               { return "steer-route-" + id }
func nodeTag(id string) string                { return "steer-node-" + id }
func localProxyTag(id string) string          { return "steer-local-" + id }
func dnsPathTag(profile, route string) string { return "steer-dns-" + profile + "-via-" + route }
func sortedKeys[V any](values map[string]V) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
func unique(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
