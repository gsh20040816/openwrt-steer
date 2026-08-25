// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"context"
	"path/filepath"
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
