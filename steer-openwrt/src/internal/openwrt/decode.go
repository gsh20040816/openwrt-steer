// SPDX-License-Identifier: GPL-3.0-or-later

// Package openwrt contains the OpenWrt-specific UCI adapter. It is the only
// package that knows the public UCI section and option names.
package openwrt

import (
	"fmt"
	"io"
	"strconv"

	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/model"
	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/uci"
)

var optionNames = map[string]map[string]bool{
	"steer":       set("schema_version", "enabled", "log_level", "probe_url", "dns_cache_capacity", "dns_cache_persist", "dns_optimistic_cache"),
	"bootstrap":   set("protocol", "server", "server_port", "strategy"),
	"node":        set("enabled", "name", "type", "server", "server_port", "uuid", "flow", "packet_encoding", "password", "server_ports", "hop_interval", "obfs_type", "obfs_password", "up_mbps", "down_mbps", "tls_server_name", "insecure", "reality_public_key", "reality_short_id", "utls_fingerprint"),
	"route":       set("enabled", "name", "kind", "node"),
	"dns_profile": set("enabled", "name", "protocol", "server", "server_port", "tls_server_name", "path", "insecure", "strategy", "cache_persist", "optimistic_cache"),
	"local_proxy": set("enabled", "name", "protocol", "listen", "listen_port", "username", "password"),
	"rule":        set("enabled", "default", "name", "dns_profile", "route", "inbound", "domain_match", "ip_match", "source_ip_cidr", "source_mac_address", "network", "protocol", "port"),
}

var listNames = map[string]map[string]bool{
	"steer": set("probe_url"),
	"node":  set("server_ports"),
	"rule":  set("inbound", "domain_match", "ip_match", "source_ip_cidr", "source_mac_address", "network", "protocol", "port"),
}

