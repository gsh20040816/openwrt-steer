// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"testing"

	model "github.com/gsh20040816/steer/go/internal/intent"
)

func TestValidateRejectsSourceMACPolicy(t *testing.T) {
	value := validIntent()
	value.Rules = append([]model.Rule{{
		ID: "mac", Enabled: true, DNSProfile: "dns", Route: "direct",
		SourceMACAddress: []string{"02:00:00:00:00:10"},
	}}, value.Rules...)
	validation := Validate(value)
	if validation.OK {
		t.Fatalf("macOS source-MAC policy was accepted: %#v", validation)
	}
	found := false
	for _, issue := range validation.Errors {
		if issue.Code == "PLATFORM_UNSUPPORTED_SOURCE_MAC" && issue.ObjectID == "mac" {
			found = true
		}
	}
	if !found {
		t.Fatalf("platform error is missing: %#v", validation.Errors)
	}
}

func TestValidateAcceptsCanonicalLocalTrafficIntent(t *testing.T) {
	validation := Validate(validIntent())
	if !validation.OK {
		t.Fatalf("valid macOS intent was rejected: %#v", validation)
	}
}

func TestValidateLeavesGeoToolchainChecksToPrepare(t *testing.T) {
	value := validIntent()
	value.Rules = append(value.Rules[:len(value.Rules)-1], model.Rule{
		ID: "geo", Enabled: true, DNSProfile: "dns", Route: "direct", DomainMatch: []string{"geosite:cn"},
	}, value.Rules[len(value.Rules)-1])
	validation := Validate(value)
	if !validation.OK {
		t.Fatalf("macOS Geo rule should be semantically valid before toolchain checks: %#v", validation)
	}
}

func validIntent() model.Intent {
	return model.Intent{
		Main: model.Main{
			ID: "main", SchemaVersion: model.SchemaVersion, Enabled: true, LogLevel: "warn",
			ProbeDirectURL: "https://direct.example/", ProbeProxyURL: "https://proxy.example/",
			SpeedtestProxyURL: "https://speed.example/", DNSCacheCapacity: 4096,
		},
		Bootstrap:   model.Bootstrap{ID: "bootstrap", Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"},
		Routes:      []model.Route{{ID: "direct", Enabled: true, Kind: "direct"}},
		DNSProfiles: []model.DNSProfile{{ID: "dns", Enabled: true, Protocol: "udp", Server: "1.1.1.1", ServerPort: 53}},
		Rules:       []model.Rule{{ID: "default", Enabled: true, Default: true, DNSProfile: "dns", Route: "direct"}},
	}
}
