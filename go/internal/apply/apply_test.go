// SPDX-License-Identifier: GPL-3.0-or-later

package apply

import (
	"context"
	"reflect"
	"testing"

	"github.com/gsh20040816/steer/go/internal/compiler"
	"github.com/gsh20040816/steer/go/internal/generation"
	model "github.com/gsh20040816/steer/go/internal/intent"
)

type recordingBackend struct{ calls []string }

func (backend *recordingBackend) Prepare(context.Context, model.Intent, compiler.Output) (generation.Candidate, error) {
	backend.calls = append(backend.calls, "prepare")
	return generation.Candidate{Directory: "/candidate"}, nil
}
func (backend *recordingBackend) Activate(context.Context, generation.Candidate) error {
	backend.calls = append(backend.calls, "activate")
	return nil
}
func (backend *recordingBackend) Healthy(context.Context, generation.Candidate) error {
	backend.calls = append(backend.calls, "healthy")
	return nil
}
func (backend *recordingBackend) Finalize(context.Context, generation.Candidate) error {
	backend.calls = append(backend.calls, "finalize")
	return nil
}
func (backend *recordingBackend) Disable(context.Context) error {
	backend.calls = append(backend.calls, "disable")
	return nil
}

func TestRunUsesFixedLifecycle(t *testing.T) {
	backend := &recordingBackend{}
	value := validIntent(true)
	result, err := Run(context.Background(), value, compiler.Options{}, backend)
	if err != nil || !result.OK || result.Generation != "/candidate" {
		t.Fatalf("unexpected Apply result: %#v %v", result, err)
	}
	if !reflect.DeepEqual(backend.calls, []string{"prepare", "activate", "healthy", "finalize"}) {
		t.Fatalf("unexpected lifecycle: %#v", backend.calls)
	}
}

func TestRunDisablesWithoutPreparingGeneration(t *testing.T) {
	backend := &recordingBackend{}
	result, err := Run(context.Background(), validIntent(false), compiler.Options{}, backend)
	if err != nil || !result.OK || !reflect.DeepEqual(backend.calls, []string{"disable"}) {
		t.Fatalf("unexpected disabled Apply: %#v %v %#v", result, err, backend.calls)
	}
}

func validIntent(enabled bool) model.Intent {
	return model.Intent{
		Main:        model.Main{ID: "main", SchemaVersion: model.SchemaVersion, Enabled: enabled, LogLevel: "warn", ProbeDirectURL: "https://direct.example/", ProbeProxyURL: "https://proxy.example/", SpeedtestProxyURL: "https://speed.example/"},
		Bootstrap:   model.Bootstrap{ID: "bootstrap", Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"},
		Routes:      []model.Route{{ID: "direct", Enabled: true, Kind: "direct"}},
		DNSProfiles: []model.DNSProfile{{ID: "dns", Enabled: true, Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"}},
		Rules:       []model.Rule{{ID: "default", Enabled: true, Default: true, DNSProfile: "dns", Route: "direct"}},
	}
}
