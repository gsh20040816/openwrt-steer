// SPDX-License-Identifier: GPL-3.0-or-later

// Package subscription parses the deliberately small, interoperable node
// subscription format accepted by Steer: one standard proxy URI per line or
// one Base64-wrapped block containing those lines.  It never preserves an
// opaque JSON blob; every field is lowered into model.Node and validated.
package subscription

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/model"
)

func ParseList(raw string) ([]model.Node, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, fmt.Errorf("subscription is empty")
	}
	if !strings.Contains(text, "://") {
		decoded, err := decodeBase64(text)
		if err != nil {
			return nil, fmt.Errorf("subscription is neither URI text nor valid Base64: %w", err)
		}
		text = strings.TrimSpace(decoded)
		if strings.HasPrefix(text, "{") {
			node, err := parseVMessPayload(text)
			if err != nil {
				return nil, err
			}
			return []model.Node{node}, nil
		}
	}
	var nodes []model.Node
	for lineNumber, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		node, err := ParseURI(line)
		if err != nil {
			return nil, fmt.Errorf("subscription line %d: %w", lineNumber+1, err)
		}
		nodes = append(nodes, node)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("subscription contains no node URI")
	}
	return nodes, nil
}

func ParseURI(raw string) (model.Node, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" {
		return model.Node{}, fmt.Errorf("invalid node URI")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "vmess" {
		return parseVMess(u)
	}
	if scheme == "ss" {
		return parseShadowsocks(u)
	}
	if scheme == "socks" || scheme == "socks5" || scheme == "http" || scheme == "https" {
		return parseCredentialProxy(u, scheme)
	}
	if scheme == "vless" || scheme == "trojan" || scheme == "hysteria" || scheme == "hysteria2" || scheme == "hy2" || scheme == "shadowtls" || scheme == "tuic" || scheme == "anytls" || scheme == "naive+https" || scheme == "ssh" {
		return parseCredentialNode(u, scheme)
	}
	return model.Node{}, fmt.Errorf("unsupported node URI scheme %q", u.Scheme)
}

func parseCredentialProxy(u *url.URL, scheme string) (model.Node, error) {
	if u.Path != "" && u.Path != "/" {
		return model.Node{}, fmt.Errorf("%s URI path is unsupported", scheme)
	}
	if u.Hostname() == "" {
		return model.Node{}, fmt.Errorf("%s URI has no server", scheme)
	}
	node := model.Node{Enabled: true, Type: scheme, Name: u.Fragment, Server: u.Hostname(), ServerPort: port(u, 1080)}
	if scheme == "https" {
		node.Type = "http"
		node.TLSServerName = u.Hostname()
	}
	if u.User != nil {
		node.Username = u.User.Username()
		node.Password, _ = u.User.Password()
	}
	if node.Name == "" {
		node.Name = node.Type + " " + u.Host
	}
	if err := rejectUnknown(u.Query(), map[string]bool{"sni": true, "insecure": true}); err != nil {
		return model.Node{}, err
	}
	if err := validateQueryValues(u.Query()); err != nil {
		return model.Node{}, err
	}
	if value := u.Query().Get("sni"); value != "" {
		node.TLSServerName = value
	}
	if value := u.Query().Get("insecure"); value != "" {
		node.Insecure, _ = strconv.ParseBool(value)
	}
	return node, nil
}

