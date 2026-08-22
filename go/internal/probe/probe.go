// SPDX-License-Identifier: GPL-3.0-or-later

// Package probe implements platform-neutral HTTP measurement and temporary
// sing-box diagnostics. Platform adapters select Intent and persist reports.
package probe

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
)

const (
	connectTimeout  = 10 * time.Second
	downloadTimeout = 30 * time.Second
)

type Report struct {
	Scope    string    `json:"scope"`
	ObjectID string    `json:"object_id,omitempty"`
	Kind     string    `json:"kind"`
	OK       bool      `json:"ok"`
	Error    string    `json:"error,omitempty"`
	TestedAt time.Time `json:"tested_at"`
	Results  []Result  `json:"results"`
}

type Result struct {
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

func HTTPClient(proxyURL *url.URL, download bool) *http.Client {
	timeout := connectTimeout
	if download {
		timeout = downloadTimeout
	}
	return &http.Client{Timeout: timeout, Transport: &http.Transport{
		Proxy: http.ProxyURL(proxyURL), TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}}
}

func Run(ctx context.Context, client *http.Client, scope, objectID, kind, target string, download bool) Report {
	result := Measure(ctx, client, target, download)
	return Report{Scope: scope, ObjectID: objectID, Kind: kind, OK: result.OK, TestedAt: time.Now().UTC(), Results: []Result{result}}
}

func Measure(ctx context.Context, client *http.Client, target string, download bool) Result {
	result := Result{URL: target}
	attempts := 2
	if download {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		result = measureAttempt(ctx, client, target, download)
		result.Attempts = attempt
		if result.OK {
			return result
		}
	}
	return result
}

func RunTemporary(ctx context.Context, singBoxPath string, outbounds []any, finalTag, scope, objectID, kind, target string, download bool) (Report, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Report{}, fmt.Errorf("allocate test listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	if singBoxPath == "" {
		singBoxPath = "/usr/bin/sing-box"
	}
	temporary, err := os.MkdirTemp("", "steer-test.")
	if err != nil {
		return Report{}, fmt.Errorf("create test directory: %w", err)
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
		return Report{}, fmt.Errorf("encode test config: %w", err)
	}
	if err := os.WriteFile(configPath, append(encoded, '\n'), 0o600); err != nil {
		return Report{}, fmt.Errorf("write test config: %w", err)
	}
	if output, err := exec.CommandContext(ctx, singBoxPath, "check", "-c", configPath).CombinedOutput(); err != nil {
		return Report{}, fmt.Errorf("sing-box test config check failed: %w: %s", err, output)
	}
	process := exec.CommandContext(ctx, singBoxPath, "run", "-c", configPath)
	process.Stdout, process.Stderr = io.Discard, io.Discard
	if err := process.Start(); err != nil {
		return Report{}, fmt.Errorf("start temporary sing-box: %w", err)
	}
	defer func() {
		_ = process.Process.Kill()
		_ = process.Wait()
	}()
	if err := waitReady(ctx, port); err != nil {
		return Report{}, err
	}
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	return Run(ctx, HTTPClient(proxyURL, download), scope, objectID, kind, target, download), nil
}

func measureAttempt(ctx context.Context, client *http.Client, target string, download bool) Result {
	result := Result{URL: target}
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

func waitReady(ctx context.Context, port int) error {
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
