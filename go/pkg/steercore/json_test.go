// SPDX-License-Identifier: GPL-3.0-or-later

package steercore

import (
	"encoding/json"
	"strings"
	"testing"

	model "github.com/gsh20040816/steer/go/internal/intent"
)

func TestValidateJSONUsesVersionedEnvelope(t *testing.T) {
	output := ValidateJSON([]byte(`{"not":"canonical"}`))
	var envelope Envelope
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.ABIVersion != ABIVersion || envelope.OK || envelope.Error == nil || envelope.Error.Code != "INVALID_JSON" {
		t.Fatalf("unexpected ABI envelope: %#v", envelope)
	}
}

func TestCompileJSONKeepsPlatformTargetOutsideCore(t *testing.T) {
	value := minimalIntent()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	target := Target{
		Inbounds:         []any{map[string]any{"type": "direct", "tag": "dns", "listen": "127.0.0.1", "listen_port": 1053}},
		DNSCaptureMode:   "inbound_hijack",
		DNSInboundTags:   []string{"dns"},
		SniffInboundTags: []string{"tun"},
	}
	output := CompileJSON(encoded, "/tmp/steer-state", target)
	var envelope Envelope
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Error != nil || envelope.ABIVersion != ABIVersion {
		t.Fatalf("unexpected compile envelope: %#v", envelope)
	}
	if !strings.Contains(string(envelope.Value), `"action":"hijack-dns"`) {
		t.Fatalf("target DNS capture was not compiled: %s", envelope.Value)
	}
}

func TestCompileJSONReportsCompilerFailure(t *testing.T) {
	value := minimalIntent()
	geoRule := model.Rule{ID: "geo", Enabled: true, DNSProfile: "dns", Route: "direct", DomainMatch: []string{"geosite:cn"}}
	value.Rules = append(value.Rules[:len(value.Rules)-1], geoRule, value.Rules[len(value.Rules)-1])
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	output := CompileJSON(encoded, "", Target{})
	var envelope Envelope
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.OK || envelope.Error == nil || envelope.Error.Code != "COMPILE_FAILED" {
		t.Fatalf("missing explicit compiler failure: %#v", envelope)
	}
}

func minimalIntent() model.Intent {
	return model.Intent{
		Main:        model.Main{ID: "main", SchemaVersion: model.SchemaVersion, Enabled: true, LogLevel: "warn", ProbeDirectURL: "https://direct.example/", ProbeProxyURL: "https://proxy.example/", SpeedtestProxyURL: "https://speed.example/", DNSCacheCapacity: 4096},
		Bootstrap:   model.Bootstrap{ID: "bootstrap", Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"},
		Routes:      []model.Route{{ID: "direct", Enabled: true, Kind: "direct"}},
		DNSProfiles: []model.DNSProfile{{ID: "dns", Enabled: true, Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"}},
		Rules:       []model.Rule{{ID: "default", Enabled: true, Default: true, DNSProfile: "dns", Route: "direct"}},
	}
}
