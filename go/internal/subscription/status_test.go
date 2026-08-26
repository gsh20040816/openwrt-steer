// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"fmt"
	"io/fs"
	"testing"
	"time"

	model "github.com/gsh20040816/steer/go/internal/intent"
)

func TestBuildStatusKeepsSuccessFailureAndStaleReferencesSeparate(t *testing.T) {
	success := time.Date(2026, time.August, 26, 1, 0, 0, 0, time.UTC)
	failureAt := success.Add(time.Hour)
	snapshot := Snapshot{
		SubscriptionID: "feed", FetchedAt: success, Skipped: 2, Added: 1,
		LastFailure: &Failure{At: &failureAt, Summary: "subscription server returned HTTP 503"},
	}
	nodes := []model.Node{
		{ID: "feed_current", Name: "Current", NodeSource: model.NodeSource{SourceSubscription: "feed"}},
		{ID: "feed_blocked", Name: "Blocked", NodeSource: model.NodeSource{SourceSubscription: "feed", PinnedStale: true}},
		{ID: "feed_removable", Name: "Removable", NodeSource: model.NodeSource{SourceSubscription: "feed", PinnedStale: true}},
	}
	status := BuildStatus(
		model.Subscription{ID: "feed", Enabled: true, Name: "Feed", URL: "https://example.test/sub"},
		nodes,
		[]model.Route{{ID: "proxy", Name: "Proxy", Node: "feed_blocked"}},
		&snapshot,
		nil,
	)
	if status.NeverFetched || status.LastSuccess == nil || !status.LastSuccess.Equal(success) || status.LastFailure == nil {
		t.Fatalf("success/failure facts drifted: %#v", status)
	}
	if status.NodeCount != 3 || status.Current != 1 || status.Added != 1 || status.Skipped != 2 || len(status.Stale) != 2 {
		t.Fatalf("inventory facts drifted: %#v", status)
	}
	if status.Stale[0].ID != "feed_blocked" || len(status.Stale[0].ReferencedBy) != 1 || status.Stale[0].ReferencedBy[0].ID != "proxy" {
		t.Fatalf("stale references drifted: %#v", status.Stale)
	}
	if status.Stale[1].ID != "feed_removable" || len(status.Stale[1].ReferencedBy) != 0 {
		t.Fatalf("independent stale cleanup fact drifted: %#v", status.Stale)
	}
}

func TestBuildStatusRepresentsNeverFetchedWithoutYearOne(t *testing.T) {
	status := BuildStatus(model.Subscription{ID: "new", Enabled: true, URL: "https://example.test/sub"}, nil, nil, nil, fs.ErrNotExist)
	if !status.NeverFetched || status.LastSuccess != nil || status.LastFailure != nil {
		t.Fatalf("unexpected never-fetched status: %#v", status)
	}
}

func TestSnapshotsPreserveFailureHistoryAndCountAddedNodes(t *testing.T) {
	configured := model.Subscription{ID: "feed", URL: "https://example.test/sub"}
	failureTime := time.Date(2026, time.August, 26, 2, 0, 0, 0, time.UTC)
	failed := FailedSnapshot(configured, nil, fmt.Errorf("subscription returned HTTP 503"), failureTime)
	old := []model.Node{{ID: "feed_old", NodeSource: model.NodeSource{SourceSubscription: "feed"}}}
	merged := []model.Node{
		{ID: "feed_old", NodeSource: model.NodeSource{SourceSubscription: "feed"}},
		{ID: "feed_new", NodeSource: model.NodeSource{SourceSubscription: "feed"}},
		{ID: "feed_stale", NodeSource: model.NodeSource{SourceSubscription: "feed", PinnedStale: true}},
	}
	success := SuccessfulSnapshot(configured, &failed, old, merged, ParseResult{Skipped: 3}, failureTime.Add(time.Hour))
	if success.LastFailure == nil || success.LastFailure.Summary != "subscription server returned HTTP 503" || success.Added != 1 || success.Skipped != 3 {
		t.Fatalf("success destroyed failure or update facts: %#v", success)
	}
}

func TestFailureSummaryNeverPersistsConfiguredURLSecrets(t *testing.T) {
	secret := "https://user:token@example.test/private?key=secret"
	summary := SafeFailureSummary(fmt.Errorf("download subscription: Get %q: dial failed", secret))
	if summary != "subscription download failed" {
		t.Fatalf("unexpected safe summary %q", summary)
	}
	for _, value := range []string{"user", "token", "private", "secret", "example.test"} {
		if contains := stringContains(summary, value); contains {
			t.Fatalf("safe summary leaked %q: %q", value, summary)
		}
	}
}

func stringContains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
