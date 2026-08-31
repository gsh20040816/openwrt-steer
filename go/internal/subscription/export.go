// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	model "github.com/gsh20040816/steer/go/internal/intent"
)

// EncodeURI serializes one Canonical node into the same interoperable share
// formats accepted by ParseURI. It refuses nodes whose effective options
// cannot be represented instead of returning a lossy link.
func EncodeURI(node model.Node) (string, error) {
	validation := model.ValidateNode(node)
	if !validation.OK {
		issue := validation.Errors[0]
		return "", fmt.Errorf("node is invalid: %s", issue.Message)
	}

	switch node.Type {
	case "socks":
		if hasTLS(node) {
			return "", fmt.Errorf("SOCKS share links cannot represent TLS options")
		}
		return encodeCredentialProxy(node, "socks")
	case "http":
		scheme := "http"
		if hasTLS(node) {
			if node.TLSServerName == "" {
				return "", fmt.Errorf("HTTP TLS share links require a TLS server name")
			}
			scheme = "https"
		}
		return encodeCredentialProxy(node, scheme)
	case "shadowsocks":
		if hasTLS(node) {
			return "", fmt.Errorf("Shadowsocks share links cannot represent TLS options")
		}
		if node.Plugin == "" && node.PluginOptions != "" {
			return "", fmt.Errorf("Shadowsocks plugin options require a plugin")
		}
		query := url.Values{}
		if node.Plugin != "" {
			plugin := node.Plugin
			if node.PluginOptions != "" {
				plugin += ";" + node.PluginOptions
			}
			query.Set("plugin", plugin)
		}
		credential := base64.RawURLEncoding.EncodeToString([]byte(node.Method + ":" + node.Password))
		return encodeURL(node, "ss", url.User(credential), query), nil
	case "vmess":
		return encodeVMess(node)
	case "vless":
		query := url.Values{}
		query.Set("encryption", "none")
		security := "none"
		if node.RealityPublicKey != "" || node.RealityShortID != "" {
			security = "reality"
			query.Set("pbk", node.RealityPublicKey)
			query.Set("sid", node.RealityShortID)
		} else if hasTLS(node) {
			security = "tls"
		}
		if security != "none" {
			if node.TLSServerName == "" {
				return "", fmt.Errorf("VLESS TLS share links require a TLS server name")
			}
			setTLSQuery(query, node)
		} else if node.Flow != "" {
			return "", fmt.Errorf("VLESS flow cannot be represented without TLS or Reality")
		}
		query.Set("security", security)
		setIf(query, "flow", node.Flow)
		setIf(query, "packetEncoding", node.PacketEncoding)
		setTransportQuery(query, node)
		return encodeURL(node, "vless", url.User(node.UUID), query), nil
	case "trojan":
		query := url.Values{}
		setTLSQuery(query, node)
		setTransportQuery(query, node)
		return encodeURL(node, "trojan", url.User(node.Password), query), nil
	case "hysteria", "hysteria2":
		query := url.Values{}
		setTLSQuery(query, node)
		setIf(query, "hop-interval", node.HopInterval)
		if len(node.ServerPorts) > 0 {
			query.Set("mport", strings.Join(node.ServerPorts, ","))
		}
		setPositiveInt(query, "upmbps", node.UpMbps)
		setPositiveInt(query, "downmbps", node.DownMbps)
		if node.Type == "hysteria" {
			setIf(query, "obfs", node.ObfsPassword)
		} else {
			setIf(query, "obfs", node.ObfsType)
			setIf(query, "obfs-password", node.ObfsPassword)
		}
		return encodeURL(node, node.Type, url.User(node.Password), query), nil
	case "shadowtls":
		query := url.Values{}
		setTLSQuery(query, node)
		query.Set("version", strconv.Itoa(node.Version))
		return encodeURL(node, "shadowtls", url.User(node.Password), query), nil
	case "tuic":
		query := url.Values{}
		setTLSQuery(query, node)
		setIf(query, "congestion_control", node.CongestionControl)
		setIf(query, "udp_relay_mode", node.UDPRelayMode)
		setTrue(query, "udp_over_stream", node.UDPOverStream)
		setTrue(query, "zero_rtt_handshake", node.ZeroRTTHandshake)
		setIf(query, "heartbeat", node.Heartbeat)
		var user *url.Userinfo
		if node.Password == "" {
			user = url.User(node.UUID)
		} else {
			user = url.UserPassword(node.UUID, node.Password)
		}
		return encodeURL(node, "tuic", user, query), nil
	case "anytls":
		query := url.Values{}
		setTLSQuery(query, node)
		return encodeURL(node, "anytls", url.User(node.Password), query), nil
	case "naive":
		if node.InsecureConcurrency < 0 {
			return "", fmt.Errorf("NaiveProxy insecure concurrency cannot be negative")
		}
		query := url.Values{}
		setTLSQuery(query, node)
		setTrue(query, "quic", node.QUIC)
		setIf(query, "quic_congestion_control", node.QUICCongestionControl)
		setPositiveInt(query, "insecure_concurrency", node.InsecureConcurrency)
		return encodeURL(node, "naive+https", url.UserPassword(node.Username, node.Password), query), nil
	case "ssh":
		if hasTLS(node) {
			return "", fmt.Errorf("SSH share links cannot represent TLS options")
		}
		if node.PrivateKey != "" || node.HostKey != "" || len(node.HostKeyAlgorithms) > 0 {
			return "", fmt.Errorf("SSH private keys and host-key constraints are not representable in share links")
		}
		if node.Password == "" {
			return "", fmt.Errorf("SSH share links require password authentication")
		}
		return encodeURL(node, "ssh", url.UserPassword(node.Username, node.Password), nil), nil
	case "tor":
		return "", fmt.Errorf("Tor nodes do not have a share-link format")
	default:
		return "", fmt.Errorf("node type %q does not have a share-link format", node.Type)
	}
}

