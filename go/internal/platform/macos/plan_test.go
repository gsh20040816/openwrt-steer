// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gsh20040816/steer/go/internal/compiler"
	model "github.com/gsh20040816/steer/go/internal/intent"
)

func TestPlanUsesDarwinAutoRouteTUNAndPort53DNSCapture(t *testing.T) {
	plan := NewPlan(model.Intent{})
	target := plan.CompilerTarget()
	if len(target.Inbounds) != 1 || target.DNSCapture.Mode != compiler.DNSCaptureTUNPort53Hijack {
		t.Fatalf("unexpected macOS target: %#v", target)
	}
	if len(target.DNSCapture.InboundTags) != 1 || target.DNSCapture.InboundTags[0] != "steer-tun" {
		t.Fatalf("unexpected DNS inbound tags: %#v", target.DNSCapture)
	}
	tun := target.Inbounds[0].(map[string]any)
	if tun["auto_route"] != true {
		t.Fatal("macOS launchd runtime must let sing-box own auto_route")
	}
	if _, exists := tun["auto_redirect"]; exists {
		t.Fatal("macOS target must not use Linux auto_redirect")
	}
	if len(tun["route_exclude_address"].([]string)) == 0 {
		t.Fatal("macOS TUN must preserve non-global routes")
	}
}

func TestPlanCompilerOutputRetainsDedicatedDNSHijack(t *testing.T) {
	value := model.Intent{
		Main:        model.Main{ID: "main", SchemaVersion: model.SchemaVersion, LogLevel: "warn"},
		Bootstrap:   model.Bootstrap{ID: "bootstrap", Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"},
		Routes:      []model.Route{{ID: "direct", Enabled: true, Kind: "direct"}},
		DNSProfiles: []model.DNSProfile{{ID: "dns", Enabled: true, Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"}},
		Rules:       []model.Rule{{ID: "default", Enabled: true, Default: true, DNSProfile: "dns", Route: "direct"}},
	}
	bundle := compiler.Compile(value, NewPlan(value).CompilerOptions("/tmp/steer-state"))
	encoded, _ := json.Marshal(bundle.SingBox["route"])
	if !strings.Contains(string(encoded), `"action":"hijack-dns"`) || !strings.Contains(string(encoded), `"port":["53"]`) {
		t.Fatalf("macOS TUN DNS capture rule is missing: %s", encoded)
	}
	if strings.Contains(string(encoded), `"auto_redirect"`) {
		t.Fatalf("macOS route unexpectedly contains auto_redirect: %s", encoded)
	}
}
