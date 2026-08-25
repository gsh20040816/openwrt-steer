// SPDX-License-Identifier: GPL-3.0-or-later

package intent

import (
	"bytes"
	"testing"
)

func TestMigrateJSON8DropsProfileStrategies(t *testing.T) {
	raw := []byte(`{
  "main": {"id":"main","schema_version":8,"enabled":true,"log_level":"info","probe_direct":"https://direct.example/","probe_proxy":"https://proxy.example/","speedtest_proxy":"https://speed.example/"},
  "bootstrap": {"id":"bootstrap","protocol":"udp","server":"1.1.1.1","server_port":53,"strategy":"prefer_ipv6"},
  "nodes": [], "subscriptions": [], "routes": [{"id":"direct","enabled":true,"kind":"direct"}],
  "dns_profiles": [{"id":"public","enabled":true,"protocol":"udp","server":"1.1.1.1","server_port":53,"strategy":"ipv6_only"}],
  "local_proxies": [], "rules": [{"id":"default","enabled":true,"default":true,"dns_profile":"public","route":"direct"}]
}`)
	value, err := MigrateJSON8(raw)
	if err != nil {
		t.Fatal(err)
	}
	if value.Main.SchemaVersion != SchemaVersion {
		t.Fatalf("schema migration is wrong: %#v", value.Main)
	}
	if value.Bootstrap.Strategy != "prefer_ipv6" {
		t.Fatalf("bootstrap domain resolution strategy changed: %#v", value.Bootstrap)
	}
	var encoded bytes.Buffer
	if err := EncodeJSON(&encoded, value); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded.Bytes(), []byte(`"strategy":"ipv6_only"`)) || bytes.Contains(encoded.Bytes(), []byte(`"strategy": "ipv6_only"`)) {
		t.Fatalf("removed profile strategy remains: %s", encoded.String())
	}
}

func TestMigrateJSON8RejectsUnknownAndWrongSchema(t *testing.T) {
	for _, raw := range []string{
		`{"main":{"schema_version":7}}`,
		`{"main":{"schema_version":8,"unknown":true}}`,
		`{"main":{"schema_version":8}} {}`,
	} {
		if _, err := MigrateJSON8([]byte(raw)); err == nil {
			t.Fatalf("invalid schema 8 input was accepted: %s", raw)
		}
	}
}
