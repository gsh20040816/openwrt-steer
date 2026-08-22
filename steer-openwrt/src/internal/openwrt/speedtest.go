// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/compiler"
	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/model"
)

const (
	connectTestTimeout  = 10 * time.Second
	downloadTestTimeout = 30 * time.Second
)

type TestReport struct {
	Scope    string       `json:"scope"`
	ObjectID string       `json:"object_id,omitempty"`
	Kind     string       `json:"kind"`
	OK       bool         `json:"ok"`
	Error    string       `json:"error,omitempty"`
	TestedAt time.Time    `json:"tested_at"`
	Results  []TestResult `json:"results"`
}

type TestResult struct {
	URL                   string `json:"url"`
	OK                    bool   `json:"ok"`
	Attempts              int    `json:"attempts"`
	Status                int    `json:"status,omitempty"`
	ConnectMilliseconds   int64  `json:"connect_milliseconds,omitempty"`
	TLSMilliseconds       int64  `json:"tls_milliseconds,omitempty"`
	FirstByteMilliseconds int64  `json:"first_byte_milliseconds,omitempty"`
	DownloadMilliseconds  int64  `json:"download_milliseconds,omitempty"`
	DownloadedBytes       int64  `json:"downloaded_bytes,omitempty"`
	Error                 string `json:"error,omitempty"`
}

func ProbeCurrent(ctx context.Context, runDirectory, kind string, client *http.Client) (TestReport, error) {
	return probeCurrent(ctx, runDirectory, "", kind, client)
}

func ProbeCurrentWithState(ctx context.Context, runDirectory, stateDirectory, kind string, client *http.Client) (TestReport, error) {
	return probeCurrent(ctx, runDirectory, stateDirectory, kind, client)
}

func probeCurrent(ctx context.Context, runDirectory, stateDirectory, kind string, client *http.Client) (TestReport, error) {
	if runDirectory == "" {
		runDirectory = "/run/steer"
	}
	plan, err := readCurrentPlan(runDirectory)
	if err != nil {
		return TestReport{}, err
	}
	target, download := "", false
	switch kind {
	case "direct":
		target = plan.ProbeDirect
	case "proxy":
		target = plan.ProbeProxy
	case "speedtest":
		target, download = plan.SpeedtestProxy, true
	default:
		return TestReport{}, fmt.Errorf("unsupported probe kind %q", kind)
	}
	if target == "" {
		return TestReport{}, fmt.Errorf("current execution plan has no %s HTTPS probe", kind)
	}
	if client == nil {
		client = diagnosticHTTPClient(nil, download)
	}
	report := buildTestReport(ctx, client, "overview", "", kind, target, download)
	if stateDirectory != "" {
		if err := saveTestReport(stateDirectory, report); err != nil {
			return report, err
		}
	}
	return report, nil
}

func SpeedTestNode(ctx context.Context, configPath, stateDirectory, singBoxPath, nodeID string, download bool) (TestReport, error) {
	intent, err := readTestIntent(configPath, "node test")
	if err != nil {
		return TestReport{}, err
	}
	var node model.Node
	found := false
	for _, candidate := range intent.Nodes {
		if candidate.ID == nodeID {
			node, found = candidate, true
			break
		}
	}
	if !found || !node.Enabled {
		return TestReport{}, fmt.Errorf("enabled node %q was not found", nodeID)
	}
	target := intent.Main.ProbeProxyURL
	kind := "connect"
	if download {
		target, kind = intent.Main.SpeedtestProxyURL, "download"
	}
	outbounds := []any{compiler.CompileNodeOutbound(node)}
	report, err := runTemporaryProxyTest(ctx, singBoxPath, outbounds, compiler.NodeOutboundTag(node.ID), "nodes", node.ID, kind, target, download)
	if err != nil {
		return TestReport{}, err
	}
	if err := saveTestReport(stateDirectory, report); err != nil {
		return report, err
	}
	return report, nil
}

func SpeedTestRoute(ctx context.Context, configPath, stateDirectory, singBoxPath, routeID string, download bool) (TestReport, error) {
	intent, err := readTestIntent(configPath, "route test")
	if err != nil {
		return TestReport{}, err
	}
	var route model.Route
	found := false
	for _, candidate := range intent.Routes {
		if candidate.ID == routeID {
			route, found = candidate, true
			break
		}
	}
	if !found || !route.Enabled || route.Kind != "single" {
		return TestReport{}, fmt.Errorf("enabled single-node route %q was not found", routeID)
	}
	outbounds := compiler.CompileRouteChainOutbounds(intent, route.ID)
	if len(outbounds) == 0 {
		return TestReport{}, fmt.Errorf("compiled route test has no route-chain outbounds")
	}
	target := intent.Main.ProbeProxyURL
	kind := "connect"
	if download {
		target, kind = intent.Main.SpeedtestProxyURL, "download"
	}
	report, err := runTemporaryProxyTest(ctx, singBoxPath, outbounds, compiler.RouteOutboundTag(route.ID), "routes", route.ID, kind, target, download)
	if err != nil {
		return TestReport{}, err
	}
	if err := saveTestReport(stateDirectory, report); err != nil {
		return report, err
	}
	return report, nil
}

