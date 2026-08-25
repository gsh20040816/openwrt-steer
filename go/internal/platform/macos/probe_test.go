// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"testing"

	model "github.com/gsh20040816/steer/go/internal/intent"
)

func TestTemporaryProbeConfigUsesSharedBootstrapAndRoute(t *testing.T) {
	config := temporaryProbeConfig(
		model.Bootstrap{Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"},
		[]any{map[string]any{"type": "direct", "tag": "route"}},
		"route",
		12345,
	)
	route := config["route"].(map[string]any)
	if route["final"] != "route" || route["auto_detect_interface"] != true {
		t.Fatalf("unexpected temporary route: %#v", route)
	}
	inbound := config["inbounds"].([]any)[0].(map[string]any)
	if inbound["listen_port"] != 12345 || inbound["type"] != "mixed" {
		t.Fatalf("unexpected temporary inbound: %#v", inbound)
	}
}
