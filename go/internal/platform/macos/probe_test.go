// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsh20040816/steer/go/internal/compiler"
	model "github.com/gsh20040816/steer/go/internal/intent"
)

type recordingProbeTransport struct {
	urls []string
}

func (transport *recordingProbeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.urls = append(transport.urls, request.URL.String())
	return &http.Response{
		StatusCode: http.StatusNoContent,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    request,
	}, nil
}

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

func TestProbeCurrentUsesOnlyActiveGenerationAndBindsIdentity(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	active := loadProbeTestIntent(t)
	active.Main.Enabled = true
	active.Main.ProbeProxyURL = "https://active.example/proxy"
	active.Main.SpeedtestProxyURL = "https://active.example/download"
	publishProbeTestIntent(t, paths, active)

	saved := active
	saved.Main.ProbeProxyURL = "https://saved.example/proxy"
	saved.Main.SpeedtestProxyURL = "https://saved.example/download"
	writeProbeTestIntent(t, paths.ConfigPath, saved)

	transport := &recordingProbeTransport{}
	result, err := ProbeCurrent(context.Background(), paths.Root, "proxy", &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	activeDigest := compiler.IntentDigest(active)
	if result.GenerationID != activeDigest || result.IntentDigest != activeDigest {
		t.Fatalf("probe identity = %q/%q, want %q", result.GenerationID, result.IntentDigest, activeDigest)
	}
	if result.Report.TestedAt.IsZero() || result.Report.Scope != "overview" || result.Report.Kind != "proxy" || !result.Report.OK {
		t.Fatalf("unexpected active probe report: %#v", result.Report)
	}
	if len(transport.urls) != 1 || transport.urls[0] != active.Main.ProbeProxyURL {
		t.Fatalf("Save-only target affected Active probe: %#v", transport.urls)
	}

	publishProbeTestIntent(t, paths, saved)
	transport.urls = nil
	result, err = ProbeCurrent(context.Background(), paths.Root, "proxy", &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	savedDigest := compiler.IntentDigest(saved)
	if result.GenerationID != savedDigest || result.IntentDigest != savedDigest || result.IntentDigest == activeDigest {
		t.Fatalf("Apply did not switch probe identity: %#v", result)
	}
	if len(transport.urls) != 1 || transport.urls[0] != saved.Main.ProbeProxyURL {
		t.Fatalf("Apply did not switch probe target: %#v", transport.urls)
	}
}

func TestProbeCurrentWithoutActiveGenerationFailsBeforeHTTP(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	saved := loadProbeTestIntent(t)
	saved.Main.Enabled = false
	writeProbeTestIntent(t, paths.ConfigPath, saved)

	for _, kind := range []string{"proxy", "speedtest"} {
		t.Run(kind, func(t *testing.T) {
			transport := &recordingProbeTransport{}
			if _, err := ProbeCurrent(context.Background(), paths.Root, kind, &http.Client{Transport: transport}); err == nil {
				t.Fatal("probe without Active generation unexpectedly succeeded")
			}
			if len(transport.urls) != 0 {
				t.Fatalf("probe without Active generation issued HTTP: %#v", transport.urls)
			}
		})
	}
}

func loadProbeTestIntent(t *testing.T) model.Intent {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "linux", "config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := model.DecodeJSON(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func publishProbeTestIntent(t *testing.T, paths Paths, value model.Intent) {
	t.Helper()
	prepared, err := Prepare(value, paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.Publish(prepared); err != nil {
		t.Fatal(err)
	}
}

func writeProbeTestIntent(t *testing.T, path string, value model.Intent) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := model.EncodeJSON(&encoded, value); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
