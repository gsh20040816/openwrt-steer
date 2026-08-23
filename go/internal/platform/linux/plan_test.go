// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import (
	"strings"
	"testing"

	model "github.com/gsh20040816/steer/go/internal/intent"
)

func TestLinuxPlanCapturesHostAndForwardedTraffic(t *testing.T) {
	plan := NewPlan(model.Intent{})
	if plan.Resources.TunInterface != "steer0" || plan.Resources.DNSPort != 1053 || plan.Resources.DNSPort6 != 1054 {
		t.Fatalf("unexpected Linux resources: %#v", plan.Resources)
	}
	target := plan.CompilerTarget()
	if len(target.MACBindings) != 0 || len(target.Inbounds) != 3 || len(target.DNSInboundTags) != 2 {
		t.Fatalf("Linux target has unexpected resources: %#v", target)
	}
	dns4 := target.Inbounds[1].(map[string]any)
	dns6 := target.Inbounds[2].(map[string]any)
	if dns4["listen"] != "0.0.0.0" || dns4["listen_port"] != 1053 || dns6["listen"] != "::" || dns6["listen_port"] != 1054 {
		t.Fatalf("Linux DNS listeners do not cover redirected IPv4 and IPv6 traffic: %#v %#v", dns4, dns6)
	}
	tun := target.Inbounds[0].(map[string]any)
	if _, restricted := tun["include_interface"]; restricted {
		t.Fatal("Linux TUN unexpectedly restricts interception to a host interface")
	}
	if target.RequiredCapabilities == nil || strings.Join(target.RequiredCapabilities, ",") != "tun,auto_route,auto_redirect" {
		t.Fatalf("unexpected capability contract: %#v", target.RequiredCapabilities)
	}
}

func TestLinuxFirewallCapturesHostAndForwardedDNS(t *testing.T) {
	text := RenderFirewall(NewPlan(model.Intent{}))
	if !strings.Contains(text, "hook output") || !strings.Contains(text, "hook prerouting") {
		t.Fatalf("Linux firewall does not cover host and forwarded DNS:\n%s", text)
	}
	for _, required := range []string{
		"meta nfproto ipv4 meta l4proto { tcp, udp } th dport 53 counter redirect to :1053",
		"meta nfproto ipv6 meta l4proto { tcp, udp } th dport 53 counter redirect to :1054",
		"iifname \"steer0\" return",
		"meta mark 0x2024 counter return",
		"th dport { 1053, 1054 } ct status dnat counter accept",
		"th dport { 1053, 1054 } counter reject",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("Linux DNS firewall is missing %q:\n%s", required, text)
		}
	}
}
