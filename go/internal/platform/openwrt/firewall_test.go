// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"strings"
	"testing"
)

func TestRenderMinimalDNSAndMACShims(t *testing.T) {
	plan := Plan{Resources: Resources{DNSPort: 1053, AutoRedirectOutputMark: 0x2024, MACMark: 0x2026, MACTable: 2023, MACPriority: 8999,
		MACBindings: []MACBinding{{Address: "02:00:00:00:00:10", DNSPort: 20001, TProxyPort: 20000}}}}
	config, err := RenderFirewall(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"redirect to :1053", "redirect to :20001", "dnat ip to 127.0.0.1:1053", "dnat ip6 to [::1]:1053", "snat ip to 127.0.0.1", "snat ip6 to ::1", "ether saddr 02:00:00:00:00:10", "tproxy to :20000", "meta mark set 0x2026", "udp dport 123"} {
		if !strings.Contains(config, required) {
			t.Fatalf("missing %q:\n%s", required, config)
		}
	}
	if strings.Contains(config, "meta l4proto { tcp, udp } meta mark set") && !strings.Contains(config, "ether saddr") {
		t.Fatal("renderer created a general TPROXY path")
	}
	routes := RenderMACRoutes(plan)
	if len(routes) != 4 {
		t.Fatalf("unexpected MAC route commands: %#v", routes)
	}
	for _, route := range routes {
		if strings.Contains(strings.Join(route.Args, " "), " iif ") {
			t.Fatalf("MAC route unexpectedly scoped to an interface: %#v", route)
		}
	}
	if strings.Contains(config, "managed_devices") || strings.Contains(config, "iifname @") {
		t.Fatalf("firewall still contains managed-zone scoping:\n%s", config)
	}
}

func TestRenderFirewallDoesNotRequireResolvedDevices(t *testing.T) {
	if _, err := RenderFirewall(Plan{}); err != nil {
		t.Fatal(err)
	}
}
