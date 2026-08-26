// SPDX-License-Identifier: GPL-3.0-or-later

package probe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectDNSCaptureVerifiesPublishedArtifactsWithoutClaimingTrafficObservation(t *testing.T) {
	root := t.TempDir()
	singBox := filepath.Join(root, "sing-box.json")
	firewall := filepath.Join(root, "firewall.nft")
	if err := os.WriteFile(singBox, []byte(`{"route":{"rules":[{"inbound":["steer-dns"],"action":"hijack-dns"}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(firewall, []byte(`meta l4proto { tcp, udp } th dport 53 redirect to :1053`), 0o600); err != nil {
		t.Fatal(err)
	}
	shim := InspectDNSCapture("dedicated_shim", "generation-a", singBox, firewall)
	if !shim.Configured || shim.ActiveGeneration != "generation-a" {
		t.Fatalf("dedicated shim diagnostic = %#v", shim)
	}

	if err := os.WriteFile(singBox, []byte(`{"route":{"rules":[{"inbound":["steer-tun"],"network":["tcp","udp"],"port":[53],"action":"hijack-dns"}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tun := InspectDNSCapture("tun_port53_hijack", "generation-b", singBox, "")
	if !tun.Configured {
		t.Fatalf("TUN port-53 diagnostic = %#v", tun)
	}

	if err := os.WriteFile(singBox, []byte(`{"route":{"rules":[{"action":"hijack-dns"}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	missingPort := InspectDNSCapture("tun_port53_hijack", "generation-c", singBox, "")
	if missingPort.Configured || missingPort.Detail == "" {
		t.Fatalf("missing destination port 53 was accepted: %#v", missingPort)
	}

	if err := os.WriteFile(singBox, []byte(`{"route":{"rules":[{"inbound":["other"],"action":"hijack-dns"}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	wrongInbound := InspectDNSCapture("dedicated_shim", "generation-d", singBox, firewall)
	if wrongInbound.Configured {
		t.Fatalf("unrelated hijack-dns rule was accepted: %#v", wrongInbound)
	}
}
