// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gsh20040816/steer/go/pkg/steercore"
)

func TestMacOSBridgeAppliesPlatformValidation(t *testing.T) {
	value := validIntent()
	value.Rules = append(value.Rules, value.Rules[0])
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var envelope steercore.Envelope
	if err := json.Unmarshal(ValidateJSON(encoded), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.OK || envelope.Error == nil || envelope.Error.Code != "VALIDATION_FAILED" {
		t.Fatalf("unexpected macOS validation envelope: %#v", envelope)
	}
}

func TestMacOSBridgeCompilesTUNPort53DNSCapture(t *testing.T) {
	value := validIntent()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var envelope steercore.Envelope
	if err := json.Unmarshal(CompileJSON(encoded, "/tmp/steer-state"), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Error != nil || !strings.Contains(string(envelope.Value), `"action":"hijack-dns"`) || !strings.Contains(string(envelope.Value), `"port":["53"]`) {
		t.Fatalf("unexpected macOS compile envelope: %#v", envelope)
	}
}
