// SPDX-License-Identifier: GPL-3.0-or-later
package intent

import (
	"strings"
	"testing"
)

func validIntent() Intent {
	return Intent{
		Main: Main{ID: "main", SchemaVersion: SchemaVersion, Enabled: true, LogLevel: "warn",
			ProbeDirectURL: "https://direct.example/", ProbeProxyURL: "https://proxy.example/", SpeedtestProxyURL: "https://speed.example/"},
		Bootstrap:   Bootstrap{ID: "bootstrap", Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"},
		Nodes:       []Node{{ID: "proxy", Enabled: true, Type: "vless", Server: "proxy.example", ServerPort: 443, NodeCredentials: NodeCredentials{UUID: "00000000-0000-4000-8000-000000000001"}, NodeTransport: NodeTransport{PacketEncoding: "xudp"}}},
		Routes:      []Route{{ID: "direct", Enabled: true, Kind: "direct"}, {ID: "proxy_route", Enabled: true, Kind: "single", Node: "proxy"}, {ID: "block", Enabled: true, Kind: "block"}},
		DNSProfiles: []DNSProfile{{ID: "dns", Enabled: true, Protocol: "https", Server: "1.1.1.1", ServerPort: 443, TLSServerName: "one.one.one.one", Path: "/dns-query"}},
		Rules:       []Rule{{ID: "proxy_rule", Enabled: true, DNSProfile: "dns", Route: "proxy_route", DomainMatch: []string{"domain:example.com"}}, {ID: "default", Enabled: true, Default: true, DNSProfile: "dns", Route: "direct"}},
	}
}

func TestValidateRepresentativeIntent(t *testing.T) {
	validation := Validate(validIntent())
	if !validation.OK {
		t.Fatalf("unexpected validation errors: %#v", validation.Errors)
	}
}

func TestValidateAllowsHTTPSubscriptionURL(t *testing.T) {
	intent := validIntent()
	intent.Subscriptions = []Subscription{{ID: "feed", Enabled: true, URL: "http://192.168.1.2/subscription"}}
	if validation := Validate(intent); !validation.OK {
		t.Fatalf("ordinary HTTP subscription URL was rejected: %#v", validation.Errors)
	}
}

func TestValidateNodeRejectsControlCharacters(t *testing.T) {
	node := validIntent().Nodes[0]
	node.Name = "bad\nname"
	validation := ValidateNode(node)
	if validation.OK || !hasIssue(validation, "CONTROL_CHARACTER") {
		t.Fatalf("node control character was accepted: %#v", validation.Errors)
	}
}

func TestValidateNodeAllowsMultilinePrivateKey(t *testing.T) {
	node := validIntent().Nodes[0]
	node.Type = "ssh"
	node.Server = "ssh.example"
	node.ServerPort = 22
	node.Username = "root"
	node.Password = ""
	node.PrivateKey = "-----BEGIN OPENSSH PRIVATE KEY-----\nkey-material\n-----END OPENSSH PRIVATE KEY-----\n"
	validation := ValidateNode(node)
	if !validation.OK {
		t.Fatalf("multiline private key was rejected: %#v", validation.Errors)
	}
	node.PrivateKey = "key\tmaterial"
	validation = ValidateNode(node)
	if validation.OK || !hasIssue(validation, "CONTROL_CHARACTER") {
		t.Fatalf("tab in private key was accepted: %#v", validation.Errors)
	}
}

func TestValidateRequiresEveryHTTPSProbe(t *testing.T) {
	intent := validIntent()
	intent.Main.ProbeDirectURL = ""
	if validation := Validate(intent); validation.OK || !hasIssue(validation, "REQUIRED_PROBE_URL") {
		t.Fatalf("missing required probe was accepted: %#v", validation.Errors)
	}
}

func TestValidateRouteDetourGraph(t *testing.T) {
	intent := validIntent()
	intent.Routes = append(intent.Routes,
		Route{ID: "front", Enabled: true, Kind: "single", Node: "proxy", Detour: "ingress"},
		Route{ID: "ingress", Enabled: true, Kind: "single", Node: "proxy"},
	)
	intent.Routes[1].Detour = "front"
	if validation := Validate(intent); !validation.OK {
		t.Fatalf("valid route detour chain was rejected: %#v", validation.Errors)
	}

	intent.Routes[4].Detour = "proxy_route"
	validation := Validate(intent)
	if validation.OK || !hasIssue(validation, "ROUTE_DETOUR_CYCLE") {
		t.Fatalf("indirect route detour cycle was accepted: %#v", validation.Errors)
	}
	for _, issue := range validation.Errors {
		if issue.Code == "ROUTE_DETOUR_CYCLE" && !strings.Contains(issue.Message, "proxy_route -> front -> ingress -> proxy_route") {
			t.Fatalf("cycle error omitted the complete path: %#v", issue)
		}
	}
}

func TestValidateRejectsInvalidRouteDetourTargets(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Intent)
		code string
	}{
		{"missing", func(intent *Intent) { intent.Routes[1].Detour = "missing" }, "DANGLING_DETOUR"},
		{"direct", func(intent *Intent) { intent.Routes[1].Detour = "direct" }, "INVALID_DETOUR_KIND"},
		{"disabled", func(intent *Intent) {
			intent.Routes = append(intent.Routes, Route{ID: "disabled_route", Enabled: false, Kind: "single", Node: "proxy"})
			intent.Routes[1].Detour = "disabled_route"
		}, "DISABLED_DETOUR"},
		{"self", func(intent *Intent) { intent.Routes[1].Detour = "proxy_route" }, "ROUTE_DETOUR_CYCLE"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			intent := validIntent()
			testCase.edit(&intent)
			validation := Validate(intent)
			if validation.OK || !hasIssue(validation, testCase.code) {
				t.Fatalf("invalid detour target was accepted: %#v", validation.Errors)
			}
		})
	}
}

