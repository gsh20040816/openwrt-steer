// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

// Platform probe tests cover OpenWrt intent and runtime integration.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveTestReport(t *testing.T) {
	stateDirectory := t.TempDir()
	report := TestReport{
		Scope:    "nodes",
		ObjectID: "node_a",
		Kind:     "download",
		OK:       true,
		TestedAt: time.Date(2026, time.August, 22, 4, 5, 6, 0, time.UTC),
		Results: []TestResult{{
			URL:                  "https://speed.example/",
			OK:                   true,
			Status:               http.StatusOK,
			DownloadMilliseconds: 250,
			DownloadedBytes:      1024,
		}},
	}
	if err := saveTestReport(stateDirectory, report); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(stateDirectory, "logs", "tests", "nodes", "node_a", "download.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved TestReport
	if err := json.Unmarshal(content, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Scope != report.Scope || saved.ObjectID != report.ObjectID || !saved.OK || !saved.TestedAt.Equal(report.TestedAt) || len(saved.Results) != 1 {
		t.Fatalf("unexpected saved test report: %#v", saved)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("test report mode = %o, want 600", info.Mode().Perm())
	}
}

func TestNodeAndRouteTestsRejectDisabledObjects(t *testing.T) {
	config := minimalConfig + `
config node 'disabled_node'
	option enabled '0'
	option type 'socks'
	option server '192.0.2.10'
	option server_port '1080'

config route 'disabled_route'
	option enabled '0'
	option kind 'single'
	option node 'disabled_node'
`
	configPath := filepath.Join(t.TempDir(), "steer")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SpeedTestNode(context.Background(), configPath, t.TempDir(), "/definitely/missing/sing-box", "disabled_node", false); err == nil {
		t.Fatal("disabled node test was accepted")
	}
	if _, err := SpeedTestRoute(context.Background(), configPath, t.TempDir(), "/definitely/missing/sing-box", "disabled_route", false); err == nil {
		t.Fatal("disabled route test was accepted")
	}
}
