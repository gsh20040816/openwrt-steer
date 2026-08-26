// SPDX-License-Identifier: GPL-3.0-or-later
package subscription

// These tests lock the platform-neutral subscription URI semantics.

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseStandardURIs(t *testing.T) {
	result, err := ParseList("vless://00000000-0000-4000-8000-000000000001@example.com:443?security=tls&sni=edge.example.com&type=ws&path=%2Fproxy\n" +
		"ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ@example.com:8388#ss")
	if err != nil {
		t.Fatal(err)
	}
	nodes := result.Nodes
	if len(nodes) != 2 || nodes[0].Type != "vless" || nodes[0].Transport != "ws" || nodes[1].Type != "shadowsocks" {
		t.Fatalf("unexpected parsed nodes: %#v", nodes)
	}
	if result.Skipped != 0 {
		t.Fatalf("valid nodes were skipped: %#v", result)
	}
	if Fingerprint(nodes[0]) != Fingerprint(nodes[0]) {
		t.Fatal("fingerprint is not stable")
	}
}

func TestParseBase64VMess(t *testing.T) {
	raw := `{"v":"2","ps":"fixture","add":"vmess.example.com","port":"443","id":"00000000-0000-4000-8000-000000000001","aid":"0","scy":"auto","net":"ws","tls":"tls","sni":"edge.example.com","host":"edge.example.com","path":"/ws","type":"none"}`
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	result, err := ParseList(encoded)
	if err != nil {
		t.Fatal(err)
	}
	nodes := result.Nodes
	if len(nodes) != 1 || nodes[0].Type != "vmess" || nodes[0].Transport != "ws" || nodes[0].Network != "" {
		t.Fatalf("unexpected VMess node: %#v", nodes)
	}
}

func TestCredentialProxyDefaultPortsFollowScheme(t *testing.T) {
	tests := []struct {
		raw      string
		nodeType string
		port     int
		tlsName  string
	}{
		{raw: "socks://proxy.example", nodeType: "socks", port: 1080},
		{raw: "socks5://proxy.example", nodeType: "socks", port: 1080},
		{raw: "http://proxy.example", nodeType: "http", port: 80},
		{raw: "https://proxy.example", nodeType: "http", port: 443, tlsName: "proxy.example"},
		{raw: "https://proxy.example:8443", nodeType: "http", port: 8443, tlsName: "proxy.example"},
	}
	for _, testCase := range tests {
		node, err := ParseURI(testCase.raw)
		if err != nil {
			t.Fatalf("ParseURI(%q): %v", testCase.raw, err)
		}
		if node.Type != testCase.nodeType || node.ServerPort != testCase.port || node.TLSServerName != testCase.tlsName {
			t.Errorf("ParseURI(%q) = %#v; want type=%q port=%d tls=%q", testCase.raw, node, testCase.nodeType, testCase.port, testCase.tlsName)
		}
	}

	result, err := ParseList("socks://socks.example\nhttp://http.example\nhttps://https.example")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 3 || result.Nodes[0].ServerPort != 1080 || result.Nodes[1].ServerPort != 80 || result.Nodes[2].ServerPort != 443 {
		t.Fatalf("subscription list did not reuse scheme defaults: %#v", result.Nodes)
	}
}

func TestRejectUnknownParameter(t *testing.T) {
	if _, err := ParseURI("tuic://00000000-0000-4000-8000-000000000001:password@example.com:443?unsupported=x"); err == nil {
		t.Fatal("unknown URI parameter was silently accepted")
	}
}

func TestRejectInvalidAndConflictingParameters(t *testing.T) {
	for _, raw := range []string{
		"tuic://00000000-0000-4000-8000-000000000001:secret@example.com:443?insecure=maybe",
		"vless://00000000-0000-4000-8000-000000000001@example.com:443?sni=a.example&serverName=b.example",
		"trojan://secret@example.com:443?sni=a.example&peer=b.example",
		"anytls://secret@example.com:443?type=ws",
	} {
		if _, err := ParseURI(raw); err == nil {
			t.Fatalf("invalid URI was accepted: %s", raw)
		}
	}
}

func TestParsePassWallCompatibleAliases(t *testing.T) {
	result, err := ParseList(
		"trojan://secret@example.com:443?type=tcp&sni=edge.example&peer=edge.example&allowInsecure=1#trojan\n" +
			"anytls://secret@example.com:443?type=tcp&sni=edge.example&insecure=1#anytls\n" +
			"hysteria://secret@example.com:443?peer=edge.example&allowInsecure=1&upmbps=100&downmbps=100#hysteria\n")
	if err != nil {
		t.Fatal(err)
	}
	nodes := result.Nodes
	if len(nodes) != 3 {
		t.Fatalf("unexpected compatible node count: %d", len(nodes))
	}
	for _, node := range nodes {
		if node.TLSServerName != "edge.example" || !node.Insecure {
			t.Fatalf("compatibility aliases were not lowered explicitly: %#v", node)
		}
	}
}

func TestParseListSkipsOnlyInvalidNodes(t *testing.T) {
	result, err := ParseList("not-a-node\n" +
		"socks5://user:password@example.com:1080#SOCKS\n" +
		"trojan://secret@example.com:443?sni=edge.example#bad%0Aname\n")
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped != 2 || len(result.Nodes) != 1 || result.Nodes[0].Type != "socks" {
		t.Fatalf("unexpected lenient parse result: %#v", result)
	}
}

func TestParseListAllowsAllNodesToBeSkipped(t *testing.T) {
	result, err := ParseList("not-a-node\nstill-not-a-node\n")
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped != 2 || len(result.Nodes) != 0 {
		t.Fatalf("all-invalid subscription did not become an empty result: %#v", result)
	}
}

