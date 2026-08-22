// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import "testing"

func TestTemporaryProbeMarksEveryOutboundAndDNS(t *testing.T) {
	config, err := temporaryProbeConfig([]any{map[string]any{"type": "direct", "tag": "route"}}, "route", 12345)
	if err != nil {
		t.Fatal(err)
	}
	outbound := config["outbounds"].([]any)[0].(map[string]any)
	if outbound["routing_mark"] != AutoRedirectOutputMark {
		t.Fatalf("temporary outbound is not bypass-marked: %#v", outbound)
	}
	dns := config["dns"].(map[string]any)["servers"].([]any)[0].(map[string]any)
	if dns["routing_mark"] != AutoRedirectOutputMark {
		t.Fatalf("temporary DNS is not bypass-marked: %#v", dns)
	}
}