func TestValidateRejectsBrokenReferencesAndOldSchema(t *testing.T) {
	intent := validIntent()
	intent.Main.SchemaVersion = 3
	intent.Rules[0].Route = "missing"
	validation := Validate(intent)
	if validation.OK || !hasIssue(validation, "UNSUPPORTED_SCHEMA") || !hasIssue(validation, "DANGLING_ROUTE") {
		t.Fatalf("missing strict failures: %#v", validation.Errors)
	}
}

func TestDisabledObjectsRemainStagingOnly(t *testing.T) {
	intent := validIntent()
	intent.Nodes = append(intent.Nodes, Node{ID: "unfinished", Enabled: false, Type: "unknown"})
	intent.Rules = append(intent.Rules[:1], append([]Rule{{ID: "disabled", Enabled: false, DNSProfile: "missing", Route: "missing"}}, intent.Rules[1:]...)...)
	validation := Validate(intent)
	if !validation.OK {
		t.Fatalf("disabled objects must not enter semantic validation: %#v", validation.Errors)
	}
}

func TestGlobalDNSCacheFeaturesValidateOn114(t *testing.T) {
	intent := validIntent()
	intent.Main.DNSCachePersist = true
	intent.Main.DNSOptimisticCache = true
	validation := Validate(intent)
	if !validation.OK {
		t.Fatalf("unexpected result: %#v", validation)
	}
}

func TestAllFrozenDNSTransportsValidate(t *testing.T) {
	for _, protocol := range []string{"udp", "tcp", "tls", "https", "quic", "h3"} {
		t.Run(protocol, func(t *testing.T) {
			intent := validIntent()
			intent.DNSProfiles[0].Protocol = protocol
			if protocol == "udp" || protocol == "tcp" {
				intent.DNSProfiles[0].TLSServerName = ""
			}
			if protocol != "https" && protocol != "h3" {
				intent.DNSProfiles[0].Path = ""
			}
			if validation := Validate(intent); !validation.OK {
				t.Fatalf("%s rejected: %#v", protocol, validation.Errors)
			}
		})
	}
}

