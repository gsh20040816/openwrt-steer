// SPDX-License-Identifier: GPL-3.0-or-later

package openwrt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateSchema8WritesNarrowUCIBatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "steer")
	legacy := strings.Replace(minimalConfig, "schema_version '9'", "schema_version '8'", 1)
	legacy = strings.Replace(legacy, "config dns_profile 'direct_dns'", "config dns_profile 'direct_dns'\n\toption strategy 'ipv6_only'", 1)
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	var batch string
	changed, err := MigrateSchema8WithWriter(context.Background(), path, func(_ context.Context, value string) error {
		batch = value
		return nil
	})
	if err != nil || !changed {
		t.Fatalf("migration failed: changed=%v err=%v", changed, err)
	}
	for _, line := range []string{
		"set steer.main.schema_version='9'", "delete steer.direct_dns.strategy", "commit steer",
	} {
		if !strings.Contains(batch, line) {
			t.Fatalf("migration batch is missing %q: %s", line, batch)
		}
	}
}

func TestMigrateSchema9IsNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "steer")
	if err := os.WriteFile(path, []byte(minimalConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := MigrateSchema8WithWriter(context.Background(), path, func(context.Context, string) error {
		t.Fatal("schema 9 migration attempted a write")
		return nil
	})
	if err != nil || changed {
		t.Fatalf("schema 9 migration result: changed=%v err=%v", changed, err)
	}
}

func TestMigrateSchema7IsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "steer")
	legacy := strings.Replace(minimalConfig, "schema_version '9'", "schema_version '7'", 1)
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := MigrateSchema8WithWriter(context.Background(), path, func(context.Context, string) error {
		t.Fatal("schema 7 migration attempted a write")
		return nil
	})
	if err == nil || changed || !strings.Contains(err.Error(), `cannot migrate UCI schema "7"`) {
		t.Fatalf("schema 7 migration result: changed=%v err=%v", changed, err)
	}
}
