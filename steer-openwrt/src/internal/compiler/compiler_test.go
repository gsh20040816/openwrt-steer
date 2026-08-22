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
		Main: model.Main{ID: "main", SchemaVersion: model.SchemaVersion, Enabled: true, LogLevel: "warn",
			ProbeDirectURL: "https://www.baidu.com/", ProbeProxyURL: "https://www.google.com/generate_204", SpeedtestProxyURL: "https://speed.cloudflare.com/__down?bytes=1000000", DNSCacheCapacity: 4096},
		Bootstrap:    model.Bootstrap{ID: "bootstrap", Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"},
		Nodes:        []model.Node{{ID: "node", Enabled: true, Type: "vless", Server: "node.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{UUID: "00000000-0000-4000-8000-000000000001"}, NodeTransport: model.NodeTransport{Flow: "xtls-rprx-vision", PacketEncoding: "xudp"}, NodeTLS: model.NodeTLS{TLSServerName: "www.example.com", RealityPublicKey: "fixture", RealityShortID: "0123456789abcdef", UTLSFingerprint: "chrome"}}},
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

func TestCompileRoutePrivateOutboundsAndDetour(t *testing.T) {
	intent := representativeIntent()
	intent.Routes = append(intent.Routes, model.Route{ID: "chained", Enabled: true, Kind: "single", Node: "node", Detour: "proxy"})
	bundle := Compile(intent)
	if !bundle.Validation.OK {
		t.Fatalf("compile failed: %#v", bundle.Validation.Errors)
	}
	outbounds := bundle.SingBox["outbounds"].([]any)
	var proxy, chained map[string]any
	for _, raw := range outbounds {
		outbound := raw.(map[string]any)
		switch outbound["tag"] {
		case "steer-route-proxy":
			proxy = outbound
		case "steer-route-chained":
			chained = outbound
		}
	}
	if proxy == nil || chained == nil {
		t.Fatalf("route-private outbounds are missing: %#v", outbounds)
	}
	if proxy["type"] != "vless" || proxy["detour"] != nil {
		t.Fatalf("base route outbound is wrong: %#v", proxy)
	}
	if chained["type"] != "vless" || chained["detour"] != "steer-route-proxy" {
		t.Fatalf("chained route outbound lost detour: %#v", chained)
	}
	encoded, _ := json.Marshal(outbounds)
	if strings.Contains(string(encoded), "steer-node-node") || strings.Contains(string(encoded), `"type":"selector"`) {
		t.Fatalf("legacy global node selector leaked into route-private compilation: %s", encoded)
	}
	chain := CompileRouteChainOutbounds(intent, "chained")
	chainJSON, _ := json.Marshal(chain)
	if len(chain) != 2 || !strings.Contains(string(chainJSON), `"tag":"steer-route-proxy"`) || !strings.Contains(string(chainJSON), `"tag":"steer-route-chained"`) {
		t.Fatalf("route test did not retain the complete target chain: %s", chainJSON)
	}
	if strings.Contains(string(chainJSON), `"tag":"steer-route-direct"`) || strings.Contains(string(chainJSON), `"tag":"steer-route-block"`) {
		t.Fatalf("route test included unrelated outbounds: %s", chainJSON)
	}
}

func TestPlanDiffJSONUsesArraysForEmptyCategories(t *testing.T) {
	current := Compile(representativeIntent()).Plan
	candidate := current
	candidate.Objects = append(candidate.Objects, PlanObject{Type: "node", ID: "new", Digest: "digest"})

	diff := Diff(current, candidate)
	encoded, err := json.Marshal(diff)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if !strings.Contains(text, `"added":[`) || !strings.Contains(text, `"removed":[]`) || !strings.Contains(text, `"modified":[]`) {
		t.Fatalf("plan diff must encode every object category as an array: %s", text)
	}
}

func TestPlanDecodesSchemaSixProbeArraysForRuntimeCleanup(t *testing.T) {
	var plan Plan
	legacy := `{
		"schema_version": 6,
		"resources": {"dns_port": 1053, "mac_bindings": []},
		"probe_direct": ["https://first.example/", "https://second.example/"],
		"probe_proxy": ["https://proxy.example/"],
		"speedtest_proxy": ["https://speed.example/"]
	}`
	if err := json.Unmarshal([]byte(legacy), &plan); err != nil {
		t.Fatalf("schema 7 binary cannot decode the schema 6 runtime plan: %v", err)
	}
	if plan.SchemaVersion != 6 || plan.Resources.DNSPort != 1053 || plan.ProbeDirect != "https://first.example/" || plan.ProbeProxy != "https://proxy.example/" || plan.SpeedtestProxy != "https://speed.example/" {
		t.Fatalf("legacy runtime plan was decoded incorrectly: %#v", plan)
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"probe_direct":[`) || !strings.Contains(string(encoded), `"probe_direct":"https://first.example/"`) {
		t.Fatalf("legacy plan did not marshal back as schema 7 scalars: %s", encoded)
	}
}

func TestCompileSingBoxProxyNodeFamilies(t *testing.T) {
	base := representativeIntent()
	base.Nodes = []model.Node{
		{ID: "ss", Enabled: true, Type: "shadowsocks", Server: "ss.example", ServerPort: 8388, NodeCredentials: model.NodeCredentials{Password: "secret"}, NodeProtocol: model.NodeProtocol{Method: "2022-blake3-aes-128-gcm"}},
		{ID: "vmess", Enabled: true, Type: "vmess", Server: "vmess.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{UUID: "00000000-0000-4000-8000-000000000001"}, NodeProtocol: model.NodeProtocol{Security: "auto"}, NodeTransport: model.NodeTransport{Transport: "ws", TransportPath: "/ws"}, NodeTLS: model.NodeTLS{TLSServerName: "vmess.example"}},
		{ID: "tuic", Enabled: true, Type: "tuic", Server: "tuic.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{UUID: "00000000-0000-4000-8000-000000000002", Password: "secret"}, NodeTLS: model.NodeTLS{TLSServerName: "tuic.example"}},
		{ID: "ssh", Enabled: true, Type: "ssh", Server: "ssh.example", ServerPort: 22, NodeCredentials: model.NodeCredentials{Username: "root", Password: "secret"}},
	}
	base.Routes[1].Node = "ss"
	base.Routes = append(base.Routes,
		model.Route{ID: "vmess_route", Enabled: true, Kind: "single", Node: "vmess"},
		model.Route{ID: "tuic_route", Enabled: true, Kind: "single", Node: "tuic"},
		model.Route{ID: "ssh_route", Enabled: true, Kind: "single", Node: "ssh"},
	)
	bundle := Compile(base)
	if !bundle.Validation.OK {
		t.Fatalf("proxy family validation failed: %#v", bundle.Validation.Errors)
	}
	encoded, _ := json.Marshal(bundle.SingBox["outbounds"])
	for _, expected := range []string{`"type":"shadowsocks"`, `"type":"vmess"`, `"type":"tuic"`, `"type":"ssh"`} {
		if !strings.Contains(string(encoded), expected) {
			t.Fatalf("compiled outbounds missing %s: %s", expected, encoded)
		}
	}
}
