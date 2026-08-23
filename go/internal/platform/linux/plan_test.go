// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import (
	"strings"
	"testing"

	model "github.com/gsh20040816/openwrt-steer/go/internal/intent"
)

func TestLinuxPlanCapturesHostAndForwardedTraffic(t *testing.T) {
	plan := NewPlan(model.Intent{})
	if plan.Resources.TunInterface != "steer0" || plan.Resources.DNSPort != 1053 {
		t.Fatalf("unexpected Linux resources: %#v", plan.Resources)
	}
	target := plan.CompilerTarget()
	if len(target.MACBindings) != 0 || len(target.Inbounds) != 3 || len(target.DNSInboundTags) != 2 {
		t.Fatalf("Linux target has unexpected resources: %#v", target)
	}
	dns4 := target.Inbounds[1].(map[string]any)
	dns6 := target.Inbounds[2].(map[string]any)
	if dns4["listen"] != "127.0.0.1" || dns6["listen"] != "::1" {
		t.Fatalf("Linux DNS listeners are not explicit loopback sockets: %#v %#v", dns4, dns6)
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
	if !strings.Contains(text, "redirect to :1053") || !strings.Contains(text, "iifname \"steer0\" return") || !strings.Contains(text, "meta mark 0x2024 counter return") || !strings.Contains(text, "th dport 53") {
		t.Fatalf("Linux DNS loop guards are missing:\n%s", text)
	}
}
