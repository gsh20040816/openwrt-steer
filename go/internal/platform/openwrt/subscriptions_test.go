// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	model "github.com/gsh20040816/openwrt-steer/go/internal/intent"
	"github.com/gsh20040816/openwrt-steer/go/internal/subscription"
)

func TestSubscriptionDisappearanceIsPinnedStale(t *testing.T) {
	old := model.Node{ID: "public_old", Enabled: false, Type: "vless", Server: "old.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{UUID: "00000000-0000-4000-8000-000000000001"}}
	fresh := model.Node{Type: "vless", Server: "new.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{UUID: "00000000-0000-4000-8000-000000000002"}}
	merged := subscription.Merge("public", []model.Node{old}, []model.Node{fresh})
	stale := false
	for _, node := range merged {
		if node.PinnedStale {
			stale = true
			if node.Enabled {
				t.Fatalf("disabled stale node was re-enabled: %#v", merged)
			}
		}
	}
	if len(merged) != 2 || !stale {
		t.Fatalf("disappeared node was not pinned stale: %#v", merged)
	}
}

func TestSubscriptionRefreshPreservesDisabledNode(t *testing.T) {
	old := model.Node{ID: "public_node", Enabled: false, Type: "trojan", Server: "same.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{Password: "secret"}}
	fresh := old
	fresh.ID = ""
	fresh.Enabled = true
	merged := subscription.Merge("public", []model.Node{old}, []model.Node{fresh})
	if len(merged) != 1 || merged[0].ID != old.ID || merged[0].Enabled {
		t.Fatalf("subscription refresh did not preserve the local enabled state: %#v", merged)
	}
	var batch strings.Builder
	appendNodeBatch(&batch, merged[0])
	if !strings.Contains(batch.String(), "set steer.public_node.enabled=0\n") {
		t.Fatalf("disabled subscription node was not persisted: %s", batch.String())
	}
}

func TestSubscriptionDuplicateFingerprintsAreCollapsed(t *testing.T) {
	first := model.Node{Name: "First", Type: "trojan", Server: "same.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{Password: "secret"}}
	second := first
	second.Name = "Duplicate label"
	merged := subscription.Merge("public", nil, []model.Node{first, second})
	if len(merged) != 1 || merged[0].Name != "First" {
		t.Fatalf("duplicate subscription identities were not collapsed deterministically: %#v", merged)
	}
}

func TestSubscriptionNodeIDIsAddressableByUCI(t *testing.T) {
	node := model.Node{Type: "socks", Server: "proxy.example", ServerPort: 1080}
	id := subscription.Merge("public", nil, []model.Node{node})[0].ID
	if !strings.HasPrefix(id, "public_") || !uciIdentifier.MatchString(id) || strings.Contains(id, "-") {
		t.Fatalf("subscription node ID is not a strict UCI identifier: %q", id)
	}
}

func TestUpdateSubscriptionWritesUCIAndDoesNotApply(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte("vless://00000000-0000-4000-8000-000000000001@example.com:443?security=tls&sni=edge.example.com\n"))
	}))
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "steer")
	config := validSubscriptionConfig(server.URL)
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
	if strings.Contains(batch, ".enabled=1\n") || strings.Contains(batch, ".pinned_stale=0\n") {
		t.Fatalf("subscription batch persisted default boolean values: %s", batch)
	}
}

func TestUpdateSubscriptionValidatesCandidateBeforeCommit(t *testing.T) {
	vmess := `{"v":"2","ps":"invalid","add":"vmess.example.com","port":"443","id":"00000000-0000-4000-8000-000000000001","aid":"0","scy":"auto","net":"kcp","tls":"none"}`
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(base64.StdEncoding.EncodeToString([]byte(vmess))))
	}))
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "steer")
	if err := os.WriteFile(configPath, []byte(validSubscriptionConfig(server.URL)), 0o600); err != nil {
		t.Fatal(err)
	}
	committed := false
	_, err := UpdateConfiguredSubscriptionsWithWriter(context.Background(), server.Client(), configPath, t.TempDir(), "public", func(context.Context, string) error {
		committed = true
		return nil
	})
	if err == nil || committed || !strings.Contains(err.Error(), "invalid candidate") {
		t.Fatalf("invalid subscription candidate reached UCI: committed=%v err=%v", committed, err)
	}
}

func validSubscriptionConfig(subscriptionURL string) string {
	return `config steer 'main'
	option schema_version '7'
	option enabled '1'
	option log_level 'warn'
	option probe_direct 'https://www.baidu.com/'
	option probe_proxy 'https://www.google.com/generate_204'
	option speedtest_proxy 'https://speed.cloudflare.com/__down?bytes=1000000'

config bootstrap 'bootstrap'
	option protocol 'udp'
	option server '1.1.1.1'
	option server_port '53'
	option strategy 'prefer_ipv4'

config route 'direct'
	option kind 'direct'

config route 'block'
	option kind 'block'

config dns_profile 'resolver'
	option protocol 'udp'
	option server '1.1.1.1'
	option server_port '53'
	option strategy 'prefer_ipv4'

config rule 'default'
	option default '1'
	option dns_profile 'resolver'
	option route 'direct'

config subscription 'public'
	option enabled '1'
	option url '` + subscriptionURL + `'
`
}
