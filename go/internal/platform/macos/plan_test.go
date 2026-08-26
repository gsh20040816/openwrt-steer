// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"encoding/json"
	"reflect"
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
	if tun["dns_mode"] != "hijack" {
		t.Fatal("macOS TUN must install the native Apple DNS path instead of leaving LAN resolvers outside Steer")
	}
	if _, exists := tun["auto_redirect"]; exists {
		t.Fatal("macOS target must not use Linux auto_redirect")
	}
	if len(tun["route_exclude_address"].([]string)) == 0 {
		t.Fatal("macOS TUN must preserve non-global routes")
	}
	excluded := tun["route_exclude_address"].([]string)
	for _, prefix := range excluded {
		if prefix == "198.18.0.0/15" {
			t.Fatal("macOS must not exclude the IPv4 subnet containing the system-stack peer")
		}
	}
	for _, lan := range []string{"10.0.0.0/8", "100.64.0.0/10", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"} {
		if !contains(excluded, lan) {
			t.Fatalf("non-DNS LAN traffic would enter the proxy core because %s is not excluded", lan)
		}
	}
}

func TestPlanCompilerOutputRetainsDedicatedDNSHijack(t *testing.T) {
	value := model.Intent{
		Main:        model.Main{ID: "main", SchemaVersion: model.SchemaVersion, LogLevel: "warn"},
		Bootstrap:   model.Bootstrap{ID: "bootstrap", Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"},
		Routes:      []model.Route{{ID: "direct", Enabled: true, Kind: "direct"}},
		DNSProfiles: []model.DNSProfile{{ID: "dns", Enabled: true, Protocol: "udp", Server: "1.1.1.1", ServerPort: 53}},
		Rules:       []model.Rule{{ID: "default", Enabled: true, Default: true, DNSProfile: "dns", Route: "direct"}},
	}
	bundle := compiler.Compile(value, NewPlan(value).CompilerOptions("/tmp/steer-state"))
	encoded, _ := json.Marshal(bundle.SingBox["route"])
	if !strings.Contains(string(encoded), `"action":"hijack-dns"`) || !strings.Contains(string(encoded), `"port":[53]`) {
		t.Fatalf("macOS TUN DNS capture rule is missing: %s", encoded)
	}
	if strings.Contains(string(encoded), `"auto_redirect"`) {
		t.Fatalf("macOS route unexpectedly contains auto_redirect: %s", encoded)
	}
	rules := bundle.SingBox["route"].(map[string]any)["rules"].([]any)
	if len(rules) < 2 {
		t.Fatalf("macOS route rules are incomplete: %#v", rules)
	}
	first := rules[0].(map[string]any)
	if first["action"] != "hijack-dns" || !reflect.DeepEqual(first["port"], []uint16{53}) || !reflect.DeepEqual(first["network"], []string{"tcp", "udp"}) {
		t.Fatalf("macOS DNS hijack must match TCP/UDP destination port 53 exactly: %#v", first)
	}
	if _, exists := first["source_port"]; exists {
		t.Fatalf("macOS DNS hijack accidentally matches the reusable UDP source port: %#v", first)
	}
	if rules[1].(map[string]any)["action"] != "sniff" {
		t.Fatalf("ordinary TUN traffic must remain outside DNS hijack: %#v", rules)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
