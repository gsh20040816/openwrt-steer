// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	model "github.com/gsh20040816/steer/go/internal/intent"
	"github.com/gsh20040816/steer/go/internal/subscription"
)

type SubscriptionSnapshot = subscription.Snapshot

type SubscriptionStatus struct {
	ID             string    `json:"id"`
	Name           string    `json:"name,omitempty"`
	URL            string    `json:"url"`
	Enabled        bool      `json:"enabled"`
	UpdateInterval string    `json:"update_interval,omitempty"`
	FetchedAt      time.Time `json:"fetched_at,omitempty"`
	NodeCount      int       `json:"node_count"`
	Skipped        int       `json:"skipped"`
	StaleNodeIDs   []string  `json:"stale_node_ids,omitempty"`
	Error          string    `json:"error,omitempty"`
}

var _ subscription.Store = IntentStore{}

func (store IntentStore) ReplaceNodes(_ context.Context, subscriptionID string, existing, replacement []model.Node) error {
	value, revision, err := store.Load()
	if err != nil {
		return err
	}
	value.Nodes = subscription.Replace(value.Nodes, subscriptionID, replacement)
	_, err = store.Save(value, revision)
	return err
}

func (store IntentStore) RemoveNode(_ context.Context, subscriptionID, nodeID string) error {
	value, revision, err := store.Load()
	if err != nil {
		return err
	}
	filtered := make([]model.Node, 0, len(value.Nodes))
	for _, node := range value.Nodes {
		if node.ID == nodeID && node.SourceSubscription == subscriptionID {
			continue
		}
		filtered = append(filtered, node)
	}
	value.Nodes = filtered
	_, err = store.Save(value, revision)
	return err
}

func existingNodes(nodes []model.Node, subscriptionID string) []model.Node {
	result := make([]model.Node, 0, len(nodes))
	for _, node := range nodes {
		if node.SourceSubscription == subscriptionID {
			result = append(result, node)
		}
	}
	return result
}

type SubscriptionUpdate struct {
	SubscriptionID string `json:"subscription_id"`
	NodeCount      int    `json:"node_count"`
	Skipped        int    `json:"skipped"`
}

func UpdateSubscription(ctx context.Context, client *http.Client, store IntentStore, subscriptionID string) (SubscriptionUpdate, error) {
	value, _, err := store.Load()
	if err != nil {
		return SubscriptionUpdate{}, err
	}
	var configured *model.Subscription
	for index := range value.Subscriptions {
		if value.Subscriptions[index].ID == subscriptionID {
			configured = &value.Subscriptions[index]
			break
		}
	}
	if configured == nil || !configured.Enabled {
		return SubscriptionUpdate{}, fmt.Errorf("enabled subscription %q was not found", subscriptionID)
	}
	parsed, err := subscription.Fetch(ctx, client, *configured)
	if err != nil {
		return SubscriptionUpdate{}, err
	}
	old := existingNodes(value.Nodes, subscriptionID)
	merged := subscription.Merge(subscriptionID, old, parsed.Nodes)
	if err := store.ReplaceNodes(ctx, subscriptionID, old, merged); err != nil {
		return SubscriptionUpdate{}, err
	}
	return SubscriptionUpdate{SubscriptionID: subscriptionID, NodeCount: len(parsed.Nodes), Skipped: parsed.Skipped}, nil
}

func subscriptionSnapshotPath(stateDirectory, id string) string {
	return filepath.Join(stateDirectory, "subscriptions", id+".json")
}

