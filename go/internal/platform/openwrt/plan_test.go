// SPDX-License-Identifier: GPL-3.0-or-later

package openwrt

import (
	"encoding/json"
	"strings"
	"testing"

	model "github.com/gsh20040816/steer/go/internal/intent"
)

func TestPlanOwnsOpenWrtResourcesAndCompilerTarget(t *testing.T) {
	value := model.Intent{
		LocalProxies: []model.LocalProxy{{Enabled: true, ListenPort: 20000}},
		Rules:        []model.Rule{{Enabled: true, SourceMACAddress: []string{"02:00:00:00:00:10"}}},
	}
	plan := NewPlan(value)
	if len(plan.Resources.MACBindings) != 1 || plan.Resources.MACMark == 0 {
		t.Fatalf("OpenWrt MAC resources were not planned: %#v", plan.Resources)
	}
	if plan.Resources.MACMark == plan.Resources.AutoRedirectOutputMark {
		t.Fatalf("MAC mark collides with sing-box output mark: %#v", plan.Resources)
	}
	binding := plan.Resources.MACBindings[0]
	if binding.TProxyPort == 20000 || binding.DNSPort == 20000 {
		t.Fatalf("MAC listeners collided with local proxy: %#v", binding)
	}
	target, _ := json.Marshal(plan.CompilerTarget())
	if !strings.Contains(string(target), `"auto_redirect":true`) || !strings.Contains(string(target), `"type":"tproxy"`) {
		t.Fatalf("compiler target omitted OpenWrt inbounds: %s", target)
	}
}
