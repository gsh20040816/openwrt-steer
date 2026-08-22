// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gsh20040816/openwrt-steer/go/internal/compiler"
	"github.com/gsh20040816/openwrt-steer/go/internal/generation"
	model "github.com/gsh20040816/openwrt-steer/go/internal/intent"
	"github.com/gsh20040816/openwrt-steer/go/internal/probe"
)

type TestReport = probe.Report
type TestResult = probe.Result

func ProbeCurrentWithState(ctx context.Context, runDirectory, stateDirectory, kind string, client *http.Client) (TestReport, error) {
	if runDirectory == "" {
		runDirectory = "/run/steer"
	}
	intent, err := generation.ReadIntent(filepath.Join(runDirectory, "current"))
	if err != nil {
		return TestReport{}, err
	}
	target, download := "", false
	switch kind {
	case "direct":
		target = intent.Main.ProbeDirectURL
	case "proxy":
		target = intent.Main.ProbeProxyURL
	case "speedtest":
		target, download = intent.Main.SpeedtestProxyURL, true
	default:
		return TestReport{}, fmt.Errorf("unsupported probe kind %q", kind)
	}
	if target == "" {
		return TestReport{}, fmt.Errorf("current intent has no %s HTTPS probe", kind)
	}
	if client == nil {
		client = probe.HTTPClient(nil, download)
	}
	report := probe.Run(ctx, client, "overview", "", kind, target, download)
	if stateDirectory != "" {
		if err := saveTestReport(stateDirectory, report); err != nil {
			return report, err
		}
	}
	return report, nil
}

func SpeedTestNode(ctx context.Context, configPath, stateDirectory, singBoxPath, nodeID string, download bool) (TestReport, error) {
	value, err := readProbeIntent(configPath)
	if err != nil {
		return TestReport{}, err
	}
	var node model.Node
	found := false
	for _, candidate := range value.Nodes {
		if candidate.ID == nodeID {
			node, found = candidate, true
			break
		}
	}
	if !found || !node.Enabled {
		return TestReport{}, fmt.Errorf("enabled node %q was not found", nodeID)
	}
	target, kind := value.Main.ProbeProxyURL, "connect"
	if download {
		target, kind = value.Main.SpeedtestProxyURL, "download"
	}
	report, err := runTemporaryProbe(ctx, singBoxPath, []any{compiler.CompileNodeOutbound(node)}, compiler.NodeOutboundTag(node.ID), "nodes", node.ID, kind, target, download)
	if err != nil {
		return TestReport{}, err
	}
	if err := saveTestReport(stateDirectory, report); err != nil {
		return report, err
	}
	return report, nil
}

func SpeedTestRoute(ctx context.Context, configPath, stateDirectory, singBoxPath, routeID string, download bool) (TestReport, error) {
	value, err := readProbeIntent(configPath)
	if err != nil {
		return TestReport{}, err
	}
	var route model.Route
	found := false
	for _, candidate := range value.Routes {
		if candidate.ID == routeID {
			route, found = candidate, true
			break
		}
	}
	if !found || !route.Enabled || route.Kind != "single" {
		return TestReport{}, fmt.Errorf("enabled single-node route %q was not found", routeID)
	}
	outbounds := compiler.CompileRouteChainOutbounds(value, route.ID)
	if len(outbounds) == 0 {
		return TestReport{}, fmt.Errorf("compiled route test has no route-chain outbounds")
	}
	target, kind := value.Main.ProbeProxyURL, "connect"
	if download {
		target, kind = value.Main.SpeedtestProxyURL, "download"
	}
	report, err := runTemporaryProbe(ctx, singBoxPath, outbounds, compiler.RouteOutboundTag(route.ID), "routes", route.ID, kind, target, download)
	if err != nil {
		return TestReport{}, err
	}
	if err := saveTestReport(stateDirectory, report); err != nil {
		return report, err
	}
	return report, nil
}

