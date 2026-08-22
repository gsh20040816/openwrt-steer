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

	coreapply "github.com/gsh20040816/openwrt-steer/go/internal/apply"
	"github.com/gsh20040816/openwrt-steer/go/internal/generation"
)

type applyRunner struct{ calls []string }

func (runner *applyRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	runner.calls = append(runner.calls, call)
	switch {
	case strings.HasSuffix(call, "sing-box version"):
		return []byte("sing-box version 1.13.19\nTags: with_quic,with_utls\n"), nil
	case strings.Contains(call, "sing-box check -c"), strings.Contains(call, "nft -c -f"):
		return nil, nil
	case call == "/test/init stop", call == "/usr/bin/env STEER_USE_CURRENT=1 /test/init start":
		return nil, nil
	case call == "/test/nft -j list tables":
		return []byte(`{"nftables":[]}`), nil
	case call == "ip -json -4 rule show", call == "ip -json -6 rule show":
		return []byte(`[{"priority":32766,"table":254}]`), nil
	case call == "ip -json -4 route show table all", call == "ip -json -6 route show table all":
		return []byte(`[{"dst":"default","table":254}]`), nil
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

func TestApplyCompletesNormalOpenWrtLifecycleWithoutRunningConfiguredProbes(t *testing.T) {
	root := t.TempDir()
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests++
		response.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	config := strings.Replace(minimalConfig, "option probe_direct 'https://www.baidu.com/'", "option probe_direct '"+server.URL+"'", 1)
	value, err := DecodeBytes([]byte(config))
	if err != nil {
		t.Fatal(err)
	}
	runDirectory := filepath.Join(root, "run")
	old := filepath.Join(runDirectory, "generations", "old")
	if err := os.MkdirAll(old, 0o700); err != nil {
		t.Fatal(err)
	}
	runner := &applyRunner{}
	backend := NewBackend(runner, value, BackendOptions{
		RunDirectory: runDirectory, StateDirectory: filepath.Join(root, "state"), SingBoxBinary: "/test/sing-box",
		NFTBinary: "/test/nft", InitScript: "/test/init", HealthTimeout: time.Second,
		CheckListeners: func(ports []int) error {
			if len(ports) != 1 || ports[0] != 1053 {
				return fmt.Errorf("unexpected listeners: %v", ports)
			}
			return nil
		},
	})
	result, err := coreapply.Run(context.Background(), value, backend.CompilerOptions(), backend)
	if err != nil || !result.OK {
		t.Fatalf("Apply failed: result=%#v error=%v", result, err)
	}
	if requests != 0 {
		t.Fatalf("Apply ran %d configured HTTPS probes", requests)
	}
	entries, err := os.ReadDir(filepath.Join(runDirectory, "generations"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("obsolete runtime generation was not pruned: %#v %v", entries, err)
	}
}

func TestDisableStopsAndRemovesRuntimeState(t *testing.T) {
	root := t.TempDir()
	config := strings.Replace(minimalConfig, "option enabled '1'", "option enabled '0'", 1)
	value, err := DecodeBytes([]byte(config))
	if err != nil {
		t.Fatal(err)
	}
	runDirectory := filepath.Join(root, "run")
	if err := os.MkdirAll(filepath.Join(runDirectory, "generations", "old"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(runDirectory, "generations", "old"), filepath.Join(runDirectory, "current")); err != nil {
		t.Fatal(err)
	}
	runner := &applyRunner{}
	backend := NewBackend(runner, value, BackendOptions{RunDirectory: runDirectory, InitScript: "/test/init"})
	result, err := coreapply.Run(context.Background(), value, backend.CompilerOptions(), backend)
	if err != nil || !result.OK {
		t.Fatalf("disable failed: %#v %v", result, err)
	}
	if _, err := os.Lstat(filepath.Join(runDirectory, "current")); !os.IsNotExist(err) {
		t.Fatalf("disabled current generation remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runDirectory, "generations")); !os.IsNotExist(err) {
		t.Fatalf("disabled generations remain: %v", err)
	}
}

func TestProbeCurrentReadsCanonicalIntent(t *testing.T) {
	root := t.TempDir()
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	config := strings.Replace(minimalConfig, "option probe_direct 'https://www.baidu.com/'", "option probe_direct '"+server.URL+"'", 1)
	value, err := DecodeBytes([]byte(config))
	if err != nil {
		t.Fatal(err)
	}
	runDirectory := filepath.Join(root, "run")
	candidate, err := generation.Create(filepath.Join(runDirectory, "generations"), value, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(candidate.Directory, filepath.Join(runDirectory, "current")); err != nil {
		t.Fatal(err)
	}
	report, err := ProbeCurrent(context.Background(), runDirectory, "direct", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || len(report.Results) != 1 || report.Results[0].Attempts != 2 {
		t.Fatalf("unexpected probe report: %#v", report)
	}
}
