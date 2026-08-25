// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"context"
	"fmt"
	"net/http"

	model "github.com/gsh20040816/steer/go/internal/intent"
	"github.com/gsh20040816/steer/go/internal/subscription"
)

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