func readTestIntent(configPath, operation string) (model.Intent, error) {
	config, err := os.ReadFile(configPath)
	if err != nil {
		return model.Intent{}, fmt.Errorf("read UCI for %s: %w", operation, err)
	}
	intent, err := DecodeBytes(config)
	if err != nil {
		return model.Intent{}, err
	}
	validation := model.Validate(intent)
	if !validation.OK {
		return model.Intent{}, ValidationError{Validation: validation}
	}
	return intent, nil
}

func runTemporaryProxyTest(ctx context.Context, singBoxPath string, outbounds []any, finalTag, scope, objectID, kind, target string, download bool) (TestReport, error) {
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
	config := map[string]any{
		"log":       map[string]any{"level": "error"},
		"dns":       map[string]any{"servers": []any{map[string]any{"type": "local", "tag": "local"}}},
		"inbounds":  []any{map[string]any{"type": "mixed", "tag": "test-in", "listen": "127.0.0.1", "listen_port": port}},
		"outbounds": outbounds,
		"route":     map[string]any{"final": finalTag, "auto_detect_interface": true, "default_domain_resolver": map[string]any{"server": "local"}},
	}
	configPath := filepath.Join(temporary, "sing-box.json")
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return TestReport{}, fmt.Errorf("encode test config: %w", err)
	}
	if err := os.WriteFile(configPath, append(encoded, '\n'), 0o600); err != nil {
		return TestReport{}, fmt.Errorf("write test config: %w", err)
	}
	if output, err := exec.CommandContext(ctx, singBoxPath, "check", "-c", configPath).CombinedOutput(); err != nil {
		return TestReport{}, fmt.Errorf("sing-box test config check failed: %w: %s", err, output)
	}
	process := exec.CommandContext(ctx, singBoxPath, "run", "-c", configPath)
	process.Stdout, process.Stderr = io.Discard, io.Discard
	if err := process.Start(); err != nil {
		return TestReport{}, fmt.Errorf("start temporary sing-box: %w", err)
	}
	defer func() {
		_ = process.Process.Kill()
		_ = process.Wait()
	}()
	if err := waitProxyReady(ctx, port); err != nil {
		return TestReport{}, err
	}
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	client := diagnosticHTTPClient(proxyURL, download)
	return buildTestReport(ctx, client, scope, objectID, kind, target, download), nil
}

func diagnosticHTTPClient(proxyURL *url.URL, download bool) *http.Client {
	timeout := connectTestTimeout
	if download {
		timeout = downloadTestTimeout
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
}

func buildTestReport(ctx context.Context, client *http.Client, scope, objectID, kind, target string, download bool) TestReport {
	result := measureHTTPTest(ctx, client, target, download)
	return TestReport{
		Scope:    scope,
		ObjectID: objectID,
		Kind:     kind,
		OK:       result.OK,
		TestedAt: time.Now().UTC(),
		Results:  []TestResult{result},
	}
}

func measureHTTPTest(ctx context.Context, client *http.Client, target string, download bool) TestResult {
	result := TestResult{URL: target}
	attempts := 2
	if download {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		result = measureHTTPAttempt(ctx, client, target, download)
		result.Attempts = attempt
		if result.OK {
			return result
		}
	}
	return result
}

func measureHTTPAttempt(ctx context.Context, client *http.Client, target string, download bool) TestResult {
	result := TestResult{URL: target}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	started := time.Now()
	var connectedAt, tlsStartedAt, tlsFinishedAt, firstByteAt time.Time
	trace := &httptrace.ClientTrace{
		GotConn:              func(httptrace.GotConnInfo) { connectedAt = time.Now() },
		TLSHandshakeStart:    func() { tlsStartedAt = time.Now() },
		TLSHandshakeDone:     func(_ tls.ConnectionState, _ error) { tlsFinishedAt = time.Now() },
		GotFirstResponseByte: func() { firstByteAt = time.Now() },
	}
	response, err := client.Do(request.WithContext(httptrace.WithClientTrace(request.Context(), trace)))
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer response.Body.Close()
	if !tlsStartedAt.IsZero() {
		result.ConnectMilliseconds = tlsStartedAt.Sub(started).Milliseconds()
	} else if !connectedAt.IsZero() {
		result.ConnectMilliseconds = connectedAt.Sub(started).Milliseconds()
	}
	if !tlsStartedAt.IsZero() && !tlsFinishedAt.IsZero() {
		result.TLSMilliseconds = tlsFinishedAt.Sub(tlsStartedAt).Milliseconds()
	}
	if !firstByteAt.IsZero() {
		result.FirstByteMilliseconds = firstByteAt.Sub(started).Milliseconds()
	}
	result.Status = response.StatusCode
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		result.Error = fmt.Sprintf("unexpected HTTP status %d", response.StatusCode)
		return result
	}
	if download {
		downloadStarted := time.Now()
		result.DownloadedBytes, err = io.Copy(io.Discard, response.Body)
		result.DownloadMilliseconds = time.Since(downloadStarted).Milliseconds()
	} else {
		_, err = io.CopyN(io.Discard, response.Body, 1)
		if err == io.EOF {
			err = nil
		}
	}
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.OK = true
	return result
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
	if err := atomicWrite(filepath.Join(directory, report.Kind+".json"), append(encoded, '\n')); err != nil {
		return fmt.Errorf("save test report: %w", err)
	}
	return nil
}

func waitProxyReady(ctx context.Context, port int) error {
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
