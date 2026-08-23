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

	model "github.com/gsh20040816/steer/go/internal/intent"
)

type ParseResult struct {
	Nodes   []model.Node
	Skipped int
}

func ParseList(raw string) (ParseResult, error) {
	result := ParseResult{Nodes: []model.Node{}}
	text := strings.TrimSpace(raw)
	if text == "" {
		return result, nil
	}
	if !strings.Contains(text, "://") {
		decoded, err := decodeBase64(text)
		if err == nil {
			text = strings.TrimSpace(decoded)
			if strings.HasPrefix(text, "{") {
				node, parseErr := parseVMessPayload(text)
				appendParsedNode(&result, node, parseErr)
				return result, nil
			}
		}
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		node, err := ParseURI(line)
		appendParsedNode(&result, node, err)
	}
	return result, nil
}

func appendParsedNode(result *ParseResult, node model.Node, err error) {
	if err == nil {
		validation := model.ValidateNode(node)
		if !validation.OK {
			err = fmt.Errorf("invalid node")
		}
	}
	if err != nil {
		result.Skipped++
		return
	}
	result.Nodes = append(result.Nodes, node)
}

func ParseURI(raw string) (model.Node, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return model.Node{}, fmt.Errorf("invalid node URI")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "vmess" {
		return parseVMess(raw)
	}
	if scheme == "ss" {
		return parseShadowsocks(raw, u)
	}
	if scheme == "socks5" {
		return parseCredentialProxy(u, "socks")
	}
	if scheme == "socks" || scheme == "http" || scheme == "https" {
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
		if encryption := query.Get("encryption"); encryption != "" && encryption != "none" {
			return model.Node{}, fmt.Errorf("unsupported VLESS encryption %q", encryption)
		}
		node.Flow, node.PacketEncoding = query.Get("flow"), first(query.Get("packetEncoding"), query.Get("packet_encoding"))
		if node.Flow != "" && node.Flow != "xtls-rprx-vision" {
			return model.Node{}, fmt.Errorf("unsupported VLESS flow %q", node.Flow)
		}
		if node.PacketEncoding != "" && node.PacketEncoding != "xudp" && node.PacketEncoding != "packetaddr" {
			return model.Node{}, fmt.Errorf("unsupported VLESS packet encoding %q", node.PacketEncoding)
		}
		security := first(query.Get("security"), "none")
		if security != "none" && security != "tls" && security != "reality" {
			return model.Node{}, fmt.Errorf("unsupported VLESS security %q", security)
		}
		sni := first(query.Get("sni"), query.Get("serverName"))
		publicKey := first(query.Get("pbk"), query.Get("publicKey"))
		shortID := first(query.Get("sid"), query.Get("shortId"))
		fingerprint := first(query.Get("fp"), query.Get("fingerprint"))
		insecure := boolValue(first(query.Get("allowInsecure"), query.Get("insecure")))
		if security != "reality" && (publicKey != "" || shortID != "") {
			return model.Node{}, fmt.Errorf("VLESS Reality parameters require security=reality")
		}
		if security == "none" && (sni != "" || fingerprint != "" || insecure || node.Flow != "") {
			return model.Node{}, fmt.Errorf("VLESS TLS parameters require security=tls or security=reality")
		}
		switch security {
		case "tls":
			if sni == "" {
				return model.Node{}, fmt.Errorf("VLESS TLS requires sni")
			}
			node.TLSServerName, node.UTLSFingerprint = sni, fingerprint
		case "reality":
			if sni == "" || publicKey == "" || shortID == "" || fingerprint == "" {
				return model.Node{}, fmt.Errorf("VLESS Reality requires sni, public key, short ID and fingerprint")
			}
			node.TLSServerName, node.RealityPublicKey, node.RealityShortID, node.UTLSFingerprint = sni, publicKey, shortID, fingerprint
		}
		node.Insecure = insecure
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
		node.ServerPorts = splitPorts(query.Get("mport"))
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

func parseShadowsocks(raw string, u *url.URL) (model.Node, error) {
	if err := rejectUnknown(u.Query(), map[string]bool{"plugin": true, "plugin-opts": true}); err != nil {
		return model.Node{}, err
	}
	if err := validateQueryValues(u.Query()); err != nil {
		return model.Node{}, err
	}
	plugin, pluginOptions, err := shadowsocksPlugin(u.Query())
	if err != nil {
		return model.Node{}, err
	}
	var credential, server string
	serverPort := 443
	if u.User != nil && u.Hostname() != "" {
		server, serverPort = u.Hostname(), port(u, 443)
		if password, plaintext := u.User.Password(); plaintext {
			credential = u.User.Username() + ":" + password
		} else {
			credential, err = decodeBase64(u.User.Username())
			if err != nil {
				return model.Node{}, fmt.Errorf("invalid Shadowsocks credential: %w", err)
			}
		}
	} else {
		payload, decodeErr := decodeBase64(opaquePayload(raw))
		if decodeErr != nil {
			return model.Node{}, fmt.Errorf("invalid legacy Shadowsocks payload: %w", decodeErr)
		}
		at := strings.LastIndex(payload, "@")
		if at < 0 {
			return model.Node{}, fmt.Errorf("ss URI must contain method/password and server")
		}
		credential = payload[:at]
		serverURL, parseErr := url.Parse("ss://" + payload[at+1:])
		if parseErr != nil || serverURL.Hostname() == "" {
			return model.Node{}, fmt.Errorf("invalid Shadowsocks server")
		}
		server, serverPort = serverURL.Hostname(), port(serverURL, 443)
	}
	methodPassword := strings.SplitN(credential, ":", 2)
	if len(methodPassword) != 2 {
		return model.Node{}, fmt.Errorf("ss URI credential must be method:password")
	}
	if methodPassword[0] == "" || methodPassword[1] == "" {
		return model.Node{}, fmt.Errorf("ss URI method and password must be non-empty")
	}
	return model.Node{Enabled: true, Type: "shadowsocks", Name: u.Fragment, Server: server, ServerPort: serverPort, NodeCredentials: model.NodeCredentials{Password: methodPassword[1]}, NodeProtocol: model.NodeProtocol{Method: methodPassword[0], Plugin: plugin, PluginOptions: pluginOptions}}, nil
}

func shadowsocksPlugin(query url.Values) (string, string, error) {
	plugin := query.Get("plugin")
	options := ""
	if separator := strings.IndexByte(plugin, ';'); separator >= 0 {
		plugin, options = plugin[:separator], plugin[separator+1:]
	}
	legacyOptions := query.Get("plugin-opts")
	if options != "" && legacyOptions != "" && options != legacyOptions {
		return "", "", fmt.Errorf("conflicting Shadowsocks plugin options")
	}
	if options == "" {
		options = legacyOptions
	}
	if plugin == "" && options != "" {
		return "", "", fmt.Errorf("Shadowsocks plugin options require a plugin")
	}
	return plugin, options, nil
}

func opaquePayload(raw string) string {
	separator := strings.IndexByte(raw, ':')
	if separator < 0 {
		return ""
	}
	payload := strings.TrimPrefix(raw[separator+1:], "//")
	if end := strings.IndexAny(payload, "?#"); end >= 0 {
		payload = payload[:end]
	}
	return payload
}

func parseVMess(raw string) (model.Node, error) {
	decoded, err := decodeBase64(opaquePayload(raw))
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
		NodeTransport:   model.NodeTransport{Transport: value.Network, TransportHost: value.Host, TransportPath: value.Path},
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
	for index, item := range items {
		if strings.Count(item, "-") == 1 {
			items[index] = strings.Replace(item, "-", ":", 1)
		}
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
