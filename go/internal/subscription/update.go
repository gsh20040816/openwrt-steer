// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	model "github.com/gsh20040816/steer/go/internal/intent"
)

// Store is the only mutable configuration boundary required by shared
// subscription logic. UCI and future JSON adapters implement it differently.
type Store interface {
	ReplaceNodes(context.Context, string, []model.Node, []model.Node) error
	RemoveNode(context.Context, string, string) error
}

type Change struct {
	SubscriptionID string
	Existing       []model.Node
	Replacement    []model.Node
}

func Persist(ctx context.Context, store Store, changes []Change) error {
	for _, change := range changes {
		if err := store.ReplaceNodes(ctx, change.SubscriptionID, change.Existing, change.Replacement); err != nil {
			return err
		}
	}
	return nil
}

type Snapshot struct {
	SubscriptionID string       `json:"subscription_id"`
	URL            string       `json:"url"`
	FetchedAt      time.Time    `json:"fetched_at"`
	Nodes          []model.Node `json:"nodes"`
	Skipped        int          `json:"skipped"`
}

const maxSubscriptionBytes = 16 << 20

func Fetch(ctx context.Context, client *http.Client, configured model.Subscription) (ParseResult, error) {
	parsedURL, parseErr := url.Parse(configured.URL)
	if parseErr != nil || (!strings.EqualFold(parsedURL.Scheme, "http") && !strings.EqualFold(parsedURL.Scheme, "https")) || parsedURL.Host == "" {
		return ParseResult{}, fmt.Errorf("subscription URL must be an absolute HTTP or HTTPS URL")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, configured.URL, nil)
	if err != nil {
		return ParseResult{}, fmt.Errorf("create subscription request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return ParseResult{}, fmt.Errorf("download subscription: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ParseResult{}, fmt.Errorf("subscription returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxSubscriptionBytes+1))
	if err != nil {
		return ParseResult{}, fmt.Errorf("read subscription: %w", err)
	}
	if len(body) > maxSubscriptionBytes {
		return ParseResult{}, fmt.Errorf("subscription exceeds the 16 MiB size limit")
	}
	parsed, err := ParseList(string(body))
	if err != nil {
		return ParseResult{}, err
	}
	if len(parsed.Nodes) == 0 {
		return ParseResult{}, fmt.Errorf("subscription contains no valid nodes")
	}
	return parsed, nil
}

func Merge(subscriptionID string, old, fresh []model.Node) []model.Node {
	oldByFingerprint := make(map[string]model.Node, len(old))
	for _, node := range old {
		oldByFingerprint[Fingerprint(node)] = node
	}
	seen := make(map[string]bool, len(fresh))
	merged := make([]model.Node, 0, len(old)+len(fresh))
	for _, node := range fresh {
		fingerprint := Fingerprint(node)
		if seen[fingerprint] {
			continue
		}
		previous, exists := oldByFingerprint[fingerprint]
		enabled := true
		if exists {
			node.ID = previous.ID
			enabled = previous.Enabled
		}
		if node.ID == "" {
			node.ID = stableNodeID(subscriptionID, fingerprint)
		}
		node.Enabled, node.SourceSubscription, node.SourceFingerprint, node.PinnedStale = enabled, subscriptionID, fingerprint, false
		merged = append(merged, node)
		seen[fingerprint] = true
	}
	for _, node := range old {
		fingerprint := Fingerprint(node)
		if seen[fingerprint] {
			continue
		}
		node.SourceSubscription, node.SourceFingerprint, node.PinnedStale = subscriptionID, fingerprint, true
		merged = append(merged, node)
	}
	sort.SliceStable(merged, func(left, right int) bool { return merged[left].ID < merged[right].ID })
	return merged
}

func Replace(existing []model.Node, subscriptionID string, replacement []model.Node) []model.Node {
	result := make([]model.Node, 0, len(existing)+len(replacement))
	for _, node := range existing {
		if node.SourceSubscription != subscriptionID {
			result = append(result, node)
		}
	}
	return append(result, replacement...)
}

func stableNodeID(subscriptionID, fingerprint string) string {
	prefix := subscriptionID
	if len(prefix) > 19 {
		prefix = prefix[:19]
	}
	return prefix + "_" + fingerprint[:12]
}
