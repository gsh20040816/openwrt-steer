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

type NodeSpeedTestReport struct {
	NodeID   string             `json:"node_id"`
	Download bool               `json:"download"`
	TestedAt time.Time          `json:"tested_at"`
	Results  []NodeSpeedTestURL `json:"results"`
}

type NodeSpeedTestURL struct {
	URL                   string `json:"url"`
	Status                int    `json:"status,omitempty"`
	ConnectMilliseconds   int64  `json:"connect_milliseconds,omitempty"`
	TLSMilliseconds       int64  `json:"tls_milliseconds,omitempty"`
	FirstByteMilliseconds int64  `json:"first_byte_milliseconds,omitempty"`
	DownloadMilliseconds  int64  `json:"download_milliseconds,omitempty"`
	DownloadedBytes       int64  `json:"downloaded_bytes,omitempty"`
	Error                 string `json:"error,omitempty"`
}

func SpeedTestNode(ctx context.Context, configPath, stateDirectory, singBoxPath, nodeID string, download bool) (NodeSpeedTestReport, error) {
	config, err := os.ReadFile(configPath)
	if err != nil {
		return NodeSpeedTestReport{}, fmt.Errorf("read UCI for node speed test: %w", err)
	}
	intent, err := DecodeBytes(config)
	if err != nil {
		return NodeSpeedTestReport{}, err
	}
	validation := model.Validate(intent)
	if !validation.OK {
		return NodeSpeedTestReport{}, ValidationError{Validation: validation}
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
		return NodeSpeedTestReport{}, fmt.Errorf("enabled node %q was not found", nodeID)
	}
	if len(intent.Main.SpeedtestProxyURLs) == 0 {
		return NodeSpeedTestReport{}, fmt.Errorf("no speedtest_proxy URLs are configured")
	}
	if singBoxPath == "" {
		singBoxPath = "/usr/bin/sing-box"
	}
	report, err := runTemporaryNodeProxy(ctx, singBoxPath, node, intent.Main.SpeedtestProxyURLs, download)
	if err != nil {
		return NodeSpeedTestReport{}, err
	}
	if err := saveNodeSpeedTestReport(stateDirectory, report); err != nil {
		return NodeSpeedTestReport{}, err
	}
	return report, nil
}

func runTemporaryNodeProxy(ctx context.Context, singBoxPath string, node model.Node, targets []string, download bool) (NodeSpeedTestReport, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return NodeSpeedTestReport{}, fmt.Errorf("allocate speed-test listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	temporary, err := os.MkdirTemp("", "steer-speedtest.")
	if err != nil {
		return NodeSpeedTestReport{}, fmt.Errorf("create speed-test directory: %w", err)
	}
	defer os.RemoveAll(temporary)
	tag := compiler.NodeOutboundTag(node.ID)
	config := map[string]any{
		"log":       map[string]any{"level": "error"},
		"dns":       map[string]any{"servers": []any{map[string]any{"type": "local", "tag": "local"}}},
		"inbounds":  []any{map[string]any{"type": "mixed", "tag": "probe-in", "listen": "127.0.0.1", "listen_port": port}},
		"outbounds": []any{compiler.CompileNodeOutbound(node)},
		"route":     map[string]any{"final": tag, "auto_detect_interface": true, "default_domain_resolver": map[string]any{"server": "local"}},
	}
	configPath := filepath.Join(temporary, "sing-box.json")
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return NodeSpeedTestReport{}, fmt.Errorf("encode speed-test config: %w", err)
	}
	if err := os.WriteFile(configPath, append(encoded, '\n'), 0o600); err != nil {
		return NodeSpeedTestReport{}, fmt.Errorf("write speed-test config: %w", err)
	}
	if output, err := exec.CommandContext(ctx, singBoxPath, "check", "-c", configPath).CombinedOutput(); err != nil {
		return NodeSpeedTestReport{}, fmt.Errorf("sing-box speed-test config check failed: %w: %s", err, output)
	}
	process := exec.CommandContext(ctx, singBoxPath, "run", "-c", configPath)
	process.Stdout, process.Stderr = io.Discard, io.Discard
	if err := process.Start(); err != nil {
		return NodeSpeedTestReport{}, fmt.Errorf("start temporary sing-box: %w", err)
	}
	defer func() {
		_ = process.Process.Kill()
		_ = process.Wait()
	}()
	if err := waitProxyReady(ctx, port); err != nil {
		return NodeSpeedTestReport{}, err
	}
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}, Timeout: 30 * time.Second}
	report := NodeSpeedTestReport{
		NodeID:   node.ID,
		Download: download,
		TestedAt: time.Now().UTC(),
		Results:  make([]NodeSpeedTestURL, 0, len(targets)),
	}
	for _, target := range targets {
		result := measureNodeSpeedTest(ctx, client, target, download)
		report.Results = append(report.Results, result)
	}
	return report, nil
}

func saveNodeSpeedTestReport(stateDirectory string, report NodeSpeedTestReport) error {
	if stateDirectory == "" {
		stateDirectory = "/var/lib/steer"
	}
	directory := filepath.Join(stateDirectory, "logs", "speedtests", report.NodeID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create speed-test log directory: %w", err)
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode speed-test report: %w", err)
	}
	kind := "connect"
	if report.Download {
		kind = "download"
	}
	path := filepath.Join(directory, kind+".json")
	if err := atomicWrite(path, append(encoded, '\n')); err != nil {
		return fmt.Errorf("save speed-test report: %w", err)
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

func measureNodeSpeedTest(ctx context.Context, client *http.Client, target string, download bool) NodeSpeedTestURL {
	result := NodeSpeedTestURL{URL: target}
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
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))
	response, err := client.Do(request)
	if err != nil {
		result.Error = err.Error()
		return result
	}
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
	if download {
		downloadStarted := time.Now()
		result.DownloadedBytes, err = io.Copy(io.Discard, response.Body)
		result.DownloadMilliseconds = time.Since(downloadStarted).Milliseconds()
	} else {
		_, err = io.CopyN(io.Discard, response.Body, 1)
	}
	_ = response.Body.Close()
	if err != nil && err != io.EOF {
		result.Error = err.Error()
	}
	return result
}
