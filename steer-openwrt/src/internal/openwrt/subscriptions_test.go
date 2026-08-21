// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/model"
)

func TestFetchAndLoadSubscriptionSnapshot(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte("vless://00000000-0000-4000-8000-000000000001@example.com:443?security=tls&sni=edge.example.com\n"))
	}))
	defer server.Close()
	client := server.Client()
	configured := model.Subscription{ID: "public", Enabled: true, URL: server.URL}
	directory := t.TempDir()
	snapshot, err := FetchSubscription(context.Background(), client, configured, directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Nodes) != 1 || snapshot.Nodes[0].ID == "" || snapshot.Nodes[0].SourceFingerprint == "" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if _, err := filepath.Abs(SubscriptionSnapshotPath(directory, configured.ID)); err != nil {
		t.Fatal(err)
	}
}

func TestSubscriptionDisappearanceIsPinnedStale(t *testing.T) {
	old := model.Node{ID: "public-old", Enabled: true, Type: "vless", Server: "old.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{UUID: "00000000-0000-4000-8000-000000000001"}}
	fresh := model.Node{Type: "vless", Server: "new.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{UUID: "00000000-0000-4000-8000-000000000002"}}
	merged := mergeSubscriptionNodes("public", []model.Node{old}, []model.Node{fresh})
	stale := false
	for _, node := range merged {
		stale = stale || node.PinnedStale
	}
	if len(merged) != 2 || !stale {
		t.Fatalf("disappeared node was not pinned stale: %#v", merged)
	}
}

func TestUpdateSubscriptionWritesUCIAndDoesNotApply(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte("vless://00000000-0000-4000-8000-000000000001@example.com:443?security=tls&sni=edge.example.com\n"))
	}))
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "steer")
	config := `config steer 'main'
	option schema_version '6'
	option enabled '1'

config bootstrap 'bootstrap'
	option protocol 'udp'
	option server '1.1.1.1'
	option server_port '53'
	option strategy 'prefer_ipv4'

config subscription 'public'
	option enabled '1'
	option url '` + server.URL + `'
`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	var batch string
	result, err := UpdateConfiguredSubscriptionsWithWriter(context.Background(), server.Client(), configPath, t.TempDir(), "public", func(_ context.Context, value string) error { batch = value; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || !strings.Contains(batch, "commit steer\n") || strings.Contains(batch, "apply") || !strings.Contains(batch, "source_subscription") {
		t.Fatalf("unexpected subscription batch: result=%#v batch=%s", result, batch)
	}
}