func TestParseOpaqueBase64PayloadsContainingSlash(t *testing.T) {
	vmessJSON := `{"v":"2","ps":"࠿","add":"vmess.example.com","port":"443","id":"00000000-0000-4000-8000-000000000001","aid":"0","scy":"auto","net":"tcp","tls":"none","type":"none"}`
	vmessPayload := base64.StdEncoding.EncodeToString([]byte(vmessJSON))
	if !strings.Contains(vmessPayload, "/") {
		t.Fatal("VMess fixture does not exercise a slash in standard Base64")
	}
	vmess, err := ParseURI("vmess://" + vmessPayload)
	if err != nil || vmess.Name != "࠿" {
		t.Fatalf("VMess slash payload: node=%#v err=%v", vmess, err)
	}

	ssPayload := base64.StdEncoding.EncodeToString([]byte("2022-blake3-aes-128-gcm:࠿@example.com:8388"))
	if !strings.Contains(ssPayload, "/") {
		t.Fatal("Shadowsocks fixture does not exercise a slash in standard Base64")
	}
	shadowsocks, err := ParseURI("ss://" + ssPayload + "#SS2022")
	if err != nil || shadowsocks.Method != "2022-blake3-aes-128-gcm" || shadowsocks.Password != "࠿" {
		t.Fatalf("Shadowsocks slash payload: node=%#v err=%v", shadowsocks, err)
	}
}

func TestParseCompleteSIP002Forms(t *testing.T) {
	for name, raw := range map[string]string{
		"plaintext 2022":     "ss://2022-blake3-aes-128-gcm:password@example.com:8388#SS2022",
		"plaintext escaped":  "ss://2022-blake3-aes-128-gcm:p%40ss%3Aword%2Fkey@example.com:8388",
		"encoded userinfo":   "ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ@example.com:8388",
		"legacy password at": "ss://" + base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:p@ssword@example.com:8388")),
		"standard plugin":    "ss://aes-256-gcm:password@example.com:8388?plugin=obfs-local%3Bobfs%3Dhttp%3Bobfs-host%3Dexample.com",
	} {
		node, err := ParseURI(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if node.Type != "shadowsocks" || node.Server != "example.com" || node.ServerPort != 8388 {
			t.Fatalf("%s: unexpected node %#v", name, node)
		}
		if name == "legacy password at" && node.Password != "p@ssword" {
			t.Fatalf("legacy @ password was truncated: %#v", node)
		}
		if name == "plaintext escaped" && node.Password != "p@ss:word/key" {
			t.Fatalf("escaped plaintext password was truncated: %#v", node)
		}
		if name == "standard plugin" && (node.Plugin != "obfs-local" || node.PluginOptions != "obfs=http;obfs-host=example.com") {
			t.Fatalf("standard plugin was not split: %#v", node)
		}
	}
}

func TestParseVLESSLowersSecurityIntoCanonicalTLSFields(t *testing.T) {
	tlsNode, err := ParseURI("vless://00000000-0000-4000-8000-000000000001@example.com:443?encryption=none&security=tls&sni=edge.example.com&fp=chrome")
	if err != nil {
		t.Fatal(err)
	}
	if tlsNode.Security != "" || tlsNode.TLSServerName != "edge.example.com" || tlsNode.UTLSFingerprint != "chrome" {
		t.Fatalf("VLESS TLS was not lowered into canonical fields: %#v", tlsNode)
	}
	realityNode, err := ParseURI("vless://00000000-0000-4000-8000-000000000001@example.com:443?security=reality&sni=edge.example.com&fp=chrome&pbk=public-key&sid=0123456789abcdef&flow=xtls-rprx-vision")
	if err != nil {
		t.Fatal(err)
	}
	if realityNode.Security != "" || realityNode.RealityPublicKey != "public-key" || realityNode.RealityShortID != "0123456789abcdef" {
		t.Fatalf("VLESS Reality was not lowered into canonical fields: %#v", realityNode)
	}
}

func TestParseVLESSRejectsSecurityContradictions(t *testing.T) {
	for _, raw := range []string{
		"vless://00000000-0000-4000-8000-000000000001@example.com:443?security=tls",
		"vless://00000000-0000-4000-8000-000000000001@example.com:443?security=none&sni=edge.example.com",
		"vless://00000000-0000-4000-8000-000000000001@example.com:443?security=none&flow=xtls-rprx-vision",
		"vless://00000000-0000-4000-8000-000000000001@example.com:443?security=tls&sni=edge.example.com&pbk=public-key",
		"vless://00000000-0000-4000-8000-000000000001@example.com:443?security=unsupported",
		"vless://00000000-0000-4000-8000-000000000001@example.com:443?encryption=aes-128-gcm",
	} {
		if _, err := ParseURI(raw); err == nil {
			t.Fatalf("contradictory VLESS URI was accepted: %s", raw)
		}
	}
}

func TestParseHysteriaMPortPreservesSingleAndMultipleRanges(t *testing.T) {
	for raw, expected := range map[string][]string{
		"hy2://secret@example.com:443?mport=20000:30000":       {"20000:30000"},
		"hy2://secret@example.com:443?mport=12000-12010,13000": {"12000:12010", "13000"},
	} {
		node, err := ParseURI(raw)
		if err != nil {
			t.Fatal(err)
		}
		if node.ServerPort != 443 || len(node.ServerPorts) != len(expected) {
			t.Fatalf("unexpected Hysteria2 ports for %s: %#v", raw, node)
		}
		for index := range expected {
			if node.ServerPorts[index] != expected[index] {
				t.Fatalf("Hysteria2 ports for %s = %#v, want %#v", raw, node.ServerPorts, expected)
			}
		}
	}
}
