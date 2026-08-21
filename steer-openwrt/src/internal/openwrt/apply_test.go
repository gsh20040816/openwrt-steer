// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"errors"
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
	case strings.Contains(call, "sing-box check -c"), strings.Contains(call, "nft -c -f"):
		return nil, nil
	case call == "/test/init stop", call == "/test/init start", call == "/usr/bin/env STEER_USE_CURRENT=1 /test/init start":
		return nil, nil
	case call == "/test/nft -j list tables":
		return []byte(`{"nftables":[]}`), nil
	case call == "ip -json -4 rule show", call == "ip -json -6 rule show":
		return []byte(`[{"priority":32766,"table":254}]`), nil
	case call == "ip -json -4 route show table all", call == "ip -json -6 route show table all":
		return []byte(`[{"dst":"default","table":254}]`), nil
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

func TestApplyCommitsHealthyCandidateWithoutRunningConfiguredProbes(t *testing.T) {
	root := t.TempDir()
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests++
		response.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	config := strings.Replace(minimalConfig, "option log_level 'warn'", "option log_level 'warn'\n\tlist probe_direct '"+server.URL+"'", 1)
	configPath, runDirectory := writeApplyFixture(t, root, config, nil)
	runner := &applyRunner{}
	result, err := Apply(context.Background(), runner, ApplyOptions{
		Prepare:    PrepareOptions{ConfigPath: configPath, RunDirectory: runDirectory, StateDirectory: filepath.Join(root, "state"), SingBoxBinary: "/test/sing-box", NFTBinary: "/test/nft"},
		InitScript: "/test/init", HealthTimeout: time.Second,
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
	if !result.OK {
		t.Fatalf("unexpected apply result: %#v", result)
	}
	if requests != 0 {
		t.Fatalf("Apply ran %d configured HTTPS probes", requests)
	}
	entries, err := os.ReadDir(filepath.Join(runDirectory, "generations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("obsolete runtime generation was not pruned: %#v", entries)
	}
}

func TestApplyHealthFailurePreservesCandidateAndFailureScene(t *testing.T) {
	root := t.TempDir()
	configPath, runDirectory := writeApplyFixture(t, root, minimalConfig, nil)
	runner := &applyRunner{}
	result, err := Apply(context.Background(), runner, ApplyOptions{
		Prepare:    PrepareOptions{ConfigPath: configPath, RunDirectory: runDirectory, StateDirectory: filepath.Join(root, "state"), SingBoxBinary: "/test/sing-box", NFTBinary: "/test/nft"},
		InitScript: "/test/init", HealthTimeout: time.Millisecond,
		CheckListeners: func([]int) error { return errors.New("listener missing") },
	})
	if err == nil {
		t.Fatal("unhealthy candidate was accepted")
	}
	if result.OK || result.Generation == "" {
		t.Fatalf("failure result lost candidate identity: %#v", result)
	}
	preserved, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(preserved) != minimalConfig {
		t.Fatalf("candidate UCI was rewritten: %q", preserved)
	}
	if _, err := os.Readlink(filepath.Join(runDirectory, "current")); err != nil {
		t.Fatalf("candidate failure scene was removed: %v", err)
	}
	calls := strings.Join(runner.calls, "\n")
	if strings.Count(calls, "/test/init stop") != 1 || strings.Count(calls, "STEER_USE_CURRENT=1 /test/init start") != 1 {
		t.Fatalf("failure unexpectedly ran a rollback sequence:\n%s", calls)
	}
}

func TestApplyBacksUpHealthyCurrentAndRollbackConsumesBackup(t *testing.T) {
	root := t.TempDir()
	oldConfig := []byte(strings.Replace(minimalConfig, "option log_level 'warn'", "option log_level 'error'", 1))
	configPath, runDirectory := writeApplyFixture(t, root, minimalConfig, oldConfig)
	writeCurrentPlan(t, runDirectory)
	backupPath := filepath.Join(root, "state", "rollback.uci")
	runner := &applyRunner{}
	options := ApplyOptions{
		Prepare:        PrepareOptions{ConfigPath: configPath, RunDirectory: runDirectory, StateDirectory: filepath.Join(root, "state"), SingBoxBinary: "/test/sing-box", NFTBinary: "/test/nft"},
		InitScript:     "/test/init",
		BackupPath:     backupPath,
		HealthTimeout:  time.Second,
		CheckListeners: func([]int) error { return nil },
	}
	if result, err := Apply(context.Background(), runner, options); err != nil || !result.OK {
		t.Fatalf("apply failed: result=%#v error=%v", result, err)
	}
	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != string(oldConfig) {
		t.Fatalf("rollback backup is not the previous healthy UCI: %q", backup)
	}
	if result, err := Rollback(context.Background(), runner, options); err != nil || !result.OK {
		t.Fatalf("rollback failed: result=%#v error=%v", result, err)
	}
	restored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(oldConfig) {
		t.Fatalf("rollback did not restore UCI: %q", restored)
	}
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatalf("successful rollback did not consume backup: %v", err)
	}
}

func TestProbeCurrentReportsFailuresWithoutApplying(t *testing.T) {
	root := t.TempDir()
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	runDirectory := filepath.Join(root, "run")
	current := filepath.Join(runDirectory, "current")
	if err := os.MkdirAll(current, 0o700); err != nil {
		t.Fatal(err)
	}
	plan := fmt.Sprintf(`{"probe_direct":[%q],"probe_proxy":[%q]}`, server.URL, server.URL)
	if err := os.WriteFile(filepath.Join(current, "plan.json"), []byte(plan), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := ProbeCurrent(context.Background(), runDirectory, "direct", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || len(report.Results) != 1 || report.Results[0].Attempts != 2 {
		t.Fatalf("unexpected probe report: %#v", report)
	}
	proxyReport, err := ProbeCurrent(context.Background(), runDirectory, "proxy", server.Client())
	if err != nil || proxyReport.Kind != "proxy" || len(proxyReport.Results) != 1 {
		t.Fatalf("selected proxy probe was not used: report=%#v err=%v", proxyReport, err)
	}
}

func TestProbeCurrentRejectsEmptyDiagnosticPlan(t *testing.T) {
	runDirectory := filepath.Join(t.TempDir(), "run")
	current := filepath.Join(runDirectory, "current")
	if err := os.MkdirAll(current, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(current, "plan.json"), []byte(`{"probe_direct":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ProbeCurrent(context.Background(), runDirectory, "direct", nil); err == nil {
		t.Fatal("empty manual diagnostic was reported successful")
	}
}

func TestRollbackFailureKeepsSingleUseBackup(t *testing.T) {
	root := t.TempDir()
	configPath, runDirectory := writeApplyFixture(t, root, minimalConfig, nil)
	backupPath := filepath.Join(root, "rollback.uci")
	broken := []byte("config steer 'main'\n\toption schema_version '3'\n")
	if err := os.WriteFile(backupPath, broken, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Rollback(context.Background(), &applyRunner{}, ApplyOptions{
		Prepare:    PrepareOptions{ConfigPath: configPath, RunDirectory: runDirectory},
		BackupPath: backupPath,
	})
	if err == nil || result.OK {
		t.Fatalf("invalid rollback backup was accepted: result=%#v error=%v", result, err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("failed rollback consumed its backup: %v", err)
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
	if !result.OK {
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

func writeCurrentPlan(t *testing.T, runDirectory string) {
	t.Helper()
	plan := `{"resources":{"dns_port":1053,"tun_interface":"steer0"}}`
	if err := os.WriteFile(filepath.Join(runDirectory, "current", "plan.json"), []byte(plan), 0o600); err != nil {
		t.Fatal(err)
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