func TestAllSingBox113ProxyOutboundsValidate(t *testing.T) {
	cases := []struct {
		name string
		node Node
	}{
		{"socks", Node{Type: "socks", Server: "proxy.example", ServerPort: 1080}},
		{"http", Node{Type: "http", Server: "proxy.example", ServerPort: 8080}},
		{"shadowsocks", Node{Type: "shadowsocks", Server: "proxy.example", ServerPort: 8388, NodeCredentials: NodeCredentials{Password: "secret"}, NodeProtocol: NodeProtocol{Method: "aes-256-gcm"}}},
		{"vmess", Node{Type: "vmess", Server: "proxy.example", ServerPort: 443, NodeCredentials: NodeCredentials{UUID: "00000000-0000-4000-8000-000000000001"}, NodeProtocol: NodeProtocol{Security: "auto"}}},
		{"hysteria", Node{Type: "hysteria", Server: "proxy.example", ServerPort: 443, NodeCredentials: NodeCredentials{Password: "secret"}, NodeProtocol: NodeProtocol{UpMbps: 100, DownMbps: 100}, NodeTLS: NodeTLS{TLSServerName: "proxy.example"}}},
		{"shadowtls", Node{Type: "shadowtls", Server: "proxy.example", ServerPort: 443, NodeCredentials: NodeCredentials{Password: "secret"}, NodeProtocol: NodeProtocol{Version: 3}, NodeTLS: NodeTLS{TLSServerName: "proxy.example"}}},
		{"tuic", Node{Type: "tuic", Server: "proxy.example", ServerPort: 443, NodeCredentials: NodeCredentials{UUID: "00000000-0000-4000-8000-000000000002", Password: "secret"}, NodeTLS: NodeTLS{TLSServerName: "proxy.example"}}},
		{"anytls", Node{Type: "anytls", Server: "proxy.example", ServerPort: 443, NodeCredentials: NodeCredentials{Password: "secret"}, NodeTLS: NodeTLS{TLSServerName: "proxy.example"}}},
		{"naive", Node{Type: "naive", Server: "proxy.example", ServerPort: 443, NodeCredentials: NodeCredentials{Username: "user", Password: "secret"}, NodeTLS: NodeTLS{TLSServerName: "proxy.example"}}},
		{"ssh", Node{Type: "ssh", Server: "proxy.example", ServerPort: 22, NodeCredentials: NodeCredentials{Username: "root", Password: "secret"}}},
		{"tor", Node{Type: "tor", NodeProtocol: NodeProtocol{ExecutablePath: "/usr/bin/tor"}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			intent := validIntent()
			testCase.node.ID, testCase.node.Enabled = "proxy", true
			intent.Nodes[0] = testCase.node
			if validation := Validate(intent); !validation.OK {
				t.Fatalf("%s rejected: %#v", testCase.name, validation.Errors)
			}
		})
	}
}

func TestRejectVMessTransportInOutboundNetwork(t *testing.T) {
	intent := validIntent()
	intent.Nodes[0] = Node{ID: "proxy", Enabled: true, Type: "vmess", Server: "proxy.example", ServerPort: 443,
		NodeCredentials: NodeCredentials{UUID: "00000000-0000-4000-8000-000000000001"},
		NodeTransport:   NodeTransport{Network: "ws", Transport: "ws", TransportPath: "/ws"},
		NodeProtocol:    NodeProtocol{Security: "auto"}}
	validation := Validate(intent)
	if validation.OK || !hasIssue(validation, "INVALID_NODE_NETWORK") {
		t.Fatalf("VMess transport leaked into outbound network without rejection: %#v", validation.Errors)
	}
}

func TestRejectVLESSURISecurityLeakingIntoCanonicalModel(t *testing.T) {
	intent := validIntent()
	intent.Nodes[0] = Node{ID: "proxy", Enabled: true, Type: "vless", Server: "proxy.example", ServerPort: 443,
		NodeCredentials: NodeCredentials{UUID: "00000000-0000-4000-8000-000000000001"},
		NodeProtocol:    NodeProtocol{Security: "tls"},
		NodeTLS:         NodeTLS{TLSServerName: "proxy.example"}}
	validation := Validate(intent)
	if validation.OK || !hasIssue(validation, "UNSUPPORTED_NODE_OPTION") {
		t.Fatalf("VLESS URI security leaked into canonical model: %#v", validation.Errors)
	}
}

func TestPinnedStaleSubscriptionNodeWarns(t *testing.T) {
	intent := validIntent()
	intent.Nodes[0].PinnedStale = true
	validation := Validate(intent)
	if !validation.OK || !hasWarning(validation, "SUBSCRIPTION_NODE_STALE") {
		t.Fatalf("stale subscription node must remain usable but warn: %#v", validation)
	}
}

func hasIssue(validation Validation, code string) bool {
	for _, issue := range validation.Errors {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func hasWarning(validation Validation, code string) bool {
	for _, issue := range validation.Warnings {
		if issue.Code == code {
			return true
		}
	}
	return false
}
