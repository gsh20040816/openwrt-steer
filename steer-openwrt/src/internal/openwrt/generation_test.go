// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type prepareRunner struct {
	calls     []string
	failCheck bool
}

func (runner *prepareRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	runner.calls = append(runner.calls, call)
	switch {
	case strings.HasSuffix(call, "sing-box version"):
		return []byte("sing-box version 1.13.18\nTags: with_quic,with_utls\n"), nil
	case call == "ubus call network.interface.lan status":
		return []byte(`{"up":true,"l3_device":"br-lan"}`), nil
	case call == "ip -json -4 route show default", call == "ip -json -6 route show default":
		return []byte(`[{"dev":"wan"}]`), nil
	case strings.Contains(call, "sing-box check -c") || strings.Contains(call, "nft -c -f"):
		if runner.failCheck {
			return nil, fmt.Errorf("injected native check failure")
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected command: %s", call)
	}
}

func TestPrepareGenerationPerformsAllPreMutationChecks(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "steer")
	if err := os.WriteFile(configPath, []byte(minimalConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &prepareRunner{}
	generation, err := PrepareGeneration(context.Background(), runner, PrepareOptions{ConfigPath: configPath, RunDirectory: filepath.Join(root, "run"), StateDirectory: filepath.Join(root, "state"), SingBoxBinary: "/test/sing-box", NFTBinary: "/test/nft", FirewallConfig: writeFirewallConfig(t, root)})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"steer.uci", "bundle.json", "plan.json", "sing-box.json", "environment.json", "firewall.nft"} {
		if _, err := os.Stat(filepath.Join(generation.Directory, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	content, err := os.ReadFile(filepath.Join(generation.Directory, "environment.json"))
	if err != nil {
		t.Fatal(err)
	}
	var environment Environment
	if err := json.Unmarshal(content, &environment); err != nil {
		t.Fatal(err)
	}
	if environment.WANDevice != "wan" {
		t.Fatalf("unexpected environment: %#v", environment)
	}
	joined := strings.Join(runner.calls, "\n")
	for _, required := range []string{"sing-box version", "sing-box check -c", "nft -c -f"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing check %q in %s", required, joined)
		}
	}
}

func TestPrepareGenerationRemovesRejectedCandidate(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "steer")
	if err := os.WriteFile(configPath, []byte(minimalConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &prepareRunner{failCheck: true}
	if _, err := PrepareGeneration(context.Background(), runner, PrepareOptions{ConfigPath: configPath, RunDirectory: filepath.Join(root, "run"), StateDirectory: filepath.Join(root, "state"), SingBoxBinary: "/test/sing-box", NFTBinary: "/test/nft", FirewallConfig: writeFirewallConfig(t, root)}); err == nil {
		t.Fatal("native check failure accepted")
	}
	entries, err := os.ReadDir(filepath.Join(root, "run", "generations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("rejected generation leaked: %#v", entries)
	}
}