func parseCredentialNode(u *url.URL, scheme string) (model.Node, error) {
	if u.Path != "" && u.Path != "/" {
		return model.Node{}, fmt.Errorf("%s URI path is unsupported", scheme)
	}
	if u.Hostname() == "" {
		return model.Node{}, fmt.Errorf("%s URI has no server", scheme)
	}
	node := model.Node{Enabled: true, Server: u.Hostname(), ServerPort: port(u, 443), Name: u.Fragment}
	if node.Name == "" {
		node.Name = scheme + " " + u.Host
	}
	username := ""
	password := ""
	if u.User != nil {
		username = u.User.Username()
		password, _ = u.User.Password()
	}
	query := u.Query()
	allowed := map[string][]string{
		"vless":       {"encryption", "flow", "security", "sni", "serverName", "fp", "fingerprint", "pbk", "publicKey", "sid", "shortId", "type", "packetEncoding", "packet_encoding", "allowInsecure", "insecure", "path", "host", "serviceName"},
		"trojan":      {"sni", "serverName", "peer", "fp", "fingerprint", "type", "allowInsecure", "insecure", "path", "host", "serviceName"},
		"hysteria":    {"sni", "peer", "insecure", "allowInsecure", "obfs", "obfs-password", "hop-interval", "hopInterval", "mport", "upmbps", "upMbps", "downmbps", "downMbps"},
		"hysteria2":   {"sni", "insecure", "allowInsecure", "obfs", "obfs-password", "hop-interval", "hopInterval", "mport", "upmbps", "upMbps", "downmbps", "downMbps"},
		"hy2":         {"sni", "insecure", "allowInsecure", "obfs", "obfs-password", "hop-interval", "hopInterval", "mport", "upmbps", "upMbps", "downmbps", "downMbps"},
		"shadowtls":   {"version", "sni", "insecure", "allowInsecure", "fp", "fingerprint"},
		"tuic":        {"congestion_control", "udp_relay_mode", "udp_over_stream", "zero_rtt_handshake", "heartbeat", "sni", "insecure", "fp", "fingerprint"},
		"anytls":      {"sni", "insecure", "fp", "fingerprint", "type"},
		"naive+https": {"sni", "insecure", "quic", "quic_congestion_control", "fp", "fingerprint"},
		"ssh":         {},
	}
	if err := rejectUnknown(query, setAllowed(allowed[scheme])); err != nil {
		return model.Node{}, err
	}
	if err := validateQueryValues(query); err != nil {
		return model.Node{}, err
	}
	if scheme == "anytls" && query.Get("type") != "" && !strings.EqualFold(query.Get("type"), "tcp") {
		return model.Node{}, fmt.Errorf("anytls URI parameter %q must be tcp", "type")
	}
	node.UTLSFingerprint = first(query.Get("fp"), query.Get("fingerprint"))
	switch scheme {
	case "vless":
		node.Type, node.UUID = "vless", username
		if node.UUID == "" {
			return model.Node{}, fmt.Errorf("vless URI requires UUID")
		}
		node.Flow, node.PacketEncoding = query.Get("flow"), first(query.Get("packetEncoding"), query.Get("packet_encoding"))
		security := first(query.Get("security"), query.Get("encryption"), "none")
		node.Security = security
		node.TLSServerName = first(query.Get("sni"), query.Get("serverName"))
		// uTLS fingerprint is carried by the shared TLS group.
		if security == "reality" {
			node.RealityPublicKey, node.RealityShortID = first(query.Get("pbk"), query.Get("publicKey")), first(query.Get("sid"), query.Get("shortId"))
		}
		node.Insecure = boolValue(first(query.Get("allowInsecure"), query.Get("insecure")))
		setTransport(&node, query)
	case "trojan":
		node.Type, node.Password = "trojan", username
		node.TLSServerName = first(query.Get("sni"), query.Get("serverName"), query.Get("peer"), node.Server)
		// uTLS fingerprint is carried by the shared TLS group.
		node.Insecure = boolValue(first(query.Get("allowInsecure"), query.Get("insecure")))
		setTransport(&node, query)
	case "hysteria", "hysteria2", "hy2":
		node.Type, node.Password = "hysteria2", username
		if scheme == "hysteria" {
			node.Type = "hysteria"
		}
		node.TLSServerName = first(query.Get("sni"), query.Get("peer"), node.Server)
		node.Insecure = boolValue(first(query.Get("insecure"), query.Get("allowInsecure")))
		node.HopInterval = first(query.Get("hop-interval"), query.Get("hopInterval"))
		node.ObfsType, node.ObfsPassword = query.Get("obfs"), query.Get("obfs-password")
		node.ServerPorts = splitPorts(first(query.Get("mport"), u.Port()))
		node.UpMbps = intValue(first(query.Get("upmbps"), query.Get("upMbps")))
		node.DownMbps = intValue(first(query.Get("downmbps"), query.Get("downMbps")))
	case "shadowtls":
		node.Type, node.Password = "shadowtls", username
		node.Version = intValue(first(query.Get("version"), "1"))
		node.TLSServerName = first(query.Get("sni"), node.Server)
		node.Insecure = boolValue(first(query.Get("insecure"), query.Get("allowInsecure")))
	case "tuic":
		node.Type, node.UUID, node.Password = "tuic", username, password
		node.CongestionControl, node.UDPRelayMode = query.Get("congestion_control"), query.Get("udp_relay_mode")
		node.UDPOverStream, node.ZeroRTTHandshake = boolValue(query.Get("udp_over_stream")), boolValue(query.Get("zero_rtt_handshake"))
		node.Heartbeat, node.TLSServerName = query.Get("heartbeat"), first(query.Get("sni"), node.Server)
		node.Insecure = boolValue(query.Get("insecure"))
	case "anytls":
		node.Type, node.Password = "anytls", username
		node.TLSServerName, node.Insecure = first(query.Get("sni"), node.Server), boolValue(query.Get("insecure"))
	case "naive+https":
		node.Type, node.Username, node.Password = "naive", username, password
		node.TLSServerName, node.Insecure = first(query.Get("sni"), node.Server), boolValue(query.Get("insecure"))
		node.QUIC, node.QUICCongestionControl = boolValue(query.Get("quic")), query.Get("quic_congestion_control")
	case "ssh":
		node.Type, node.Username, node.Password = "ssh", username, password
	}
	return node, nil
}

