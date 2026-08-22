// SPDX-License-Identifier: GPL-3.0-or-later

// Package probe implements platform-neutral HTTP measurement. Platform
// adapters select Intent, provide transports and persist reports.
package probe

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
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
