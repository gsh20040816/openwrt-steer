// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	model "github.com/gsh20040816/steer/go/internal/intent"
)

func TestAutomaticUpdateDue(t *testing.T) {
	now := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		interval  string
		fetchedAt time.Time
		wantDue   bool
		wantErr   bool
	}{
		{name: "empty interval is manual only"},
		{name: "empty interval ignores old snapshot", fetchedAt: now.Add(-24 * time.Hour)},
		{name: "first scheduled fetch", interval: "6h", wantDue: true},
		{name: "not due", interval: "6h", fetchedAt: now.Add(-6*time.Hour + time.Nanosecond)},
		{name: "exactly due", interval: "6h", fetchedAt: now.Add(-6 * time.Hour), wantDue: true},
		{name: "overdue", interval: "6h", fetchedAt: now.Add(-7 * time.Hour), wantDue: true},
		{name: "invalid duration", interval: "later", wantErr: true},
		{name: "zero duration", interval: "0s", wantErr: true},
		{name: "negative duration", interval: "-1h", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			due, err := AutomaticUpdateDue(test.interval, test.fetchedAt, now)
			if (err != nil) != test.wantErr || due != test.wantDue {
				t.Fatalf("AutomaticUpdateDue(%q, %v, %v) = %v, %v; want due=%v err=%v", test.interval, test.fetchedAt, now, due, err, test.wantDue, test.wantErr)
			}
		})
	}
}

func TestFetchHTTPSubscription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte("vless://00000000-0000-4000-8000-000000000001@example.com:443?security=tls&sni=edge.example.com\n"))
	}))
	defer server.Close()
	result, err := Fetch(context.Background(), server.Client(), model.Subscription{ID: "public", Enabled: true, URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 1 || result.Nodes[0].Type != "vless" || result.Skipped != 0 {
		t.Fatalf("unexpected fetched nodes: %#v", result)
	}
}

func TestFetchFollowsHTTPSRedirectToHTTP(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte("socks5://user:password@example.com:1080\n"))
	}))
	defer target.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, target.URL, http.StatusFound)
	}))
	defer redirect.Close()
	result, err := Fetch(context.Background(), redirect.Client(), model.Subscription{ID: "redirect", Enabled: true, URL: redirect.URL})
	if err != nil || len(result.Nodes) != 1 || result.Nodes[0].Type != "socks" {
		t.Fatalf("HTTPS to HTTP redirect was not accepted: result=%#v err=%v", result, err)
	}
}

func TestFetchAllowsCredentialsAndFragment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		username, password, ok := request.BasicAuth()
		if !ok || username != "user" || password != "password" {
			http.Error(response, "missing credentials", http.StatusUnauthorized)
			return
		}
		_, _ = response.Write([]byte("socks://example.com:1080\n"))
	}))
	defer server.Close()
	address := strings.Replace(server.URL, "http://", "http://user:password@", 1) + "#client-only"
	result, err := Fetch(context.Background(), server.Client(), model.Subscription{ID: "authenticated", Enabled: true, URL: address})
	if err != nil || len(result.Nodes) != 1 {
		t.Fatalf("ordinary authenticated URL was rejected: result=%#v err=%v", result, err)
	}
}

func TestMergePreservesUserStatePinsReferencedStaleAndDropsUnreferencedNodes(t *testing.T) {
	fresh := model.Node{Enabled: true, Type: "socks", Server: "proxy.example", ServerPort: 1080}
	fingerprint := Fingerprint(fresh)
	old := fresh
	old.ID, old.Enabled, old.SourceFingerprint = "feed_existing", false, fingerprint
	referenced := model.Node{ID: "feed_referenced", Enabled: true, Type: "socks", Server: "referenced.example", ServerPort: 1080}
	unreferenced := model.Node{ID: "feed_unreferenced", Enabled: true, Type: "socks", Server: "unreferenced.example", ServerPort: 1080}
	routes := []model.Route{{ID: "proxy", Kind: "single", Node: referenced.ID}}
	merged := Merge("feed", []model.Node{old, referenced, unreferenced}, []model.Node{fresh}, routes)
	if len(merged) != 2 || merged[0].ID != "feed_existing" || merged[0].Enabled || merged[1].ID != referenced.ID || !merged[1].PinnedStale {
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

func TestFetchRejectsHTTP200WithNoValidNodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte("not-a-node\nstill-not-a-node\n"))
	}))
	defer server.Close()
	_, err := Fetch(context.Background(), server.Client(), model.Subscription{ID: "empty", Enabled: true, URL: server.URL})
	if err == nil || !strings.Contains(err.Error(), "no valid nodes") {
		t.Fatalf("invalid HTTP 200 body was accepted: %v", err)
	}
}

func TestFetchRejectsHTTP200EmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {}))
	defer server.Close()
	_, err := Fetch(context.Background(), server.Client(), model.Subscription{ID: "empty-body", Enabled: true, URL: server.URL})
	if err == nil || !strings.Contains(err.Error(), "no valid nodes") {
		t.Fatalf("empty HTTP 200 body was accepted: %v", err)
	}
}