func runTemporaryProbe(ctx context.Context, singBoxPath string, outbounds []any, finalTag, scope, objectID, kind, target string, download bool) (TestReport, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return TestReport{}, fmt.Errorf("allocate test listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	if singBoxPath == "" {
		singBoxPath = "/usr/bin/sing-box"
	}
	temporary, err := os.MkdirTemp("", "steer-test.")
	if err != nil {
		return TestReport{}, fmt.Errorf("create test directory: %w", err)
	}
	defer os.RemoveAll(temporary)
	config, err := temporaryProbeConfig(outbounds, finalTag, port)
	if err != nil {
		return TestReport{}, err
	}
	configPath := filepath.Join(temporary, "sing-box.json")
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return TestReport{}, fmt.Errorf("encode test config: %w", err)
	}
	if err := os.WriteFile(configPath, append(encoded, '\n'), 0o600); err != nil {
		return TestReport{}, fmt.Errorf("write test config: %w", err)
	}
	check := newProbeCommand(ctx, singBoxPath, "check", "-c", configPath)
	if output, err := check.CombinedOutput(); err != nil {
		return TestReport{}, fmt.Errorf("sing-box test config check failed: %w: %s", err, output)
	}
	process := newProbeCommand(ctx, singBoxPath, "run", "-c", configPath)
	process.Stdout, process.Stderr = io.Discard, io.Discard
	if err := process.Start(); err != nil {
		return TestReport{}, fmt.Errorf("start temporary sing-box: %w", err)
	}
	defer func() {
		_ = process.Process.Kill()
		_ = process.Wait()
	}()
	if err := waitTemporaryProbeReady(ctx, port); err != nil {
		return TestReport{}, err
	}
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	return probe.Run(ctx, probe.HTTPClient(proxyURL, download), scope, objectID, kind, target, download), nil
}

func temporaryProbeConfig(outbounds []any, finalTag string, port int) (map[string]any, error) {
	marked := make([]any, 0, len(outbounds))
	for _, value := range outbounds {
		outbound, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("temporary probe outbound has unexpected type %T", value)
		}
		copy := make(map[string]any, len(outbound)+1)
		for key, field := range outbound {
			copy[key] = field
		}
		// The main Linux auto-redirect path must not capture a probe that is
		// intentionally testing a different node or route.
		copy["routing_mark"] = AutoRedirectOutputMark
		marked = append(marked, copy)
	}
	return map[string]any{
		"log": map[string]any{"level": "error"},
		"dns": map[string]any{
			"servers": []any{map[string]any{"type": "local", "tag": "local", "routing_mark": AutoRedirectOutputMark}},
		},
		"inbounds":  []any{map[string]any{"type": "mixed", "tag": "test-in", "listen": "127.0.0.1", "listen_port": port}},
		"outbounds": marked,
		"route":     map[string]any{"final": finalTag, "auto_detect_interface": true, "default_domain_resolver": map[string]any{"server": "local"}},
	}, nil
}

func newProbeCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, name, args...)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return command
}

func waitTemporaryProbeReady(ctx context.Context, port int) error {
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		connection, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("temporary sing-box did not become ready: %w", ctx.Err())
		case <-deadline.C:
			return fmt.Errorf("temporary sing-box did not become ready: %w", err)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func readProbeIntent(path string) (model.Intent, error) {
	value, _, err := (JSONStore{Path: path}).Load()
	if err != nil {
		return model.Intent{}, err
	}
	validation := Validate(value)
	if !validation.OK {
		return model.Intent{}, ValidationError{Validation: validation}
	}
	return value, nil
}

func saveTestReport(stateDirectory string, report TestReport) error {
	if stateDirectory == "" {
		stateDirectory = "/var/lib/steer"
	}
	directory := filepath.Join(stateDirectory, "logs", "tests", report.Scope)
	if report.ObjectID != "" {
		directory = filepath.Join(directory, report.ObjectID)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create test log directory: %w", err)
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode test report: %w", err)
	}
	return atomicWrite(filepath.Join(directory, report.Kind+".json"), append(encoded, '\n'))
}
