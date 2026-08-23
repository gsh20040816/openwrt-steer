// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gsh20040816/steer/go/internal/compiler"
	model "github.com/gsh20040816/steer/go/internal/intent"
)

func TestPlanUsesNetworkExtensionOwnedTUNAndDNSProxyInbound(t *testing.T) {
	plan := NewPlan(model.Intent{})
	target := plan.CompilerTarget()
	if len(target.Inbounds) != 3 || target.DNSCapture.Mode != compiler.DNSCaptureInboundHijack {
		t.Fatalf("unexpected macOS target: %#v", target)
	}
	if len(target.DNSCapture.InboundTags) != 2 {
		t.Fatalf("unexpected DNS inbound tags: %#v", target.DNSCapture)
	}
	tun := target.Inbounds[0].(map[string]any)
	if _, exists := tun["auto_route"]; exists {
		t.Fatal("macOS target delegates routes to NetworkExtension and must not set auto_route")
	}
	if _, exists := tun["auto_redirect"]; exists {
		t.Fatal("macOS target must not use Linux auto_redirect")
	}
	dns4 := target.Inbounds[1].(map[string]any)
	dns6 := target.Inbounds[2].(map[string]any)
	if dns4["listen"] != "127.0.0.1" || dns4["listen_port"] != DNSPort {
		t.Fatalf("unexpected IPv4 DNS listener: %#v", dns4)
	}
	if dns6["listen"] != "::1" || dns6["listen_port"] != DNSPort6 {
		t.Fatalf("unexpected IPv6 DNS listener: %#v", dns6)
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
	if !strings.Contains(string(encoded), `"action":"hijack-dns"`) {
		t.Fatalf("dedicated macOS DNS inbound lost hijack-dns route: %s", encoded)
	}
	if strings.Contains(string(encoded), `"auto_redirect"`) {
		t.Fatalf("macOS route unexpectedly contains auto_redirect: %s", encoded)
	}
}
