// SPDX-License-Identifier: GPL-3.0-or-later

// Package compiler lowers validated Canonical Intent into a platform-neutral
// Execution Plan and a deterministic sing-box 1.13 configuration.
package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/model"
)

const (
	TunInterface = "steer0"
	DNSPort      = 1053
	// MAC policy packets need a distinct mark now that their policy rule is global.
	// Reusing the sing-box output mark would route locally generated auto-redirect
	// traffic through the MAC table before sing-box's own rules can handle it.
	MACMark                = 0x2026
	MACTable               = 2023
	MACPriority            = 8999
	TunTable               = 2022
	TunPriority            = 9000
	TunFallbackPriority    = 32768
	AutoRedirectInputMark  = 0x2023
	AutoRedirectOutputMark = 0x2024
	AutoRedirectResetMark  = 0x2025
	AutoRedirectNFQueue    = 100
)

var nonGlobalIPv4 = []string{
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
	"172.16.0.0/12", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16",
	"198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
}

var nonGlobalIPv6 = []string{
	"::/128", "::1/128", "::ffff:0:0/96", "64:ff9b:1::/48", "100::/64", "100:0:0:1::/64",
	"2001:db8::/32", "2002::/16", "3fff::/20", "5f00::/16", "fc00::/7",
	"fe80::/10", "ff00::/8",
}

type Bundle struct {
	IntentDigest string           `json:"intent_digest"`
	Validation   model.Validation `json:"validation"`
	Plan         Plan             `json:"plan"`
	SingBox      map[string]any   `json:"sing_box"`
}

type Options struct {
	StateDirectory string
}

type Plan struct {
	SchemaVersion        int          `json:"schema_version"`
	RequiredCapabilities []string     `json:"required_capabilities"`
	Resources            Resources    `json:"resources"`
	DNSPaths             []DNSPath    `json:"dns_paths"`
	GeoRuleSets          []GeoRuleSet `json:"geo_rule_sets"`
	Objects              []PlanObject `json:"objects"`
	ProbeDirect          []string     `json:"probe_direct"`
	ProbeProxy           []string     `json:"probe_proxy"`
	SpeedtestProxy       []string     `json:"speedtest_proxy"`
}

type PlanObject struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Digest string `json:"digest"`
}

type PlanDiff struct {
	Changed          bool         `json:"changed"`
	Added            []PlanObject `json:"added"`
	Removed          []PlanObject `json:"removed"`
	Modified         []PlanObject `json:"modified"`
	ResourcesChanged bool         `json:"resources_changed"`
	ProbesChanged    bool         `json:"probes_changed"`
}

type Resources struct {
	TunInterface           string       `json:"tun_interface"`
	TunAddresses           []string     `json:"tun_addresses"`
	DNSPort                int          `json:"dns_port"`
	TunTable               int          `json:"tun_table"`
	TunPriority            int          `json:"tun_priority"`
	TunFallbackPriority    int          `json:"tun_fallback_priority"`
	AutoRedirectInputMark  int          `json:"auto_redirect_input_mark"`
	AutoRedirectOutputMark int          `json:"auto_redirect_output_mark"`
	AutoRedirectResetMark  int          `json:"auto_redirect_reset_mark"`
	AutoRedirectNFQueue    int          `json:"auto_redirect_nfqueue"`
	MACMark                int          `json:"mac_mark,omitempty"`
	MACTable               int          `json:"mac_table,omitempty"`
	MACPriority            int          `json:"mac_priority,omitempty"`
	MACBindings            []MACBinding `json:"mac_bindings"`
}