func encodeCredentialProxy(node model.Node, scheme string) (string, error) {
	if node.RealityPublicKey != "" || node.RealityShortID != "" {
		return "", fmt.Errorf("%s share links cannot represent Reality options", strings.ToUpper(node.Type))
	}
	query := url.Values{}
	if scheme == "https" {
		setTLSQuery(query, node)
	}
	var user *url.Userinfo
	if node.Username != "" || node.Password != "" {
		if node.Password == "" {
			user = url.User(node.Username)
		} else {
			user = url.UserPassword(node.Username, node.Password)
		}
	}
	return encodeURL(node, scheme, user, query), nil
}

func encodeVMess(node model.Node) (string, error) {
	if node.Network != "" {
		return "", fmt.Errorf("VMess socket network cannot be represented in a VMess v2 share link")
	}
	type vmessLink struct {
		Version        string   `json:"v"`
		Name           string   `json:"ps,omitempty"`
		Server         string   `json:"add"`
		Port           string   `json:"port"`
		UUID           string   `json:"id"`
		AlterID        int      `json:"aid"`
		Security       string   `json:"scy,omitempty"`
		Transport      string   `json:"net"`
		TLS            string   `json:"tls"`
		SNI            string   `json:"sni,omitempty"`
		Host           string   `json:"host,omitempty"`
		Path           string   `json:"path,omitempty"`
		HeaderType     string   `json:"type"`
		ALPN           []string `json:"alpn,omitempty"`
		Fingerprint    string   `json:"fp,omitempty"`
		AllowInsecure  bool     `json:"allowInsecure,omitempty"`
		PacketEncoding string   `json:"packetEncoding,omitempty"`
	}
	transport := node.Transport
	if transport == "" || transport == "raw" {
		transport = "tcp"
	}
	tlsMode := "none"
	if hasTLS(node) {
		if node.TLSServerName == "" {
			return "", fmt.Errorf("VMess TLS share links require a TLS server name")
		}
		tlsMode = "tls"
	}
	value := vmessLink{
		Version: "2", Name: node.Name, Server: node.Server, Port: strconv.Itoa(node.ServerPort),
		UUID: node.UUID, AlterID: node.AlterID, Security: node.Security, Transport: transport,
		TLS: tlsMode, SNI: node.TLSServerName, Host: node.TransportHost, Path: node.TransportPath,
		HeaderType: "none", ALPN: node.ALPN, Fingerprint: node.UTLSFingerprint,
		AllowInsecure: node.Insecure, PacketEncoding: node.PacketEncoding,
	}
	if transport == "grpc" {
		value.Path = node.ServiceName
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode VMess share link: %w", err)
	}
	return "vmess://" + base64.RawURLEncoding.EncodeToString(encoded), nil
}

func encodeURL(node model.Node, scheme string, user *url.Userinfo, query url.Values) string {
	value := &url.URL{
		Scheme:   scheme,
		User:     user,
		Host:     net.JoinHostPort(node.Server, strconv.Itoa(node.ServerPort)),
		Fragment: node.Name,
	}
	if len(query) > 0 {
		value.RawQuery = query.Encode()
	}
	return value.String()
}

func setTLSQuery(query url.Values, node model.Node) {
	setIf(query, "sni", node.TLSServerName)
	if len(node.ALPN) > 0 {
		query.Set("alpn", strings.Join(node.ALPN, ","))
	}
	setIf(query, "fp", node.UTLSFingerprint)
	setTrue(query, "insecure", node.Insecure)
}

func setTransportQuery(query url.Values, node model.Node) {
	transport := node.Transport
	if transport == "" || transport == "raw" {
		transport = "tcp"
	}
	query.Set("type", transport)
	switch transport {
	case "ws", "http":
		setIf(query, "path", node.TransportPath)
		setIf(query, "host", node.TransportHost)
	case "grpc":
		setIf(query, "serviceName", node.ServiceName)
	}
}

func hasTLS(node model.Node) bool {
	return node.TLSServerName != "" || len(node.ALPN) > 0 || node.Insecure ||
		node.UTLSFingerprint != "" || node.RealityPublicKey != "" || node.RealityShortID != ""
}

func setIf(query url.Values, key, value string) {
	if value != "" {
		query.Set(key, value)
	}
}

func setTrue(query url.Values, key string, value bool) {
	if value {
		query.Set(key, "1")
	}
}

func setPositiveInt(query url.Values, key string, value int) {
	if value > 0 {
		query.Set(key, strconv.Itoa(value))
	}
}