func ReadSubscriptionStatus(configPath, stateDirectory string) ([]SubscriptionStatus, error) {
	value, _, err := (IntentStore{Paths: Paths{ConfigPath: configPath}}).Load()
	if err != nil {
		return nil, err
	}
	statuses := make([]SubscriptionStatus, 0, len(value.Subscriptions))
	for _, configured := range value.Subscriptions {
		status := SubscriptionStatus{ID: configured.ID, Name: configured.Name, URL: configured.URL, Enabled: configured.Enabled, UpdateInterval: configured.UpdateInterval}
		snapshot, readErr := readSubscriptionSnapshot(subscriptionSnapshotPath(stateDirectory, configured.ID))
		if readErr != nil {
			status.Error = "not fetched: " + readErr.Error()
			for _, node := range value.Nodes {
				if node.SourceSubscription != configured.ID {
					continue
				}
				status.NodeCount++
				if node.PinnedStale {
					status.StaleNodeIDs = append(status.StaleNodeIDs, node.ID)
				}
			}
		} else {
			status.FetchedAt, status.NodeCount, status.Skipped = snapshot.FetchedAt, len(snapshot.Nodes), snapshot.Skipped
			for _, node := range snapshot.Nodes {
				if node.PinnedStale {
					status.StaleNodeIDs = append(status.StaleNodeIDs, node.ID)
				}
			}
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func UpdateConfiguredSubscriptions(ctx context.Context, client *http.Client, configPath, stateDirectory, id string) ([]SubscriptionSnapshot, error) {
	store := IntentStore{Paths: Paths{ConfigPath: configPath}}
	value, revision, err := store.Load()
	if err != nil {
		return nil, err
	}
	var snapshots []SubscriptionSnapshot
	for _, configured := range value.Subscriptions {
		if !configured.Enabled || (id != "" && configured.ID != id) {
			continue
		}
		if id == "" && configured.UpdateInterval != "" {
			interval, parseErr := time.ParseDuration(configured.UpdateInterval)
			if parseErr != nil {
				return nil, fmt.Errorf("subscription %s has invalid update interval: %w", configured.ID, parseErr)
			}
			if previous, readErr := readSubscriptionSnapshot(subscriptionSnapshotPath(stateDirectory, configured.ID)); readErr == nil && !previous.FetchedAt.IsZero() && time.Since(previous.FetchedAt) < interval {
				continue
			}
		}
		fetched, err := subscription.Fetch(ctx, client, configured)
		if err != nil {
			return nil, fmt.Errorf("update subscription %s: %w", configured.ID, err)
		}
		old := existingNodes(value.Nodes, configured.ID)
		merged := subscription.Merge(configured.ID, old, fetched.Nodes)
		value.Nodes = subscription.Replace(value.Nodes, configured.ID, merged)
		snapshots = append(snapshots, SubscriptionSnapshot{SubscriptionID: configured.ID, URL: configured.URL, FetchedAt: time.Now().UTC(), Nodes: merged, Skipped: fetched.Skipped})
	}
	if id != "" && len(snapshots) == 0 {
		return nil, fmt.Errorf("enabled subscription %q was not found", id)
	}
	if len(snapshots) == 0 {
		return snapshots, nil
	}
	if _, err := store.Save(value, revision); err != nil {
		return nil, err
	}
	for _, snapshot := range snapshots {
		if err := saveSubscriptionSnapshot(stateDirectory, snapshot); err != nil {
			return nil, err
		}
	}
	return snapshots, nil
}

func CleanSubscriptionNode(configPath, stateDirectory, id, nodeID string) (SubscriptionSnapshot, error) {
	path := subscriptionSnapshotPath(stateDirectory, id)
	snapshot, err := readSubscriptionSnapshot(path)
	if err != nil {
		return SubscriptionSnapshot{}, err
	}
	store := IntentStore{Paths: Paths{ConfigPath: configPath}}
	value, revision, err := store.Load()
	if err != nil {
		return SubscriptionSnapshot{}, err
	}
	filtered := make([]model.Node, 0, len(value.Nodes))
	found := false
	for _, node := range value.Nodes {
		if node.ID == nodeID && node.SourceSubscription == id {
			if !node.PinnedStale {
				return SubscriptionSnapshot{}, fmt.Errorf("subscription node %q is current and cannot be removed", nodeID)
			}
			found = true
			continue
		}
		filtered = append(filtered, node)
	}
	if !found {
		return SubscriptionSnapshot{}, fmt.Errorf("stale subscription node %q was not found", nodeID)
	}
	value.Nodes = filtered
	validation := Validate(value)
	if !validation.OK {
		for _, issue := range validation.Errors {
			if issue.Code == "DANGLING_NODE" {
				return SubscriptionSnapshot{}, fmt.Errorf("NODE_STILL_REFERENCED: route %q still references subscription node %q", issue.ObjectID, nodeID)
			}
		}
		return SubscriptionSnapshot{}, ValidationError{Validation: validation}
	}
	if _, err := store.Save(value, revision); err != nil {
		return SubscriptionSnapshot{}, err
	}
	snapshotNodes := make([]model.Node, 0, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		if node.ID == nodeID && node.SourceSubscription == id {
			continue
		}
		snapshotNodes = append(snapshotNodes, node)
	}
	snapshot.Nodes = snapshotNodes
	if err := saveSubscriptionSnapshot(stateDirectory, snapshot); err != nil {
		return SubscriptionSnapshot{}, err
	}
	return snapshot, nil
}

func readSubscriptionSnapshot(path string) (SubscriptionSnapshot, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return SubscriptionSnapshot{}, err
	}
	var snapshot SubscriptionSnapshot
	if err := json.Unmarshal(content, &snapshot); err != nil {
		return SubscriptionSnapshot{}, err
	}
	return snapshot, nil
}

func saveSubscriptionSnapshot(stateDirectory string, snapshot SubscriptionSnapshot) error {
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(subscriptionSnapshotPath(stateDirectory, snapshot.SubscriptionID), append(encoded, '\n'))
}
