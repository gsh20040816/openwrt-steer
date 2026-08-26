// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	model "github.com/gsh20040816/steer/go/internal/intent"
)

func TestConfiguredSubscriptionSchedule(t *testing.T) {
	tests := []struct {
		name        string
		interval    string
		manualID    string
		snapshotAge time.Duration
		wantFetch   bool
	}{
		{name: "automatic empty interval"},
		{name: "manual empty interval", manualID: "public", wantFetch: true},
		{name: "first scheduled fetch", interval: "1h", wantFetch: true},
		{name: "automatic before due", interval: "1h", snapshotAge: 30 * time.Minute},
		{name: "automatic just due", interval: "1h", snapshotAge: time.Hour, wantFetch: true},
		{name: "manual before due", interval: "1h", manualID: "public", snapshotAge: 30 * time.Minute, wantFetch: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				_, _ = writer.Write([]byte("socks://user:pass@127.0.0.1:1080#Imported\n"))
			}))
			defer server.Close()

			root := t.TempDir()
			configPath := filepath.Join(root, "config.json")
			stateDirectory := filepath.Join(root, "state")
			value := validIntent()
			value.Subscriptions = []model.Subscription{{ID: "public", Enabled: true, URL: server.URL, UpdateInterval: test.interval}}
			if _, err := (IntentStore{Paths: Paths{ConfigPath: configPath}}).Save(value, ""); err != nil {
				t.Fatal(err)
			}
			if test.snapshotAge != 0 {
				if err := saveSubscriptionSnapshot(stateDirectory, SubscriptionSnapshot{SubscriptionID: "public", URL: server.URL, FetchedAt: time.Now().Add(-test.snapshotAge)}); err != nil {
					t.Fatal(err)
				}
			}

			snapshots, err := UpdateConfiguredSubscriptions(context.Background(), server.Client(), configPath, stateDirectory, test.manualID)
			if err != nil {
				t.Fatal(err)
			}
			wantCount := int32(0)
			if test.wantFetch {
				wantCount = 1
			}
			if got := requests.Load(); got != wantCount || len(snapshots) != int(wantCount) {
				t.Fatalf("requests=%d snapshots=%d, want %d", got, len(snapshots), wantCount)
			}
		})
	}
}

func TestIntentStoreSubscriptionNodeReplacement(t *testing.T) {
	paths, err := NewPaths(filepath.Join(t.TempDir(), "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	value := validIntent()
	value.Subscriptions = []model.Subscription{{ID: "sub", Enabled: true, URL: "https://example.invalid/sub"}}
	value.Nodes = []model.Node{{ID: "old", Enabled: true, Type: "socks", Server: "127.0.0.1", ServerPort: 1080, NodeSource: model.NodeSource{SourceSubscription: "sub"}}}
	store := IntentStore{Paths: paths}
	if _, err := store.Save(value, ""); err != nil {
		t.Fatal(err)
	}
	replacement := []model.Node{{ID: "new", Enabled: true, Type: "socks", Server: "127.0.0.2", ServerPort: 1080, NodeSource: model.NodeSource{SourceSubscription: "sub"}}}
	if err := store.ReplaceNodes(context.Background(), "sub", value.Nodes, replacement); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := store.Load()
	if err != nil || len(loaded.Nodes) != 1 || loaded.Nodes[0].ID != "new" {
		t.Fatalf("subscription replacement failed: %#v %v", loaded.Nodes, err)
	}
}

func TestConfiguredSubscriptionUpdatePersistsStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("socks://user:pass@127.0.0.1:1080#Imported\n"))
	}))
	defer server.Close()
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	stateDirectory := filepath.Join(root, "state")
	value := validIntent()
	value.Subscriptions = []model.Subscription{{ID: "public", Enabled: true, Name: "Public", URL: server.URL, UpdateInterval: "1h"}}
	if _, err := (IntentStore{Paths: Paths{ConfigPath: configPath}}).Save(value, ""); err != nil {
		t.Fatal(err)
	}
	snapshots, err := UpdateConfiguredSubscriptions(context.Background(), server.Client(), configPath, stateDirectory, "public")
	if err != nil || len(snapshots) != 1 || len(snapshots[0].Nodes) != 1 {
		t.Fatalf("unexpected subscription update: %#v %v", snapshots, err)
	}
	statuses, err := ReadSubscriptionStatus(configPath, stateDirectory)
	if err != nil || len(statuses) != 1 || statuses[0].NodeCount != 1 || !strings.Contains(statuses[0].URL, "http") {
		t.Fatalf("unexpected subscription status: %#v %v", statuses, err)
	}
}

func TestFailedRefreshKeepsLastSuccessfulSubscriptionStatus(t *testing.T) {
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			http.Error(writer, "temporary", http.StatusServiceUnavailable)
			return
		}
		_, _ = writer.Write([]byte("socks://user:pass@127.0.0.1:1080#Imported\n"))
	}))
	defer server.Close()
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	stateDirectory := filepath.Join(root, "state")
	value := validIntent()
	value.Subscriptions = []model.Subscription{{ID: "public", Enabled: true, URL: server.URL}}
	if _, err := (IntentStore{Paths: Paths{ConfigPath: configPath}}).Save(value, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateConfiguredSubscriptions(context.Background(), server.Client(), configPath, stateDirectory, "public"); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	if _, err := UpdateConfiguredSubscriptions(context.Background(), server.Client(), configPath, stateDirectory, "public"); err == nil || err.Error() != "update subscription public: subscription server returned HTTP 503" {
		t.Fatalf("unexpected safe update failure: %v", err)
	}
	statuses, err := ReadSubscriptionStatus(configPath, stateDirectory)
	if err != nil || len(statuses) != 1 || statuses[0].LastSuccess == nil || statuses[0].LastFailure == nil || statuses[0].NodeCount != 1 {
		t.Fatalf("failed refresh destroyed successful facts: %#v %v", statuses, err)
	}
}
