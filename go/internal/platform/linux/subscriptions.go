// SPDX-License-Identifier: GPL-3.0-or-later

package linux

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

func SubscriptionSnapshotPath(stateDirectory, id string) string {
	if stateDirectory == "" {
		stateDirectory = "/var/lib/steer"
	}
	return filepath.Join(stateDirectory, "subscriptions", id+".json")
}

func ReadSubscriptionStatus(configPath, stateDirectory string) ([]SubscriptionStatus, error) {
	intent, _, err := (IntentStore{Path: configPath}).Load()
	if err != nil {
		return nil, err
	}
	statuses := make([]SubscriptionStatus, 0, len(intent.Subscriptions))
	for _, configured := range intent.Subscriptions {
		status := SubscriptionStatus{ID: configured.ID, Name: configured.Name, URL: configured.URL, Enabled: configured.Enabled, UpdateInterval: configured.UpdateInterval}
		snapshot, readErr := readSubscriptionSnapshot(SubscriptionSnapshotPath(stateDirectory, configured.ID))
		if readErr != nil {
			status.Error = "not fetched: " + readErr.Error()
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
	store := IntentStore{Path: configPath}
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
			if previous, readErr := readSubscriptionSnapshot(SubscriptionSnapshotPath(stateDirectory, configured.ID)); readErr == nil && !previous.FetchedAt.IsZero() && time.Since(previous.FetchedAt) < interval {
				continue
			}
		}
		fetched, err := subscription.Fetch(ctx, client, configured)
		if err != nil {
			return nil, fmt.Errorf("update subscription %s: %w", configured.ID, err)
		}
		old := make([]model.Node, 0)
		for _, node := range value.Nodes {
			if node.SourceSubscription == configured.ID {
				old = append(old, node)
			}
		}
		merged := []model.Node{}
		if len(fetched.Nodes) > 0 {
			merged = subscription.Merge(configured.ID, old, fetched.Nodes)
		}
		value.Nodes = subscription.Replace(value.Nodes, configured.ID, merged)
		snapshots = append(snapshots, SubscriptionSnapshot{
			SubscriptionID: configured.ID, URL: configured.URL, FetchedAt: time.Now().UTC(), Nodes: merged, Skipped: fetched.Skipped,
		})
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
	if !safePathID(id) || !safePathID(nodeID) {
		return SubscriptionSnapshot{}, fmt.Errorf("subscription and node IDs must be simple identifiers")
	}
	path := SubscriptionSnapshotPath(stateDirectory, id)
	snapshot, err := readSubscriptionSnapshot(path)
	if err != nil {
		return SubscriptionSnapshot{}, fmt.Errorf("read subscription snapshot: %w", err)
	}
	filtered := make([]model.Node, 0, len(snapshot.Nodes))
	var snapshotNode *model.Node
	for _, node := range snapshot.Nodes {
		if node.ID == nodeID && node.SourceSubscription == id {
			candidate := node
			snapshotNode = &candidate
			continue
		}
		filtered = append(filtered, node)
	}
	if snapshotNode == nil || !snapshotNode.PinnedStale {
		return SubscriptionSnapshot{}, fmt.Errorf("stale subscription node %q was not found", nodeID)
	}
	store := IntentStore{Path: configPath}
	value, revision, err := store.Load()
	if err != nil {
		return SubscriptionSnapshot{}, err
	}
	candidate := value
	candidate.Nodes = make([]model.Node, 0, len(value.Nodes))
	found := false
	for _, node := range value.Nodes {
		if node.ID == nodeID && node.SourceSubscription == id {
			if !node.PinnedStale {
				return SubscriptionSnapshot{}, fmt.Errorf("subscription node %q is current and cannot be removed", nodeID)
			}
			found = true
			continue
		}
		candidate.Nodes = append(candidate.Nodes, node)
	}
	if !found {
		return SubscriptionSnapshot{}, fmt.Errorf("subscription node %q is not present in canonical intent", nodeID)
	}
	validation := Validate(candidate)
	if !validation.OK {
		for _, issue := range validation.Errors {
			if issue.Code == "DANGLING_NODE" && issue.ObjectType == "route" && issue.Option == "node" {
				return SubscriptionSnapshot{}, fmt.Errorf("NODE_STILL_REFERENCED: route %q still references subscription node %q", issue.ObjectID, nodeID)
			}
		}
		return SubscriptionSnapshot{}, ValidationError{Validation: validation}
	}
	if _, err := store.Save(candidate, revision); err != nil {
		return SubscriptionSnapshot{}, err
	}
	snapshot.Nodes = filtered
	if err := saveSubscriptionSnapshot(stateDirectory, snapshot); err != nil {
		return SubscriptionSnapshot{}, err
	}
	return snapshot, nil
}

func safePathID(value string) bool {
	return value != "" && filepath.Base(value) == value && value != "." && value != ".."
}

func readSubscriptionSnapshot(path string) (SubscriptionSnapshot, error) {
	file, err := os.Open(path)
	if err != nil {
		return SubscriptionSnapshot{}, err
	}
	defer file.Close()
	var snapshot SubscriptionSnapshot
	if err := json.NewDecoder(file).Decode(&snapshot); err != nil {
		return SubscriptionSnapshot{}, err
	}
	return snapshot, nil
}

func saveSubscriptionSnapshot(stateDirectory string, snapshot SubscriptionSnapshot) error {
	path := SubscriptionSnapshotPath(stateDirectory, snapshot.SubscriptionID)
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encode subscription snapshot: %w", err)
	}
	if err := atomicWrite(path, append(encoded, '\n')); err != nil {
		return fmt.Errorf("save subscription snapshot: %w", err)
	}
	return nil
}
