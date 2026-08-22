// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import (
	"strings"
	"testing"

	model "github.com/gsh20040816/openwrt-steer/go/internal/intent"
)

func TestLinuxPlanUsesOnlyWorkstationResources(t *testing.T) {
	plan := NewPlan(model.Intent{})
	if plan.Resources.TunInterface != "steer0" || plan.Resources.DNSPort != 1053 {
		t.Fatalf("unexpected Linux resources: %#v", plan.Resources)
	}
	target := plan.CompilerTarget()
	if len(target.MACBindings) != 0 || len(target.Inbounds) != 2 {
		t.Fatalf("Linux target leaked gateway/MAC resources: %#v", target)
	}
	if target.RequiredCapabilities == nil || strings.Join(target.RequiredCapabilities, ",") != "tun,auto_route,auto_redirect" {
		t.Fatalf("unexpected capability contract: %#v", target.RequiredCapabilities)
	}
}

func TestLinuxFirewallOnlyCapturesLocalDNSOutput(t *testing.T) {
	text := RenderFirewall(NewPlan(model.Intent{}))
	if !strings.Contains(text, "hook output") || strings.Contains(text, "hook prerouting") {
		t.Fatalf("Linux firewall is not output-only:\n%s", text)
	}
	if !strings.Contains(text, "meta mark 0x2024 counter return") || !strings.Contains(text, "th dport 53") {
		t.Fatalf("Linux DNS loop guards are missing:\n%s", text)
	}
}
