// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	model "github.com/gsh20040816/steer/go/internal/intent"
)

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
