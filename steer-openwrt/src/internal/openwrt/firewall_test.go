// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"strings"
	"testing"

	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/compiler"
)

func TestRenderMinimalDNSAndMACShims(t *testing.T) {
	plan := compiler.Plan{Resources: compiler.Resources{DNSPort: 1053, AutoRedirectOutputMark: 0x2024, MACMark: 0x2024, MACTable: 2023, MACPriority: 8999,
		MACBindings: []compiler.MACBinding{{Address: "02:00:00:00:00:10", DNSPort: 20001, TProxyPort: 20000}}}}
	config, err := RenderFirewall(plan, []string{"br-lan", "br-lan"})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"redirect to :1053", "redirect to :20001", "ether saddr 02:00:00:00:00:10", "tproxy to :20000", "meta mark set 0x2024", "udp dport 123"} {
		if !strings.Contains(config, required) {
			t.Fatalf("missing %q:\n%s", required, config)
		}
	}
	if strings.Contains(config, "meta l4proto { tcp, udp } meta mark set") && !strings.Contains(config, "ether saddr") {
		t.Fatal("renderer created a general TPROXY path")
	}
	routes := RenderMACRoutes(plan, []string{"br-lan"})
	if len(routes) != 4 {
		t.Fatalf("unexpected MAC route commands: %#v", routes)
	}
}

func TestRenderRejectsUnsafeDevice(t *testing.T) {
	if _, err := RenderFirewall(compiler.Plan{}, []string{"br-lan;drop"}); err == nil {
		t.Fatal("unsafe device accepted")
	}
}
