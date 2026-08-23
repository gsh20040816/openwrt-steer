// SPDX-License-Identifier: GPL-3.0-or-later

package steermacos

import (
	"encoding/json"
	"path/filepath"
	"testing"

	model "github.com/gsh20040816/steer/go/internal/intent"
	"github.com/gsh20040816/steer/go/pkg/steercore"
)

func TestPrepareJSONReturnsVersionedEnvelope(t *testing.T) {
	value := model.Intent{
		Main:        model.Main{ID: "main", SchemaVersion: model.SchemaVersion, Enabled: true, LogLevel: "warn", ProbeDirectURL: "https://direct.example/", ProbeProxyURL: "https://proxy.example/", SpeedtestProxyURL: "https://speed.example/", DNSCacheCapacity: 4096},
		Bootstrap:   model.Bootstrap{ID: "bootstrap", Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"},
		Routes:      []model.Route{{ID: "direct", Enabled: true, Kind: "direct"}},
		DNSProfiles: []model.DNSProfile{{ID: "dns", Enabled: true, Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"}},
		Rules:       []model.Rule{{ID: "default", Enabled: true, Default: true, DNSProfile: "dns", Route: "direct"}},
	}
	input, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var envelope steercore.Envelope
	if err := json.Unmarshal(PrepareJSON(input, filepath.Join(t.TempDir(), "group")), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Error != nil || envelope.ABIVersion != steercore.ABIVersion {
		t.Fatalf("unexpected macOS prepare envelope: %#v", envelope)
	}
}