type MACBinding struct {
	Address       string `json:"address"`
	TProxyPort    int    `json:"tproxy_port"`
	DNSPort       int    `json:"dns_port"`
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

func Compile(intent model.Intent) Bundle {
	return CompileWithOptions(intent, Options{StateDirectory: "/var/lib/steer"})
}

func CompileWithOptions(intent model.Intent, options Options) Bundle {
	validation := model.Validate(intent)
	if !validation.OK {
		return Bundle{Validation: validation}
	}
	digest := digestIntent(intent)
	bindings := allocateMACBindings(intent)
	if options.StateDirectory == "" {
		options.StateDirectory = "/var/lib/steer"
	}
	geoSets := collectGeoRuleSets(intent, options.StateDirectory)
	dnsPaths := collectDNSPaths(intent)
	plan := Plan{
		SchemaVersion:        model.SchemaVersion,
		RequiredCapabilities: requiredCapabilities(intent, bindings),
		Resources: Resources{
			TunInterface: TunInterface,
			TunAddresses: []string{"198.18.0.1/30", "fdfe:dcba:9876::1/126"},
			DNSPort:      DNSPort, TunTable: TunTable, TunPriority: TunPriority,
			TunFallbackPriority: TunFallbackPriority, AutoRedirectInputMark: AutoRedirectInputMark,
			AutoRedirectOutputMark: AutoRedirectOutputMark, AutoRedirectResetMark: AutoRedirectResetMark,
			AutoRedirectNFQueue: AutoRedirectNFQueue, MACMark: MACMark,
			MACTable: MACTable, MACPriority: MACPriority, MACBindings: bindings,
		},
		DNSPaths:       dnsPaths,
		GeoRuleSets:    geoSets,
		Objects:        planObjects(intent),
		ProbeDirect:    append([]string(nil), intent.Main.ProbeDirectURLs...),
		ProbeProxy:     append([]string(nil), intent.Main.ProbeProxyURLs...),
		SpeedtestProxy: append([]string(nil), intent.Main.SpeedtestProxyURLs...),
	}
	return Bundle{IntentDigest: digest, Validation: validation, Plan: plan, SingBox: compileSingBox(intent, plan)}
}

func Diff(current, candidate Plan) PlanDiff {
	result := PlanDiff{
		Added:            []PlanObject{},
		Removed:          []PlanObject{},
		Modified:         []PlanObject{},
		ResourcesChanged: !equalJSON(current.Resources, candidate.Resources),
		ProbesChanged:    !equalJSON([][]string{current.ProbeDirect, current.ProbeProxy, current.SpeedtestProxy}, [][]string{candidate.ProbeDirect, candidate.ProbeProxy, candidate.SpeedtestProxy}),
	}
	oldObjects, newObjects := map[string]PlanObject{}, map[string]PlanObject{}
	for _, object := range current.Objects {
		oldObjects[object.Type+"\x00"+object.ID] = object
	}
	for _, object := range candidate.Objects {
		newObjects[object.Type+"\x00"+object.ID] = object
	}
	for key, object := range newObjects {
		previous, exists := oldObjects[key]
		if !exists {
			result.Added = append(result.Added, object)
		} else if previous.Digest != object.Digest {
			result.Modified = append(result.Modified, object)
		}
	}
	for key, object := range oldObjects {
		if _, exists := newObjects[key]; !exists {
			result.Removed = append(result.Removed, object)
		}
	}
	sortPlanObjects(result.Added)
	sortPlanObjects(result.Removed)
	sortPlanObjects(result.Modified)
	result.Changed = result.ResourcesChanged || result.ProbesChanged || len(result.Added)+len(result.Removed)+len(result.Modified) > 0
	return result
}

func planObjects(intent model.Intent) []PlanObject {
	var result []PlanObject
	add := func(objectType, id string, value any) {
		encoded, _ := json.Marshal(value)
		sum := sha256.Sum256(encoded)
		result = append(result, PlanObject{Type: objectType, ID: id, Digest: hex.EncodeToString(sum[:])})
	}
	add("steer", intent.Main.ID, intent.Main)
	add("bootstrap", intent.Bootstrap.ID, intent.Bootstrap)
	for _, value := range intent.Nodes {
		add("node", value.ID, value)
	}
	for _, value := range intent.Subscriptions {
		add("subscription", value.ID, value)
	}
	for _, value := range intent.Routes {
		add("route", value.ID, value)
	}
	for _, value := range intent.DNSProfiles {
		add("dns_profile", value.ID, value)
	}
	for _, value := range intent.LocalProxies {
		add("local_proxy", value.ID, value)
	}
	for _, value := range intent.Rules {
		add("rule", value.ID, value)
	}
	sortPlanObjects(result)
	return result
}

func sortPlanObjects(values []PlanObject) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Type == values[j].Type {
			return values[i].ID < values[j].ID
		}
		return values[i].Type < values[j].Type
	})
}

func equalJSON(left, right any) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}

