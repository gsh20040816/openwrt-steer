// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMeasureNodeSpeedTest(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte("fixture payload"))
	}))
	defer server.Close()
	result := measureNodeSpeedTest(context.Background(), server.Client(), server.URL, true)
	if result.Error != "" || result.Status != http.StatusOK || result.DownloadedBytes != int64(len("fixture payload")) || result.FirstByteMilliseconds < 0 {
		t.Fatalf("unexpected node speed test result: %#v", result)
	}
}

func TestSaveNodeSpeedTestReport(t *testing.T) {
	stateDirectory := t.TempDir()
	report := NodeSpeedTestReport{
		NodeID:   "node_a",
		Download: true,
		TestedAt: time.Date(2026, time.August, 22, 4, 5, 6, 0, time.UTC),
		Results: []NodeSpeedTestURL{{
			URL:                  "https://speed.example/",
			Status:               http.StatusOK,
			DownloadMilliseconds: 250,
			DownloadedBytes:      1024,
		}},
	}
	if err := saveNodeSpeedTestReport(stateDirectory, report); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(stateDirectory, "logs", "speedtests", "node_a", "download.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved NodeSpeedTestReport
	if err := json.Unmarshal(content, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.NodeID != report.NodeID || !saved.Download || !saved.TestedAt.Equal(report.TestedAt) || len(saved.Results) != 1 {
		t.Fatalf("unexpected saved speed-test report: %#v", saved)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("speed-test report mode = %o, want 600", info.Mode().Perm())
	}
}
