// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gsh20040816/steer/go/internal/compiler"
	model "github.com/gsh20040816/steer/go/internal/intent"
)

func TestProbeOverviewUsesSavedIntentWithoutActiveGeneration(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "linux", "config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := model.DecodeJSON(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	value.Main.Enabled = false
	value.Main.ProbeProxyURL = server.URL

	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	var encoded bytes.Buffer
	if err := model.EncodeJSON(&encoded, value); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := ProbeOverviewWithState(
		context.Background(), configPath, filepath.Join(root, "run"), filepath.Join(root, "state"), "proxy", server.Client(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK || report.SavedDigest != compiler.IntentDigest(value) || report.ActiveGeneration != "" {
		t.Fatalf("disabled Saved overview probe = %#v", report)
	}
}

func TestTemporaryProbePreservesBootstrapAndMarksEveryDialSocket(t *testing.T) {
	bootstrap := model.Bootstrap{Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"}
	config, err := temporaryProbeConfig(bootstrap, []any{map[string]any{"type": "direct", "tag": "route"}}, "route", 12345)
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
	if dns["type"] != bootstrap.Protocol || dns["server"] != bootstrap.Server || dns["server_port"] != bootstrap.ServerPort {
		t.Fatalf("temporary DNS does not preserve bootstrap: %#v", dns)
	}
	resolver := config["route"].(map[string]any)["default_domain_resolver"].(map[string]any)
	if resolver["server"] != "steer-dns-bootstrap" || resolver["strategy"] != bootstrap.Strategy {
		t.Fatalf("temporary default resolver does not preserve bootstrap strategy: %#v", resolver)
	}
}
