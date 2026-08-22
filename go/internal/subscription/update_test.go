// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	model "github.com/gsh20040816/openwrt-steer/go/internal/intent"
)

func TestFetchPublicHTTPSSubscription(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte("vless://00000000-0000-4000-8000-000000000001@example.com:443?security=tls&sni=edge.example.com\n"))
	}))
	defer server.Close()
	nodes, err := Fetch(context.Background(), server.Client(), model.Subscription{ID: "public", Enabled: true, URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Type != "vless" {
		t.Fatalf("unexpected fetched nodes: %#v", nodes)
	}
}

func TestMergePreservesUserStateAndPinsStaleNodes(t *testing.T) {
	fresh := model.Node{Enabled: true, Type: "socks", Server: "proxy.example", ServerPort: 1080}
	fingerprint := Fingerprint(fresh)
	old := fresh
	old.ID, old.Enabled, old.SourceFingerprint = "feed_existing", false, fingerprint
	stale := model.Node{ID: "feed_stale", Enabled: true, Type: "socks", Server: "stale.example", ServerPort: 1080}
	merged := Merge("feed", []model.Node{old, stale}, []model.Node{fresh})
	if len(merged) != 2 || merged[0].ID != "feed_existing" || merged[0].Enabled || !merged[1].PinnedStale {
		t.Fatalf("unexpected merge: %#v", merged)
	}
}

func TestFetchRejectsSubscriptionLargerThanLimit(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write(bytes.Repeat([]byte{'x'}, maxSubscriptionBytes+1))
	}))
	defer server.Close()
	_, err := Fetch(context.Background(), server.Client(), model.Subscription{ID: "oversized", Enabled: true, URL: server.URL})
	if err == nil || err.Error() != "subscription exceeds the 16 MiB size limit" {
		t.Fatalf("oversized subscription was not rejected explicitly: %v", err)
	}
}
