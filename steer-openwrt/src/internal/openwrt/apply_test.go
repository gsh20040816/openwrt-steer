// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type applyRunner struct {
	calls []string
}

func (runner *applyRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	runner.calls = append(runner.calls, call)
	switch {
	case strings.HasSuffix(call, "sing-box version"):
		return []byte("sing-box version 1.13.18\nTags: with_quic,with_utls\n"), nil
	case call == `ubus call uci get {"config":"firewall","type":"zone"}`:
		return []byte(`{"values":{"cfg":{"name":"lan","network":["lan"]}}}`), nil
	case call == "ubus call network.interface.lan status":
		return []byte(`{"up":true,"l3_device":"br-lan"}`), nil
	case call == "ip -json -4 route show default", call == "ip -json -6 route show default":
		return []byte(`[{"dev":"wan"}]`), nil
	case strings.Contains(call, "sing-box check -c"), strings.Contains(call, "nft -c -f"):
		return nil, nil
	case call == "/test/init stop", call == "/test/init start", call == "/usr/bin/env STEER_USE_CURRENT=1 /test/init start":
		return nil, nil
	case call == "/test/nft -j list tables":
		return []byte(`{"nftables":[]}`), nil
	case call == "ip -json -4 rule show", call == "ip -json -6 rule show":
		return []byte(`[{"priority":32766,"table":254}]`), nil
	case call == "ip -4 route flush table 2023", call == "ip -6 route flush table 2023":
		return nil, nil
	case strings.HasPrefix(call, "/test/nft -f "):
		return nil, nil
	case call == `ubus call service list {"name":"steer"}`:
		return []byte(`{"steer":{"instances":{"sing-box":{"running":true,"pid":123}}}}`), nil
	case call == "ip -json link show dev steer0":
		return []byte(`[{"ifname":"steer0"}]`), nil
	case call == "/test/nft -j list table inet steer":
		return []byte(`{"nftables":[{"table":{"family":"inet","name":"steer"}}]}`), nil
	default:
		return nil, fmt.Errorf("unexpected command: %s", call)
	}
}

func TestApplyCommitsHealthyCandidateAndPrunesOldGeneration(t *testing.T) {
	root := t.TempDir()
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	config := strings.Replace(minimalConfig, "option log_level 'warn'", "option log_level 'warn'\n\tlist probe_url '"+server.URL+"'", 1)
	configPath, runDirectory := writeApplyFixture(t, root, config, nil)
	runner := &applyRunner{}
	result, err := Apply(context.Background(), runner, ApplyOptions{
		Prepare:    PrepareOptions{ConfigPath: configPath, RunDirectory: runDirectory, StateDirectory: filepath.Join(root, "state"), SingBoxBinary: "/test/sing-box", NFTBinary: "/test/nft"},
		InitScript: "/test/init", HTTPClient: server.Client(), HealthTimeout: time.Second,
		CheckListeners: func(ports []int) error {
			if len(ports) != 1 || ports[0] != 1053 {
				return fmt.Errorf("unexpected listeners: %v", ports)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.RolledBack || len(result.Probes) != 1 || !result.Probes[0].OK {
		t.Fatalf("unexpected apply result: %#v", result)
	}
	entries, err := os.ReadDir(filepath.Join(runDirectory, "generations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("obsolete runtime generation was not pruned: %#v", entries)
	}
}

func TestApplyRestoresUCIWhenProbeFails(t *testing.T) {
	root := t.TempDir()
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	config := strings.Replace(minimalConfig, "option log_level 'warn'", "option log_level 'warn'\n\tlist probe_url '"+server.URL+"'", 1)
	oldConfig := []byte("old configuration\n")
	configPath, runDirectory := writeApplyFixture(t, root, config, oldConfig)
	runner := &applyRunner{}
	result, err := Apply(context.Background(), runner, ApplyOptions{
		Prepare:    PrepareOptions{ConfigPath: configPath, RunDirectory: runDirectory, StateDirectory: filepath.Join(root, "state"), SingBoxBinary: "/test/sing-box", NFTBinary: "/test/nft"},
		InitScript: "/test/init", HTTPClient: server.Client(), HealthTimeout: time.Second,
		CheckListeners: func([]int) error { return nil },
	})
	if err == nil {
		t.Fatal("failed HTTPS probe was accepted")
	}
	if result.OK || !result.RolledBack || len(result.Probes) != 1 || result.Probes[0].Attempts != 2 {
		t.Fatalf("unexpected rollback result: %#v", result)
	}
	restored, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(restored) != string(oldConfig) {
		t.Fatalf("UCI was not restored: %q", restored)
	}
	calls := strings.Join(runner.calls, "\n")
	if strings.Count(calls, "/test/init stop") != 2 || !strings.HasSuffix(calls, "/test/init start") {
		t.Fatalf("rollback service sequence is incomplete:\n%s", calls)
	}
}

func TestApplyDisabledStopsAndRemovesRuntimeState(t *testing.T) {
	root := t.TempDir()
	config := strings.Replace(minimalConfig, "option enabled '1'", "option enabled '0'", 1)
	configPath, runDirectory := writeApplyFixture(t, root, config, []byte(minimalConfig))
	runner := &applyRunner{}
	result, err := Apply(context.Background(), runner, ApplyOptions{
		Prepare:    PrepareOptions{ConfigPath: configPath, RunDirectory: runDirectory},
		InitScript: "/test/init",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.RolledBack || len(result.Probes) != 0 {
		t.Fatalf("unexpected disable result: %#v", result)
	}
	if _, err := os.Lstat(filepath.Join(runDirectory, "current")); !os.IsNotExist(err) {
		t.Fatalf("disabled current generation remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runDirectory, "generations")); !os.IsNotExist(err) {
		t.Fatalf("disabled generations remain: %v", err)
	}
	if calls := strings.Join(runner.calls, "\n"); calls != "/test/init stop" {
		t.Fatalf("disable executed unexpected commands: %s", calls)
	}
}

func writeApplyFixture(t *testing.T, root, config string, oldConfig []byte) (string, string) {
	t.Helper()
	configPath := filepath.Join(root, "steer")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	runDirectory := filepath.Join(root, "run")
	if oldConfig != nil {
		oldDirectory := filepath.Join(runDirectory, "generations", "old")
		if err := os.MkdirAll(oldDirectory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(oldDirectory, "steer.uci"), oldConfig, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(oldDirectory, filepath.Join(runDirectory, "current")); err != nil {
			t.Fatal(err)
		}
	}
	return configPath, runDirectory
}
