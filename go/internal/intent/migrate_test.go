// SPDX-License-Identifier: GPL-3.0-or-later

package intent

import (
	"bytes"
	"testing"
)

func TestMigrateJSON7DropsOnlyProfileCacheFlags(t *testing.T) {
	raw := []byte(`{
  "main": {"id":"main","schema_version":7,"enabled":true,"log_level":"info","dns_cache_persist":true,"dns_optimistic_cache":true},
  "bootstrap": {"id":"bootstrap","protocol":"udp","server":"1.1.1.1","server_port":53,"strategy":"prefer_ipv4"},
  "nodes": [], "subscriptions": [], "routes": [],
  "dns_profiles": [{"id":"public","enabled":true,"protocol":"udp","server":"1.1.1.1","server_port":53,"strategy":"prefer_ipv4","cache_persist":true,"optimistic_cache":true}],
  "local_proxies": [], "rules": []
}`)
	value, err := MigrateJSON7(raw)
	if err != nil {
		t.Fatal(err)
	}
	if value.Main.SchemaVersion != SchemaVersion || !value.Main.DNSCachePersist || !value.Main.DNSOptimisticCache {
		t.Fatalf("global schema/cache fields changed: %#v", value.Main)
	}
	var encoded bytes.Buffer
	if err := EncodeJSON(&encoded, value); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded.Bytes(), []byte(`"cache_persist"`)) || bytes.Contains(encoded.Bytes(), []byte(`"optimistic_cache"`)) {
		t.Fatalf("removed profile fields remain: %s", encoded.String())
	}
}

func TestMigrateJSON7RejectsUnknownAndWrongSchema(t *testing.T) {
	for _, raw := range []string{
		`{"main":{"schema_version":8}}`,
		`{"main":{"schema_version":7,"unknown":true}}`,
		`{"main":{"schema_version":7}} {}`,
	} {
		if _, err := MigrateJSON7([]byte(raw)); err == nil {
			t.Fatalf("invalid schema 7 input was accepted: %s", raw)
		}
	}
}
