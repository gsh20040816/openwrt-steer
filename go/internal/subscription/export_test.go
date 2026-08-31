// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gsh20040816/steer/go/internal/compiler"
	model "github.com/gsh20040816/steer/go/internal/intent"
)

func TestEncodeURIRoundTripsEveryShareableNodeType(t *testing.T) {
	const uuid = "00000000-0000-4000-8000-000000000001"
	nodes := []model.Node{
		{ID: "socks", Enabled: true, Name: "SOCKS / edge", Type: "socks", Server: "2001:db8::1", ServerPort: 1080, NodeCredentials: model.NodeCredentials{Username: "user@example", Password: "p@ss:word"}},
		{ID: "http", Enabled: true, Name: "HTTPS", Type: "http", Server: "proxy.example", ServerPort: 8443, NodeCredentials: model.NodeCredentials{Username: "user", Password: "secret"}, NodeTLS: model.NodeTLS{TLSServerName: "edge.example", ALPN: []string{"h2", "http/1.1"}, Insecure: true, UTLSFingerprint: "chrome"}},
		{ID: "ss", Enabled: true, Name: "SS2022", Type: "shadowsocks", Server: "ss.example", ServerPort: 8388, NodeCredentials: model.NodeCredentials{Password: "p@ss:word"}, NodeProtocol: model.NodeProtocol{Method: "2022-blake3-aes-128-gcm", Plugin: "obfs-local", PluginOptions: "obfs=http;obfs-host=edge.example"}},
		{ID: "vmess", Enabled: true, Name: "VMess", Type: "vmess", Server: "vmess.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{UUID: uuid}, NodeTransport: model.NodeTransport{Transport: "ws", TransportPath: "/ws", TransportHost: "edge.example", PacketEncoding: "xudp"}, NodeProtocol: model.NodeProtocol{Security: "auto"}, NodeTLS: model.NodeTLS{TLSServerName: "edge.example", ALPN: []string{"h2"}, Insecure: true, UTLSFingerprint: "chrome"}},
		{ID: "vmess-grpc", Enabled: true, Name: "VMess gRPC", Type: "vmess", Server: "vmess-grpc.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{UUID: uuid}, NodeTransport: model.NodeTransport{Transport: "grpc", ServiceName: "proxy"}, NodeProtocol: model.NodeProtocol{Security: "auto"}, NodeTLS: model.NodeTLS{TLSServerName: "edge.example"}},
		{ID: "vless", Enabled: true, Name: "VLESS", Type: "vless", Server: "vless.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{UUID: uuid}, NodeTransport: model.NodeTransport{Transport: "ws", TransportPath: "/proxy", TransportHost: "edge.example", PacketEncoding: "xudp", Flow: "xtls-rprx-vision"}, NodeTLS: model.NodeTLS{TLSServerName: "edge.example", ALPN: []string{"h2"}, RealityPublicKey: "public-key", RealityShortID: "0123456789abcdef", UTLSFingerprint: "chrome"}},
		{ID: "trojan", Enabled: true, Name: "Trojan", Type: "trojan", Server: "trojan.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{Password: "secret"}, NodeTransport: model.NodeTransport{Transport: "grpc", ServiceName: "proxy"}, NodeTLS: model.NodeTLS{TLSServerName: "edge.example", ALPN: []string{"h2"}, UTLSFingerprint: "firefox"}},
		{ID: "hysteria", Enabled: true, Name: "Hysteria", Type: "hysteria", Server: "hy.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{Password: "secret"}, NodeProtocol: model.NodeProtocol{ServerPorts: []string{"20000:20010", "21000"}, HopInterval: "30s", ObfsPassword: "obfs", UpMbps: 100, DownMbps: 200}, NodeTLS: model.NodeTLS{TLSServerName: "edge.example", ALPN: []string{"h3"}, UTLSFingerprint: "chrome"}},
		{ID: "hysteria2", Enabled: true, Name: "Hysteria2", Type: "hysteria2", Server: "hy2.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{Password: "secret"}, NodeProtocol: model.NodeProtocol{ServerPorts: []string{"20000:20010"}, HopInterval: "30s", ObfsType: "salamander", ObfsPassword: "obfs", UpMbps: 100, DownMbps: 200}, NodeTLS: model.NodeTLS{TLSServerName: "edge.example", ALPN: []string{"h3"}, Insecure: true, UTLSFingerprint: "chrome"}},
		{ID: "shadowtls", Enabled: true, Name: "ShadowTLS", Type: "shadowtls", Server: "shadow.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{Password: "secret"}, NodeProtocol: model.NodeProtocol{Version: 3}, NodeTLS: model.NodeTLS{TLSServerName: "edge.example", ALPN: []string{"h2"}, UTLSFingerprint: "chrome"}},
		{ID: "tuic", Enabled: true, Name: "TUIC", Type: "tuic", Server: "tuic.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{UUID: uuid, Password: "secret"}, NodeProtocol: model.NodeProtocol{CongestionControl: "bbr", UDPRelayMode: "quic", ZeroRTTHandshake: true, Heartbeat: "10s"}, NodeTLS: model.NodeTLS{TLSServerName: "edge.example", ALPN: []string{"h3", "h2"}, UTLSFingerprint: "chrome"}},
		{ID: "anytls", Enabled: true, Name: "AnyTLS", Type: "anytls", Server: "anytls.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{Password: "secret"}, NodeTLS: model.NodeTLS{TLSServerName: "edge.example", ALPN: []string{"h2"}, UTLSFingerprint: "chrome"}},
		{ID: "naive", Enabled: true, Name: "Naive", Type: "naive", Server: "naive.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{Username: "user", Password: "secret"}, NodeProtocol: model.NodeProtocol{QUIC: true, QUICCongestionControl: "bbr", InsecureConcurrency: 2}, NodeTLS: model.NodeTLS{TLSServerName: "edge.example", ALPN: []string{"h3"}, UTLSFingerprint: "chrome"}},
		{ID: "ssh", Enabled: true, Name: "SSH", Type: "ssh", Server: "ssh.example", ServerPort: 22, NodeCredentials: model.NodeCredentials{Username: "root", Password: "secret"}},
	}

	for _, node := range nodes {
		t.Run(node.Type, func(t *testing.T) {
			raw, err := EncodeURI(node)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := ParseURI(raw)
			if err != nil {
				t.Fatalf("ParseURI(%q): %v", raw, err)
			}
			parsed.ID = node.ID
			if got, want := compiler.CompileNodeOutbound(parsed), compiler.CompileNodeOutbound(node); !reflect.DeepEqual(got, want) {
				t.Fatalf("round trip changed outbound\nlink: %s\ngot:  %#v\nwant: %#v", raw, got, want)
			}
		})
	}
}

func TestEncodeURIRefusesLossyOrManualOnlyNodes(t *testing.T) {
	const uuid = "00000000-0000-4000-8000-000000000001"
	tests := []model.Node{
		{Enabled: true, Type: "tor", NodeProtocol: model.NodeProtocol{ExecutablePath: "/usr/bin/tor"}},
		{Enabled: true, Type: "ssh", Server: "ssh.example", ServerPort: 22, NodeCredentials: model.NodeCredentials{Username: "root", PrivateKey: "private-key"}},
		{Enabled: true, Type: "vmess", Server: "vmess.example", ServerPort: 443, NodeCredentials: model.NodeCredentials{UUID: uuid}, NodeTransport: model.NodeTransport{Network: "udp", Transport: "tcp"}},
	}
	for _, node := range tests {
		if _, err := EncodeURI(node); err == nil {
			t.Fatalf("lossy %s node unexpectedly exported", node.Type)
		}
	}
}

func TestEncodeURIEscapesCredentialsAndNames(t *testing.T) {
	node := model.Node{Enabled: true, Type: "socks", Name: "日本 / edge", Server: "proxy.example", ServerPort: 1080,
		NodeCredentials: model.NodeCredentials{Username: "user@name", Password: "p@ss:/?#"}}
	raw, err := EncodeURI(node)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "p@ss") {
		t.Fatalf("credential was not URL-escaped: %s", raw)
	}
	parsed, err := ParseURI(raw)
	if err != nil || parsed.Username != node.Username || parsed.Password != node.Password || parsed.Name != node.Name {
		t.Fatalf("escaped values did not round trip: node=%#v err=%v raw=%s", parsed, err, raw)
	}
}
