// SPDX-License-Identifier: GPL-3.0-or-later
package compiler

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/model"
)

func representativeIntent() model.Intent {
	return model.Intent{
		Main:         model.Main{ID: "main", SchemaVersion: 5, Enabled: true, LogLevel: "warn", ProbeURLs: []string{"https://www.baidu.com/", "https://www.google.com/generate_204", "https://github.com/"}, DNSCacheCapacity: 4096},
		Bootstrap:    model.Bootstrap{ID: "bootstrap", Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"},
		Nodes:        []model.Node{{ID: "node", Enabled: true, Type: "vless", Server: "node.example", ServerPort: 443, UUID: "00000000-0000-4000-8000-000000000001", Flow: "xtls-rprx-vision", PacketEncoding: "xudp", TLSServerName: "www.example.com", RealityPublicKey: "fixture", RealityShortID: "0123456789abcdef", UTLSFingerprint: "chrome"}},
		Routes:       []model.Route{{ID: "direct", Enabled: true, Kind: "direct"}, {ID: "proxy", Enabled: true, Kind: "single", Node: "node"}, {ID: "block", Enabled: true, Kind: "block"}},
		DNSProfiles:  []model.DNSProfile{{ID: "public", Enabled: true, Protocol: "https", Server: "1.1.1.1", ServerPort: 443, TLSServerName: "one.one.one.one", Path: "/dns-query", Strategy: "prefer_ipv4"}},
		LocalProxies: []model.LocalProxy{{ID: "local", Enabled: true, Protocol: "mixed", Listen: "127.0.0.1", ListenPort: 1090}},
		Rules: []model.Rule{
			{ID: "mac", Enabled: true, DNSProfile: "public", Route: "direct", SourceMACAddress: []string{"02:00:00:00:00:10"}},
			{ID: "service", Enabled: true, DNSProfile: "public", Route: "proxy", DomainMatch: []string{"domain:example.com", "geosite:category-example"}, Network: []string{"udp"}, Protocol: []string{"quic"}, Port: []int{443}},
			{ID: "local_rule", Enabled: true, DNSProfile: "public", Route: "proxy", Inbound: []string{"local"}},
			{ID: "default", Enabled: true, Default: true, DNSProfile: "public", Route: "direct"},
		},
	}
}

func TestCompilePathIsolationAndProjection(t *testing.T) {
	bundle := Compile(representativeIntent())
	if !bundle.Validation.OK {
		t.Fatalf("compile failed: %#v", bundle.Validation.Errors)
	}
	if len(bundle.Plan.DNSPaths) != 2 {
		t.Fatalf("DNS paths are not isolated by route: %#v", bundle.Plan.DNSPaths)
	}
	dns := bundle.SingBox["dns"].(map[string]any)
	if dns["independent_cache"] != true {
		t.Fatal("sing-box 1.13 independent cache is required")
	}
	dnsRules := dns["rules"].([]any)
	routeRules := bundle.SingBox["route"].(map[string]any)["rules"].([]any)
	encodedDNS, _ := json.Marshal(dnsRules)
	encodedRoute, _ := json.Marshal(routeRules)
	if strings.Contains(string(encodedDNS), `"port":`) || !strings.Contains(string(encodedRoute), `"port":[443]`) {
		t.Fatalf("DNS and Route projections diverged incorrectly\nDNS=%s\nRoute=%s", encodedDNS, encodedRoute)
	}
	if !strings.Contains(string(encodedDNS), "steer-dns-public-via-proxy") {
		t.Fatalf("proxy DNS path missing: %s", encodedDNS)
	}
}

func TestCompileMACShimAndNoForbiddenFeatures(t *testing.T) {
	bundle := Compile(representativeIntent())
	if len(bundle.Plan.Resources.MACBindings) != 1 || bundle.Plan.Resources.MACMark == 0 {
		t.Fatalf("MAC shim not planned: %#v", bundle.Plan.Resources)
	}
	if bundle.Plan.Resources.MACMark == bundle.Plan.Resources.AutoRedirectOutputMark {
		t.Fatalf("global MAC policy mark collides with sing-box output mark: %#v", bundle.Plan.Resources)
	}
	encoded, _ := json.Marshal(bundle.SingBox)
	text := string(encoded)
	for _, forbidden := range []string{"fakeip", "udp/443", "smartdns", "serve_expired"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("forbidden feature %q in %s", forbidden, text)
		}
	}
	if !strings.Contains(text, `"auto_redirect":true`) || !strings.Contains(text, `"auto_route":true`) {
		t.Fatalf("TUN main path missing: %s", text)
	}
	if !strings.Contains(text, `"address":["198.18.0.1/30","fdfe:dcba:9876::1/126"]`) {
		t.Fatalf("unexpected TUN addresses: %s", text)
	}
}

func TestCompileDirectDNSWithoutDetourAndBlockAsReject(t *testing.T) {
	intent := representativeIntent()
	blockRule := model.Rule{ID: "blocked", Enabled: true, DNSProfile: "public", Route: "block", DomainMatch: []string{"domain:blocked.example"}}
	intent.Rules = append(intent.Rules[:len(intent.Rules)-1], blockRule, intent.Rules[len(intent.Rules)-1])
	bundle := Compile(intent)
	if !bundle.Validation.OK {
		t.Fatalf("compile failed: %#v", bundle.Validation.Errors)
	}
	dns := bundle.SingBox["dns"].(map[string]any)
	servers, _ := json.Marshal(dns["servers"])
	if strings.Contains(string(servers), `"tag":"steer-dns-bootstrap","detour"`) || strings.Contains(string(servers), `"tag":"steer-dns-public-via-direct","detour"`) {
		t.Fatalf("direct DNS transport has an invalid direct detour: %s", servers)
	}
	if !strings.Contains(string(servers), `"detour":"steer-route-proxy"`) {
		t.Fatalf("proxy DNS transport lost its fixed route: %s", servers)
	}
	rules, _ := json.Marshal(dns["rules"])
	if !strings.Contains(string(rules), `"domain_suffix":["blocked.example"],"action":"reject"`) && !strings.Contains(string(rules), `"action":"reject","domain_suffix":["blocked.example"]`) {
		t.Fatalf("block DNS projection is not a reject action: %s", rules)
	}
}

func TestCompileIsDeterministic(t *testing.T) {
	first := Compile(representativeIntent())
	second := Compile(representativeIntent())
	if !reflect.DeepEqual(first, second) {
		t.Fatal("same intent produced different bundle")
	}
}