func parseShadowsocks(u *url.URL) (model.Node, error) {
	if err := rejectUnknown(u.Query(), map[string]bool{"plugin": true, "plugin-opts": true}); err != nil {
		return model.Node{}, err
	}
	if u.Hostname() == "" && u.Opaque == "" {
		return model.Node{}, fmt.Errorf("ss URI has no server")
	}
	payload := strings.TrimPrefix(u.Host, "//")
	if u.User != nil {
		encoded := u.User.Username()
		decoded, err := decodeBase64(encoded)
		if err != nil {
			return model.Node{}, fmt.Errorf("invalid Shadowsocks credential: %w", err)
		}
		payload = decoded + "@" + u.Host
	}
	if u.User == nil {
		if at := strings.LastIndex(payload, "@"); at >= 0 {
			decoded, err := decodeBase64(payload[:at])
			if err != nil {
				return model.Node{}, fmt.Errorf("invalid Shadowsocks credential: %w", err)
			}
			payload = decoded + "@" + payload[at+1:]
		} else {
			decoded, err := decodeBase64(payload)
			if err == nil {
				payload = decoded
			}
		}
	}
	parts := strings.SplitN(payload, "@", 2)
	if len(parts) != 2 {
		return model.Node{}, fmt.Errorf("ss URI must contain method/password and server")
	}
	methodPassword := strings.SplitN(parts[0], ":", 2)
	if len(methodPassword) != 2 {
		return model.Node{}, fmt.Errorf("ss URI credential must be method:password")
	}
	serverURL, err := url.Parse("ss://" + parts[1])
	if err != nil || serverURL.Hostname() == "" {
		return model.Node{}, fmt.Errorf("invalid Shadowsocks server")
	}
	return model.Node{Enabled: true, Type: "shadowsocks", Name: u.Fragment, Server: serverURL.Hostname(), ServerPort: port(serverURL, 443), NodeCredentials: model.NodeCredentials{Password: methodPassword[1]}, NodeProtocol: model.NodeProtocol{Method: methodPassword[0], Plugin: u.Query().Get("plugin"), PluginOptions: u.Query().Get("plugin-opts")}}, nil
}

func parseVMess(u *url.URL) (model.Node, error) {
	decoded, err := decodeBase64(strings.TrimPrefix(u.Host, "//"))
	if err != nil {
		return model.Node{}, fmt.Errorf("invalid VMess Base64 payload: %w", err)
	}
	return parseVMessPayload(decoded)
}

