// SPDX-License-Identifier: GPL-3.0-or-later

package openwrt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateSchema7WritesNarrowUCIBatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "steer")
	legacy := strings.Replace(minimalConfig, "schema_version '8'", "schema_version '7'", 1)
	legacy = strings.Replace(legacy, "config dns_profile 'direct_dns'", "config dns_profile 'direct_dns'\n\toption cache_persist '1'\n\toption optimistic_cache '1'", 1)
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	var batch string
	changed, err := MigrateSchema7WithWriter(context.Background(), path, func(_ context.Context, value string) error {
		batch = value
		return nil
	})
	if err != nil || !changed {
		t.Fatalf("migration failed: changed=%v err=%v", changed, err)
	}
	for _, line := range []string{
		"set steer.main.schema_version='8'", "delete steer.direct_dns.cache_persist",
		"delete steer.direct_dns.optimistic_cache", "commit steer",
	} {
		if !strings.Contains(batch, line) {
			t.Fatalf("migration batch is missing %q: %s", line, batch)
		}
	}
}

func TestMigrateSchema8IsNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "steer")
	if err := os.WriteFile(path, []byte(minimalConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := MigrateSchema7WithWriter(context.Background(), path, func(context.Context, string) error {
		t.Fatal("schema 8 migration attempted a write")
		return nil
	})
	if err != nil || changed {
		t.Fatalf("schema 8 migration result: changed=%v err=%v", changed, err)
	}
}
