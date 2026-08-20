// SPDX-License-Identifier: GPL-3.0-or-later
package model

import "testing"

func validIntent() Intent {
	return Intent{
		Main:        Main{ID: "main", SchemaVersion: SchemaVersion, Enabled: true, ManagedZones: []string{"lan"}, LogLevel: "warn", ProbeURLs: []string{"https://example.com/"}},
		Bootstrap:   Bootstrap{ID: "bootstrap", Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"},
		Nodes:       []Node{{ID: "proxy", Enabled: true, Type: "vless", Server: "proxy.example", ServerPort: 443, UUID: "00000000-0000-4000-8000-000000000001", PacketEncoding: "xudp"}},
		Routes:      []Route{{ID: "direct", Enabled: true, Kind: "direct"}, {ID: "proxy_route", Enabled: true, Kind: "single", Node: "proxy"}, {ID: "block", Enabled: true, Kind: "block"}},
		DNSProfiles: []DNSProfile{{ID: "dns", Enabled: true, Protocol: "https", Server: "1.1.1.1", ServerPort: 443, TLSServerName: "one.one.one.one", Path: "/dns-query", Strategy: "prefer_ipv4"}},
		Rules:       []Rule{{ID: "proxy_rule", Enabled: true, DNSProfile: "dns", Route: "proxy_route", DomainMatch: []string{"domain:example.com"}}, {ID: "default", Enabled: true, Default: true, DNSProfile: "dns", Route: "direct"}},
	}
}

func TestValidateRepresentativeIntent(t *testing.T) {
	validation := Validate(validIntent())
	if !validation.OK {
		t.Fatalf("unexpected validation errors: %#v", validation.Errors)
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

func TestFutureDNSCacheFeaturesFailOn113(t *testing.T) {
	intent := validIntent()
	intent.DNSProfiles[0].OptimisticCache = true
	validation := Validate(intent)
	if validation.OK || !hasIssue(validation, "REQUIRES_SING_BOX_1_14") {
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

func hasIssue(validation Validation, code string) bool {
	for _, issue := range validation.Errors {
		if issue.Code == code {
			return true
		}
	}
	return false
}
