// SPDX-License-Identifier: GPL-3.0-or-later
package subscription

import (
	"encoding/base64"
	"testing"
)

func TestParseStandardURIs(t *testing.T) {
	nodes, err := ParseList("vless://00000000-0000-4000-8000-000000000001@example.com:443?security=tls&sni=edge.example.com&type=ws&path=%2Fproxy\n" +
		"ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ@example.com:8388#ss")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].Type != "vless" || nodes[0].Transport != "ws" || nodes[1].Type != "shadowsocks" {
		t.Fatalf("unexpected parsed nodes: %#v", nodes)
	}
	if Fingerprint(nodes[0]) != Fingerprint(nodes[0]) {
		t.Fatal("fingerprint is not stable")
	}
}

func TestParseBase64VMess(t *testing.T) {
	raw := `{"v":"2","ps":"fixture","add":"vmess.example.com","port":"443","id":"00000000-0000-4000-8000-000000000001","aid":"0","scy":"auto","net":"ws","tls":"tls","sni":"edge.example.com","host":"edge.example.com","path":"/ws","type":"none"}`
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	nodes, err := ParseList(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Type != "vmess" || nodes[0].Transport != "ws" || nodes[0].Network != "" {
		t.Fatalf("unexpected VMess node: %#v", nodes)
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
	nodes, err := ParseList(
		"trojan://secret@example.com:443?type=tcp&sni=edge.example&peer=edge.example&allowInsecure=1#trojan\n" +
			"anytls://secret@example.com:443?type=tcp&sni=edge.example&insecure=1#anytls\n" +
			"hysteria://secret@example.com:443?peer=edge.example&allowInsecure=1#hysteria\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("unexpected compatible node count: %d", len(nodes))
	}
	for _, node := range nodes {
		if node.TLSServerName != "edge.example" || !node.Insecure {
			t.Fatalf("compatibility aliases were not lowered explicitly: %#v", node)
		}
	}
}
