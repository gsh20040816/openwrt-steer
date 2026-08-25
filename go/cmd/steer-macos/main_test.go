// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	model "github.com/gsh20040816/steer/go/internal/intent"
)

func TestRunVersion(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"version"}, &output, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output.String()) != version {
		t.Fatalf("unexpected version output: %q", output.String())
	}
}

func TestRunRequiresExplicitPaths(t *testing.T) {
	for _, args := range [][]string{{"validate"}, {"compile"}, {"compile", "--config", "config.json"}, {"prepare", "--config", "config.json"}} {
		if err := run(args, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Fatalf("expected explicit path error for %v", args)
		}
	}
}

func TestRunCompileEmitsMacOSTarget(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	file, err := os.Create(configPath)
	if err != nil {
		t.Fatal(err)
	}
	value := model.Intent{
		Main: model.Main{
			ID: "main", SchemaVersion: model.SchemaVersion, Enabled: true, LogLevel: "warn",
			ProbeDirectURL: "https://direct.example/", ProbeProxyURL: "https://proxy.example/",
			SpeedtestProxyURL: "https://speed.example/", DNSCacheCapacity: 4096,
		},
		Bootstrap:   model.Bootstrap{ID: "bootstrap", Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"},
		Routes:      []model.Route{{ID: "direct", Enabled: true, Kind: "direct"}},
		DNSProfiles: []model.DNSProfile{{ID: "dns", Enabled: true, Protocol: "udp", Server: "1.1.1.1", ServerPort: 53}},
		Rules:       []model.Rule{{ID: "default", Enabled: true, Default: true, DNSProfile: "dns", Route: "direct"}},
	}
	if err := model.EncodeJSON(file, value); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run([]string{"compile", "--config", configPath, "--state-dir", filepath.Join(root, "state")}, &output, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"action": "hijack-dns"`, `"port": [
            53
          ]`, `"auto_route": true`} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("compiled macOS output is missing %s:\n%s", expected, output.String())
		}
	}
	if strings.Contains(output.String(), `"auto_redirect"`) {
		t.Fatalf("compiled macOS output contains Linux auto_redirect:\n%s", output.String())
	}
}
