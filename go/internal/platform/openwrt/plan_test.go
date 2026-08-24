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
	target := plan.CompilerTarget()
	encoded, _ := json.Marshal(target)
	if len(target.Inbounds) != 2 || !strings.Contains(string(encoded), `"auto_redirect":true`) {
		t.Fatalf("compiler target omitted OpenWrt native inbounds: %s", encoded)
	}
	for _, retired := range []string{`"type":"tproxy"`, `"mac_bindings"`, `"steer-mac-"`} {
		if strings.Contains(string(encoded), retired) {
			t.Fatalf("compiler target retained pre-1.14 MAC shim %q: %s", retired, encoded)
		}
	}
}
