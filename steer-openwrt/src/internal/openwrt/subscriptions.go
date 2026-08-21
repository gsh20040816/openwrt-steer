// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/model"
	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/subscription"
)

type SubscriptionSnapshot struct {
	SubscriptionID string       `json:"subscription_id"`
	URL            string       `json:"url"`
	FetchedAt      time.Time    `json:"fetched_at"`
	Nodes          []model.Node `json:"nodes"`
}

type SubscriptionStatus struct {
	ID             string    `json:"id"`
	Name           string    `json:"name,omitempty"`
	URL            string    `json:"url"`
	Enabled        bool      `json:"enabled"`
	UpdateInterval string    `json:"update_interval,omitempty"`
	FetchedAt      time.Time `json:"fetched_at,omitempty"`
	NodeCount      int       `json:"node_count"`
	StaleNodeIDs   []string  `json:"stale_node_ids,omitempty"`
	Error          string    `json:"error,omitempty"`
}

var uciIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

func ReadSubscriptionStatus(configPath, stateDirectory string) ([]SubscriptionStatus, error) {
	config, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read UCI for subscription status: %w", err)
	}
	intent, err := DecodeBytes(config)
	if err != nil {
		return nil, err
	}
	statuses := make([]SubscriptionStatus, 0, len(intent.Subscriptions))
	for _, configured := range intent.Subscriptions {
		status := SubscriptionStatus{ID: configured.ID, Name: configured.Name, URL: configured.URL, Enabled: configured.Enabled, UpdateInterval: configured.UpdateInterval}
		if !uciIdentifier.MatchString(configured.ID) {
			status.Error = "unsafe UCI section ID"
			statuses = append(statuses, status)
			continue
		}
		snapshot, readErr := readSubscriptionSnapshot(SubscriptionSnapshotPath(stateDirectory, configured.ID))
		if readErr != nil {
			status.Error = "not fetched: " + readErr.Error()
		} else {
			status.FetchedAt, status.NodeCount = snapshot.FetchedAt, len(snapshot.Nodes)
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

func SubscriptionSnapshotPath(stateDirectory, id string) string {
	return filepath.Join(stateDirectory, "subscriptions", id+".json")
}

func FetchSubscription(ctx context.Context, client *http.Client, configured model.Subscription, stateDirectory string) (SubscriptionSnapshot, error) {
	if !uciIdentifier.MatchString(configured.ID) {
		return SubscriptionSnapshot{}, fmt.Errorf("subscription %q has an unsafe UCI section ID", configured.ID)
	}
	nodes, err := fetchSubscriptionNodes(ctx, client, configured)
	if err != nil {
		return SubscriptionSnapshot{}, err
	}
	path := SubscriptionSnapshotPath(stateDirectory, configured.ID)
	old, _ := readSubscriptionSnapshot(path)
	nodes = mergeSubscriptionNodes(configured.ID, old.Nodes, nodes)
	snapshot := SubscriptionSnapshot{SubscriptionID: configured.ID, URL: configured.URL, FetchedAt: time.Now().UTC(), Nodes: nodes}
	if err := saveSubscriptionSnapshot(stateDirectory, snapshot); err != nil {
		return SubscriptionSnapshot{}, err
	}
	return snapshot, nil
}

func fetchSubscriptionNodes(ctx context.Context, client *http.Client, configured model.Subscription) ([]model.Node, error) {
	parsedURL, parseErr := url.Parse(configured.URL)
	if parseErr != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" || parsedURL.User != nil || parsedURL.Fragment != "" {
		return nil, fmt.Errorf("subscription URL must be public HTTPS without credentials or fragment")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, configured.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("create subscription request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download subscription: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("subscription returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("read subscription: %w", err)
	}
	nodes, err := subscription.ParseList(string(body))
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

func UpdateConfiguredSubscriptions(ctx context.Context, client *http.Client, configPath, stateDirectory string, id string) ([]SubscriptionSnapshot, error) {
	return UpdateConfiguredSubscriptionsWithWriter(ctx, client, configPath, stateDirectory, id, SystemUCIWriter(configPath))
}

// UCIWriter applies one complete batch and must include the commit operation.
// Subscription refresh intentionally uses this narrow path: it changes the
// node list in UCI, but never invokes Apply or emits an ubus event.
type UCIWriter func(context.Context, string) error

func SystemUCIWriter(configPath string) UCIWriter {
	configDirectory := filepath.Dir(configPath)
	return func(ctx context.Context, batch string) error {
		command := exec.CommandContext(ctx, "/sbin/uci", "-c", configDirectory, "batch")
		command.Stdin = strings.NewReader(batch)
		output, err := command.CombinedOutput()
		if err != nil {
			return fmt.Errorf("uci batch: %w: %s", err, strings.TrimSpace(string(output)))
		}
		if detail := strings.TrimSpace(string(output)); detail != "" {
			return fmt.Errorf("uci batch reported an error: %s", detail)
		}
		return nil
	}
}

func UpdateConfiguredSubscriptionsWithWriter(ctx context.Context, client *http.Client, configPath, stateDirectory string, id string, writeUCI UCIWriter) ([]SubscriptionSnapshot, error) {
	if writeUCI == nil {
		return nil, fmt.Errorf("subscription update requires a UCI writer")
	}
	config, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read UCI for subscription update: %w", err)
	}
	intent, err := DecodeBytes(config)
	if err != nil {
		return nil, err
	}
	var result []SubscriptionSnapshot
	var batch strings.Builder
	for _, configured := range intent.Subscriptions {
		if !configured.Enabled || (id != "" && configured.ID != id) {
			continue
		}
		if !uciIdentifier.MatchString(configured.ID) {
			return nil, fmt.Errorf("subscription %q has an unsafe UCI section ID", configured.ID)
		}
		if id == "" && configured.UpdateInterval != "" {
			if interval, parseErr := time.ParseDuration(configured.UpdateInterval); parseErr != nil {
				return nil, fmt.Errorf("subscription %s has invalid update interval: %w", configured.ID, parseErr)
			} else if previous, readErr := readSubscriptionSnapshot(SubscriptionSnapshotPath(stateDirectory, configured.ID)); readErr == nil && !previous.FetchedAt.IsZero() && time.Since(previous.FetchedAt) < interval {
				continue
			}
		}
		fresh, err := fetchSubscriptionNodes(ctx, client, configured)
		if err != nil {
			return nil, fmt.Errorf("update subscription %s: %w", configured.ID, err)
		}
		old := make([]model.Node, 0)
		for _, node := range intent.Nodes {
			if !uciIdentifier.MatchString(node.ID) {
				return nil, fmt.Errorf("node %q has an unsafe UCI section ID", node.ID)
			}
			if node.SourceSubscription == configured.ID {
				old = append(old, node)
			}
		}
		merged := mergeSubscriptionNodes(configured.ID, old, fresh)
		for _, node := range merged {
			if !uciIdentifier.MatchString(node.ID) {
				return nil, fmt.Errorf("subscription node %q has an unsafe UCI section ID", node.ID)
			}
		}
		existingNodes := intent.Nodes
		intent.Nodes = replaceSubscriptionNodes(existingNodes, configured.ID, merged)
		appendSubscriptionBatch(&batch, configured.ID, existingNodes, merged)
		snapshot := SubscriptionSnapshot{SubscriptionID: configured.ID, URL: configured.URL, FetchedAt: time.Now().UTC(), Nodes: merged}
		result = append(result, snapshot)
	}
	if id != "" && len(result) == 0 {
		return nil, fmt.Errorf("enabled subscription %q was not found", id)
	}
	if batch.Len() > 0 {
		validation := model.Validate(intent)
		if !validation.OK {
			issue := validation.Errors[0]
			return nil, fmt.Errorf("subscription update produced invalid candidate: %s %q option %q: %s", issue.ObjectType, issue.ObjectID, issue.Option, issue.Message)
		}
		if err := writeUCI(ctx, batch.String()+"commit steer\n"); err != nil {
			return nil, err
		}
		for _, snapshot := range result {
			if err := saveSubscriptionSnapshot(stateDirectory, snapshot); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

func replaceSubscriptionNodes(existing []model.Node, subscriptionID string, replacement []model.Node) []model.Node {
	result := make([]model.Node, 0, len(existing)+len(replacement))
	for _, node := range existing {
		if node.SourceSubscription != subscriptionID {
			result = append(result, node)
		}
	}
	return append(result, replacement...)
}

func CleanSubscriptionNode(configPath, stateDirectory, id, nodeID string) (SubscriptionSnapshot, error) {
	return CleanSubscriptionNodeWithWriter(configPath, stateDirectory, id, nodeID, SystemUCIWriter(configPath))
}

func CleanSubscriptionNodeWithWriter(configPath, stateDirectory, id, nodeID string, writeUCI UCIWriter) (SubscriptionSnapshot, error) {
	if writeUCI == nil {
		return SubscriptionSnapshot{}, fmt.Errorf("subscription cleanup requires a UCI writer")
	}
	if !uciIdentifier.MatchString(id) || !uciIdentifier.MatchString(nodeID) {
		return SubscriptionSnapshot{}, fmt.Errorf("subscription and node IDs must be safe UCI identifiers")
	}
	path := SubscriptionSnapshotPath(stateDirectory, id)
	snapshot, err := readSubscriptionSnapshot(path)
	if err != nil {
		return SubscriptionSnapshot{}, fmt.Errorf("read subscription snapshot: %w", err)
	}
	filtered := snapshot.Nodes[:0]
	removed := false
	for _, node := range snapshot.Nodes {
		if node.ID == nodeID {
			removed = true
			continue
		}
		filtered = append(filtered, node)
	}
	if !removed {
		return SubscriptionSnapshot{}, fmt.Errorf("subscription node %q was not found", nodeID)
	}
	config, err := os.ReadFile(configPath)
	if err != nil {
		return SubscriptionSnapshot{}, fmt.Errorf("read UCI for subscription cleanup: %w", err)
	}
	intent, err := DecodeBytes(config)
	if err != nil {
		return SubscriptionSnapshot{}, err
	}
	var batch strings.Builder
	for _, node := range intent.Nodes {
		if node.ID == nodeID && node.SourceSubscription == id {
			fmt.Fprintf(&batch, "delete steer.%s\n", node.ID)
		}
	}
	if batch.Len() == 0 {
		return SubscriptionSnapshot{}, fmt.Errorf("subscription node %q is not present in UCI", nodeID)
	}
	if err := writeUCI(context.Background(), batch.String()+"commit steer\n"); err != nil {
		return SubscriptionSnapshot{}, err
	}
	snapshot.Nodes = filtered
	if err := saveSubscriptionSnapshot(stateDirectory, snapshot); err != nil {
		return SubscriptionSnapshot{}, err
	}
	return snapshot, nil
}

func saveSubscriptionSnapshot(stateDirectory string, snapshot SubscriptionSnapshot) error {
	path := SubscriptionSnapshotPath(stateDirectory, snapshot.SubscriptionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create subscription state: %w", err)
	}
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encode subscription snapshot: %w", err)
	}
	if err := atomicWrite(path, append(encoded, '\n')); err != nil {
		return fmt.Errorf("save subscription snapshot: %w", err)
	}
	return nil
}

func appendSubscriptionBatch(batch *strings.Builder, subscriptionID string, existing, merged []model.Node) {
	for _, node := range existing {
		if node.SourceSubscription == subscriptionID {
			fmt.Fprintf(batch, "delete steer.%s\n", node.ID)
		}
	}
	for _, node := range merged {
		appendNodeBatch(batch, node)
	}
}

func appendNodeBatch(batch *strings.Builder, node model.Node) {
	fmt.Fprintf(batch, "set steer.%s=node\n", node.ID)
	setString := func(name, value string) {
		if value != "" {
			fmt.Fprintf(batch, "set steer.%s.%s=%s\n", node.ID, name, uciQuote(value))
		}
	}
	setBool := func(name string, value bool) {
		if value {
			fmt.Fprintf(batch, "set steer.%s.%s=1\n", node.ID, name)
		}
	}
	setInt := func(name string, value int) {
		if value != 0 {
			fmt.Fprintf(batch, "set steer.%s.%s=%d\n", node.ID, name, value)
		}
	}
	setString("name", node.Name)
	if !node.Enabled {
		fmt.Fprintf(batch, "set steer.%s.enabled=0\n", node.ID)
	}
	setString("type", node.Type)
	setString("server", node.Server)
	setInt("server_port", node.ServerPort)
	setString("uuid", node.UUID)
	setString("username", node.Username)
	setString("password", node.Password)
	setString("private_key", node.PrivateKey)
	setString("host_key", node.HostKey)
	setString("flow", node.Flow)
	setString("packet_encoding", node.PacketEncoding)
	setString("method", node.Method)
	setString("plugin", node.Plugin)
	setString("plugin_options", node.PluginOptions)
	setString("security", node.Security)
	setInt("alter_id", node.AlterID)
	setInt("version", node.Version)
	setString("network", node.Network)
	setString("transport", node.Transport)
	setString("transport_path", node.TransportPath)
	setString("transport_host", node.TransportHost)
	setString("service_name", node.ServiceName)
	setString("congestion_control", node.CongestionControl)
	setString("udp_relay_mode", node.UDPRelayMode)
	setBool("udp_over_stream", node.UDPOverStream)
	setBool("zero_rtt_handshake", node.ZeroRTTHandshake)
	setString("heartbeat", node.Heartbeat)
	setBool("quic", node.QUIC)
	setString("quic_congestion_control", node.QUICCongestionControl)
	setInt("insecure_concurrency", node.InsecureConcurrency)
	setString("executable_path", node.ExecutablePath)
	setString("data_directory", node.DataDirectory)
	setString("hop_interval", node.HopInterval)
	setString("obfs_type", node.ObfsType)
	setString("obfs_password", node.ObfsPassword)
	setInt("up_mbps", node.UpMbps)
	setInt("down_mbps", node.DownMbps)
	setString("tls_server_name", node.TLSServerName)
	setBool("insecure", node.Insecure)
	setString("reality_public_key", node.RealityPublicKey)
	setString("reality_short_id", node.RealityShortID)
	setString("utls_fingerprint", node.UTLSFingerprint)
	setString("source_subscription", node.SourceSubscription)
	setString("source_fingerprint", node.SourceFingerprint)
	setBool("pinned_stale", node.PinnedStale)
	for name, values := range map[string][]string{"server_ports": node.ServerPorts, "host_key_algorithms": node.HostKeyAlgorithms, "extra_args": node.ExtraArgs} {
		for _, value := range values {
			fmt.Fprintf(batch, "add_list steer.%s.%s=%s\n", node.ID, name, uciQuote(value))
		}
	}
}

func uciQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

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

func mergeSubscriptionNodes(subscriptionID string, old, fresh []model.Node) []model.Node {
	oldByFingerprint := make(map[string]model.Node, len(old))
	for _, node := range old {
		oldByFingerprint[subscription.Fingerprint(node)] = node
	}
	seen := make(map[string]bool, len(fresh))
	merged := make([]model.Node, 0, len(old)+len(fresh))
	for _, node := range fresh {
		fingerprint := subscription.Fingerprint(node)
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
			node.ID = stableSubscriptionNodeID(subscriptionID, fingerprint)
		}
		node.Enabled, node.SourceSubscription, node.SourceFingerprint, node.PinnedStale = enabled, subscriptionID, fingerprint, false
		merged = append(merged, node)
		seen[fingerprint] = true
	}
	for _, node := range old {
		fingerprint := subscription.Fingerprint(node)
		if seen[fingerprint] {
			continue
		}
		node.SourceSubscription, node.SourceFingerprint, node.PinnedStale = subscriptionID, fingerprint, true
		merged = append(merged, node)
	}
	sort.SliceStable(merged, func(left, right int) bool { return merged[left].ID < merged[right].ID })
	return merged
}

func stableSubscriptionNodeID(subscriptionID, fingerprint string) string {
	prefix := subscriptionID
	if len(prefix) > 19 {
		prefix = prefix[:19]
	}
	return prefix + "_" + fingerprint[:12]
}