func digestIntent(intent model.Intent) string {
	encoded, _ := json.Marshal(intent)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func requiredCapabilities(intent model.Intent, bindings []MACBinding) []string {
	capabilities := []string{"tun", "auto_route", "auto_redirect"}
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
	if len(bindings) > 0 {
		capabilities = append(capabilities, "tproxy")
	}
	sort.Strings(capabilities)
	return unique(capabilities)
}

func allocateMACBindings(intent model.Intent) []MACBinding {
	seen := map[string]bool{}
	var addresses []string
	for _, rule := range intent.Rules {
		if !rule.Enabled || rule.Default {
			continue
		}
		for _, address := range rule.SourceMACAddress {
			address = strings.ToLower(address)
			if !seen[address] {
				seen[address] = true
				addresses = append(addresses, address)
			}
		}
	}
	sort.Strings(addresses)
	occupied := map[int]bool{DNSPort: true}
	for _, proxy := range intent.LocalProxies {
		if proxy.Enabled {
			occupied[proxy.ListenPort] = true
		}
	}
	nextPort := 20000
	allocate := func() int {
		for occupied[nextPort] {
			nextPort++
		}
		result := nextPort
		occupied[result] = true
		nextPort++
		return result
	}
	bindings := make([]MACBinding, 0, len(addresses))
	for index, address := range addresses {
		bindings = append(bindings, MACBinding{Address: address, TProxyPort: allocate(), DNSPort: allocate(),
			TProxyTag: fmt.Sprintf("steer-mac-tproxy-%d", index), DNSInboundTag: fmt.Sprintf("steer-mac-dns-%d", index)})
	}
	return bindings
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

func compileSingBox(intent model.Intent, plan Plan) map[string]any {
	profiles := indexDNSProfiles(intent.DNSProfiles)
	routes := indexRoutes(intent.Routes)
	macIndex := map[string]MACBinding{}
	for _, binding := range plan.Resources.MACBindings {
		macIndex[binding.Address] = binding
	}

	inbounds := []any{map[string]any{
		"type": "tun", "tag": "steer-tun", "interface_name": TunInterface,
		"address": plan.Resources.TunAddresses, "mtu": 9000, "auto_route": true,
		"strict_route": true, "auto_redirect": true, "stack": "system",
		"iproute2_table_index": plan.Resources.TunTable, "iproute2_rule_index": plan.Resources.TunPriority,
		"auto_redirect_iproute2_fallback_rule_index": plan.Resources.TunFallbackPriority,
		"auto_redirect_input_mark":                   plan.Resources.AutoRedirectInputMark,
		"auto_redirect_output_mark":                  plan.Resources.AutoRedirectOutputMark,
		"auto_redirect_reset_mark":                   plan.Resources.AutoRedirectResetMark,
		"auto_redirect_nfqueue":                      plan.Resources.AutoRedirectNFQueue,
		"route_exclude_address":                      append(append([]string{}, nonGlobalIPv4...), nonGlobalIPv6...),
	}, map[string]any{
		"type": "direct", "tag": "steer-dns", "listen": "::", "listen_port": DNSPort,
		"network": []string{"tcp", "udp"},
	}}
	dnsInboundTags := []string{"steer-dns"}
	sniffInboundTags := []string{"steer-tun"}
	for _, binding := range plan.Resources.MACBindings {
		inbounds = append(inbounds,
			map[string]any{"type": "tproxy", "tag": binding.TProxyTag, "listen": "::", "listen_port": binding.TProxyPort, "network": []string{"tcp", "udp"}},
			map[string]any{"type": "direct", "tag": binding.DNSInboundTag, "listen": "::", "listen_port": binding.DNSPort, "network": []string{"tcp", "udp"}},
		)
		dnsInboundTags = append(dnsInboundTags, binding.DNSInboundTag)
		sniffInboundTags = append(sniffInboundTags, binding.TProxyTag)
	}
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

	outbounds := make([]any, 0, len(intent.Nodes)+len(intent.Routes))
	for _, node := range intent.Nodes {
		if node.Enabled {
			outbounds = append(outbounds, compileNode(node))
		}
	}
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
			outbounds = append(outbounds, map[string]any{"type": "selector", "tag": routeTag(route.ID), "outbounds": []string{nodeTag(route.Node)}, "default": nodeTag(route.Node), "interrupt_exist_connections": false})
		}
	}

	dnsServers := []any{map[string]any{
		"type": intent.Bootstrap.Protocol, "tag": "steer-dns-bootstrap", "server": intent.Bootstrap.Server,
		"server_port": intent.Bootstrap.ServerPort,
	}}
	for _, path := range plan.DNSPaths {
		dnsServers = append(dnsServers, compileDNSPath(profiles[path.Profile], routes[path.Route], path))
	}

	routeRules := []any{
		map[string]any{"inbound": dnsInboundTags, "action": "hijack-dns"},
		map[string]any{"inbound": sniffInboundTags, "action": "sniff", "timeout": "300ms"},
	}
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
	ruleSets := make([]any, 0, len(plan.GeoRuleSets))
	for _, ruleSet := range plan.GeoRuleSets {
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

// CompileNodeOutbound exposes the same typed lowering used by the full plan
// for temporary node diagnostics such as speed tests.
func CompileNodeOutbound(node model.Node) map[string]any { return compileNode(node) }

func NodeOutboundTag(id string) string { return nodeTag(id) }

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