func set(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func Decode(r io.Reader) (model.Intent, error) {
	document, err := uci.Parse(r)
	if err != nil {
		return model.Intent{}, err
	}
	var intent model.Intent
	mainCount, bootstrapCount := 0, 0
	for _, section := range document.Sections {
		known, exists := optionNames[section.Type]
		if !exists {
			return model.Intent{}, fmt.Errorf("UCI line %d: unsupported section type %q", section.Line, section.Type)
		}
		for key := range section.Options {
			if !known[key] {
				return model.Intent{}, fmt.Errorf("UCI line %d: unknown option %q in %s %q", section.Line, key, section.Type, section.ID)
			}
			if listNames[section.Type][key] {
				return model.Intent{}, fmt.Errorf("UCI line %d: %s.%s must use list", section.Line, section.ID, key)
			}
		}
		for key := range section.Lists {
			if !known[key] {
				return model.Intent{}, fmt.Errorf("UCI line %d: unknown list %q in %s %q", section.Line, key, section.Type, section.ID)
			}
			if !listNames[section.Type][key] {
				return model.Intent{}, fmt.Errorf("UCI line %d: %s.%s must use option", section.Line, section.ID, key)
			}
		}

		switch section.Type {
		case "steer":
			mainCount++
			intent.Main, err = decodeMain(section)
		case "bootstrap":
			bootstrapCount++
			intent.Bootstrap, err = decodeBootstrap(section)
		case "node":
			var value model.Node
			value, err = decodeNode(section)
			intent.Nodes = append(intent.Nodes, value)
		case "route":
			var value model.Route
			value, err = decodeRoute(section)
			intent.Routes = append(intent.Routes, value)
		case "dns_profile":
			var value model.DNSProfile
			value, err = decodeDNSProfile(section)
			intent.DNSProfiles = append(intent.DNSProfiles, value)
		case "local_proxy":
			var value model.LocalProxy
			value, err = decodeLocalProxy(section)
			intent.LocalProxies = append(intent.LocalProxies, value)
		case "rule":
			var value model.Rule
			value, err = decodeRule(section)
			intent.Rules = append(intent.Rules, value)
		}
		if err != nil {
			return model.Intent{}, fmt.Errorf("UCI line %d (%s %q): %w", section.Line, section.Type, section.ID, err)
		}
	}
	if mainCount != 1 {
		return model.Intent{}, fmt.Errorf("configuration requires exactly one steer section, found %d", mainCount)
	}
	if bootstrapCount != 1 {
		return model.Intent{}, fmt.Errorf("configuration requires exactly one bootstrap section, found %d", bootstrapCount)
	}
	return intent, nil
}

func decodeMain(s uci.Section) (model.Main, error) {
	schema, err := integer(s, "schema_version", true)
	if err != nil {
		return model.Main{}, err
	}
	enabled, err := boolean(s, "enabled", false)
	if err != nil {
		return model.Main{}, err
	}
	capacity, err := integer(s, "dns_cache_capacity", false)
	if err != nil {
		return model.Main{}, err
	}
	persist, err := boolean(s, "dns_cache_persist", false)
	if err != nil {
		return model.Main{}, err
	}
	optimistic, err := boolean(s, "dns_optimistic_cache", false)
	if err != nil {
		return model.Main{}, err
	}
	probes := clone(s.Lists["probe_url"])
	if len(probes) == 0 {
		probes = []string{"https://www.baidu.com/", "https://www.google.com/generate_204", "https://github.com/"}
	}
	return model.Main{ID: s.ID, SchemaVersion: schema, Enabled: enabled,
		LogLevel: value(s, "log_level", "warn"),
		ProbeURLs: probes, DNSCacheCapacity: capacity, DNSCachePersist: persist,
		DNSOptimisticCache: optimistic}, nil
}

func decodeBootstrap(s uci.Section) (model.Bootstrap, error) {
	port, err := integer(s, "server_port", true)
	if err != nil {
		return model.Bootstrap{}, err
	}
	return model.Bootstrap{ID: s.ID, Protocol: s.Options["protocol"], Server: s.Options["server"], ServerPort: port, Strategy: s.Options["strategy"]}, nil
}

func decodeNode(s uci.Section) (model.Node, error) {
	enabled, err := booleanDefault(s, "enabled", true)
	if err != nil {
		return model.Node{}, err
	}
	port, err := integer(s, "server_port", false)
	if err != nil {
		return model.Node{}, err
	}
	up, err := integer(s, "up_mbps", false)
	if err != nil {
		return model.Node{}, err
	}
	down, err := integer(s, "down_mbps", false)
	if err != nil {
		return model.Node{}, err
	}
	insecure, err := boolean(s, "insecure", false)
	if err != nil {
		return model.Node{}, err
	}
	return model.Node{ID: s.ID, Enabled: enabled, Name: s.Options["name"], Type: s.Options["type"], Server: s.Options["server"], ServerPort: port,
		UUID: s.Options["uuid"], Flow: s.Options["flow"], PacketEncoding: s.Options["packet_encoding"], Password: s.Options["password"],
		ServerPorts: clone(s.Lists["server_ports"]), HopInterval: s.Options["hop_interval"], ObfsType: s.Options["obfs_type"], ObfsPassword: s.Options["obfs_password"],
		UpMbps: up, DownMbps: down, TLSServerName: s.Options["tls_server_name"], Insecure: insecure, RealityPublicKey: s.Options["reality_public_key"],
		RealityShortID: s.Options["reality_short_id"], UTLSFingerprint: s.Options["utls_fingerprint"]}, nil
}

func decodeRoute(s uci.Section) (model.Route, error) {
	enabled, err := booleanDefault(s, "enabled", true)
	if err != nil {
		return model.Route{}, err
	}
	return model.Route{ID: s.ID, Enabled: enabled, Name: s.Options["name"], Kind: s.Options["kind"], Node: s.Options["node"]}, nil
}

func decodeDNSProfile(s uci.Section) (model.DNSProfile, error) {
	enabled, err := booleanDefault(s, "enabled", true)
	if err != nil {
		return model.DNSProfile{}, err
	}
	port, err := integer(s, "server_port", false)
	if err != nil {
		return model.DNSProfile{}, err
	}
	insecure, err := boolean(s, "insecure", false)
	if err != nil {
		return model.DNSProfile{}, err
	}
	persist, err := boolean(s, "cache_persist", false)
	if err != nil {
		return model.DNSProfile{}, err
	}
	optimistic, err := boolean(s, "optimistic_cache", false)
	if err != nil {
		return model.DNSProfile{}, err
	}
	return model.DNSProfile{ID: s.ID, Enabled: enabled, Name: s.Options["name"], Protocol: s.Options["protocol"], Server: s.Options["server"], ServerPort: port,
		TLSServerName: s.Options["tls_server_name"], Path: s.Options["path"], Insecure: insecure, Strategy: value(s, "strategy", "prefer_ipv4"),
		CachePersist: persist, OptimisticCache: optimistic}, nil
}

func decodeLocalProxy(s uci.Section) (model.LocalProxy, error) {
	enabled, err := booleanDefault(s, "enabled", true)
	if err != nil {
		return model.LocalProxy{}, err
	}
	port, err := integer(s, "listen_port", false)
	if err != nil {
		return model.LocalProxy{}, err
	}
	return model.LocalProxy{ID: s.ID, Enabled: enabled, Name: s.Options["name"], Protocol: s.Options["protocol"], Listen: s.Options["listen"], ListenPort: port,
		Username: s.Options["username"], Password: s.Options["password"]}, nil
}

func decodeRule(s uci.Section) (model.Rule, error) {
	enabled, err := booleanDefault(s, "enabled", true)
	if err != nil {
		return model.Rule{}, err
	}
	isDefault, err := boolean(s, "default", false)
	if err != nil {
		return model.Rule{}, err
	}
	ports := make([]int, 0, len(s.Lists["port"]))
	for _, raw := range s.Lists["port"] {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			return model.Rule{}, fmt.Errorf("port must be an integer: %q", raw)
		}
		ports = append(ports, parsed)
	}
	return model.Rule{ID: s.ID, Enabled: enabled, Default: isDefault, Name: s.Options["name"], DNSProfile: s.Options["dns_profile"], Route: s.Options["route"],
		Inbound: clone(s.Lists["inbound"]), DomainMatch: clone(s.Lists["domain_match"]), IPMatch: clone(s.Lists["ip_match"]),
		SourceIPCIDR: clone(s.Lists["source_ip_cidr"]), SourceMACAddress: clone(s.Lists["source_mac_address"]), Network: clone(s.Lists["network"]),
		Protocol: clone(s.Lists["protocol"]), Port: ports}, nil
}

func integer(s uci.Section, key string, required bool) (int, error) {
	raw, exists := s.Options[key]
	if !exists || raw == "" {
		if required {
			return 0, fmt.Errorf("%s is required", key)
		}
		return 0, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %q", key, raw)
	}
	return parsed, nil
}

func boolean(s uci.Section, key string, required bool) (bool, error) {
	if _, exists := s.Options[key]; !exists {
		if required {
			return false, fmt.Errorf("%s is required", key)
		}
		return false, nil
	}
	return booleanDefault(s, key, false)
}

func booleanDefault(s uci.Section, key string, fallback bool) (bool, error) {
	raw, exists := s.Options[key]
	if !exists {
		return fallback, nil
	}
	switch raw {
	case "1":
		return true, nil
	case "0":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be 0 or 1: %q", key, raw)
	}
}

func value(s uci.Section, key, fallback string) string {
	if result, exists := s.Options[key]; exists {
		return result
	}
	return fallback
}

func clone(values []string) []string { return append([]string(nil), values...) }