func parseVMessPayload(decoded string) (model.Node, error) {
	var value struct {
		Name     string          `json:"ps"`
		Add      string          `json:"add"`
		Port     json.Number     `json:"port"`
		UUID     string          `json:"id"`
		AlterID  json.RawMessage `json:"aid"`
		Security string          `json:"scy"`
		Network  string          `json:"net"`
		TLS      string          `json:"tls"`
		SNI      string          `json:"sni"`
		Host     string          `json:"host"`
		Path     string          `json:"path"`
		Type     string          `json:"type"`
	}
	if err := json.Unmarshal([]byte(decoded), &value); err != nil {
		return model.Node{}, fmt.Errorf("invalid VMess JSON payload: %w", err)
	}
	serverPort, err := strconv.Atoi(value.Port.String())
	if err != nil {
		return model.Node{}, fmt.Errorf("invalid VMess port")
	}
	node := model.Node{Enabled: true, Type: "vmess", Name: value.Name, Server: value.Add, ServerPort: serverPort,
		NodeCredentials: model.NodeCredentials{UUID: value.UUID},
		NodeTransport:   model.NodeTransport{Network: value.Network, Transport: value.Network, TransportHost: value.Host, TransportPath: value.Path},
		NodeProtocol:    model.NodeProtocol{AlterID: rawInt(value.AlterID), Security: value.Security},
		NodeTLS:         model.NodeTLS{TLSServerName: value.SNI}}
	if value.TLS != "" && value.TLS != "none" {
		node.TLSServerName = first(node.TLSServerName, node.Server)
	}
	if node.Name == "" {
		node.Name = "VMess " + node.Server
	}
	return node, nil
}

func rawInt(value json.RawMessage) int {
	var number int
	if json.Unmarshal(value, &number) == nil {
		return number
	}
	var text string
	if json.Unmarshal(value, &text) == nil {
		return intValue(text)
	}
	return 0
}

func setTransport(node *model.Node, query url.Values) {
	node.Transport, node.TransportPath, node.TransportHost, node.ServiceName = query.Get("type"), query.Get("path"), query.Get("host"), query.Get("serviceName")
	if node.Transport == "" {
		node.Transport = "tcp"
	}
}

func rejectUnknown(values url.Values, allowed map[string]bool) error {
	for key := range values {
		if !allowed[key] {
			return fmt.Errorf("unsupported URI parameter %q", key)
		}
	}
	return nil
}

func setAllowed(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func validateQueryValues(values url.Values) error {
	for key, entries := range values {
		if len(entries) > 1 {
			for _, entry := range entries[1:] {
				if entry != entries[0] {
					return fmt.Errorf("conflicting URI parameter %q", key)
				}
			}
		}
	}
	for _, aliases := range [][]string{{"sni", "serverName", "peer"}, {"fp", "fingerprint"}, {"pbk", "publicKey"}, {"sid", "shortId"}, {"packetEncoding", "packet_encoding"}, {"allowInsecure", "insecure"}, {"hop-interval", "hopInterval"}, {"upmbps", "upMbps"}, {"downmbps", "downMbps"}} {
		var previous string
		for _, alias := range aliases {
			if value := values.Get(alias); value != "" {
				if previous != "" && previous != value {
					return fmt.Errorf("conflicting URI parameter aliases %q", strings.Join(aliases, "/"))
				}
				previous = value
			}
		}
	}
	for _, key := range []string{"allowInsecure", "insecure", "udp_over_stream", "zero_rtt_handshake", "quic"} {
		if value := values.Get(key); value != "" {
			if _, err := strconv.ParseBool(value); err != nil && value != "0" && value != "1" {
				return fmt.Errorf("URI parameter %q must be boolean", key)
			}
		}
	}
	for _, key := range []string{"version", "upmbps", "upMbps", "downmbps", "downMbps"} {
		if value := values.Get(key); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 0 {
				return fmt.Errorf("URI parameter %q must be a non-negative integer", key)
			}
		}
	}
	return nil
}

func decodeBase64(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "-", "+"), "_", "/"))
	for len(value)%4 != 0 {
		value += "="
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			return string(decoded), nil
		}
	}
	return "", fmt.Errorf("invalid Base64")
}

func Fingerprint(node model.Node) string {
	copy := node
	copy.ID, copy.Name, copy.Enabled = "", "", true
	copy.SourceSubscription, copy.SourceFingerprint, copy.PinnedStale = "", "", false
	encoded, _ := json.Marshal(copy)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func port(u *url.URL, fallback int) int {
	if value := u.Port(); value != "" {
		return intValue(value)
	}
	return fallback
}

func splitPorts(value string) []string {
	if value == "" {
		return nil
	}
	items := strings.Split(value, ",")
	if len(items) == 1 {
		return nil
	}
	return items
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func boolValue(value string) bool {
	parsed, _ := strconv.ParseBool(value)
	return parsed
}

func intValue(value string) int {
	parsed, _ := strconv.Atoi(value)
	return parsed
}
