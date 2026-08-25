// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	model "github.com/gsh20040816/steer/go/internal/intent"
)

func validIntent() model.Intent {
	return model.Intent{
		Main:        model.Main{ID: "main", SchemaVersion: model.SchemaVersion, Enabled: false, LogLevel: "warn", ProbeDirectURL: "https://direct.example/", ProbeProxyURL: "https://proxy.example/", SpeedtestProxyURL: "https://speed.example/"},
		Bootstrap:   model.Bootstrap{ID: "bootstrap", Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"},
		Routes:      []model.Route{{ID: "direct", Enabled: true, Kind: "direct"}, {ID: "block", Enabled: true, Kind: "block"}},
		DNSProfiles: []model.DNSProfile{{ID: "public", Enabled: true, Protocol: "udp", Server: "1.1.1.1", ServerPort: 53}},
		Rules:       []model.Rule{{ID: "default", Enabled: true, Default: true, DNSProfile: "public", Route: "direct"}},
	}
}

func TestIntentStoreMigratesSchema8Atomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := IntentStore{Path: path}
	if _, err := store.Save(validIntent(), ""); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content = bytes.Replace(content, []byte(`"schema_version": 9`), []byte(`"schema_version": 8`), 1)
	profileStart := bytes.Index(content, []byte(`"dns_profiles"`))
	serverPortMarker := []byte(`"server_port": 53`)
	serverPortStart := bytes.Index(content[profileStart:], serverPortMarker)
	if profileStart < 0 || serverPortStart < 0 {
		t.Fatalf("cannot locate DNS profile in %s", content)
	}
	insertAt := profileStart + serverPortStart + len(serverPortMarker)
	content = append(content[:insertAt], append([]byte(",\n      \"strategy\": \"ipv6_only\""), content[insertAt:]...)...)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := store.MigrateSchema8()
	if err != nil || !changed {
		t.Fatalf("migration failed: changed=%v err=%v", changed, err)
	}
	value, _, err := store.Load()
	if err != nil || value.Main.SchemaVersion != model.SchemaVersion {
		t.Fatalf("migrated intent failed to load: value=%#v err=%v", value, err)
	}
	migrated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(migrated, []byte(`"strategy": "ipv6_only"`)) {
		t.Fatalf("removed profile strategy remains: %s", migrated)
	}
}

func TestIntentStoreRoundTripAndRevisionConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := IntentStore{Path: path}
	value := validIntent()
	rev, err := store.Save(value, "")
	if err != nil {
		t.Fatal(err)
	}
	loaded, loadedRev, err := store.Load()
	if err != nil || loaded.Main.ID != value.Main.ID || loadedRev != rev {
		t.Fatalf("round trip failed: value=%#v revision=%q err=%v", loaded, loadedRev, err)
	}
	if _, err := store.Save(value, `"sha256-invalid"`); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale revision returned %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions are %o", info.Mode().Perm())
	}
}

func TestLinuxValidationAcceptsNativeSourceMAC(t *testing.T) {
	value := validIntent()
	macRule := model.Rule{ID: "mac", Enabled: true, SourceMACAddress: []string{"02:00:00:00:00:10"}, DNSProfile: "public", Route: "direct"}
	value.Rules = append(value.Rules[:len(value.Rules)-1], macRule, value.Rules[len(value.Rules)-1])
	validation := Validate(value)
	if !validation.OK {
		t.Fatalf("sing-box 1.14 native source MAC rule was rejected on Linux: %#v", validation.Errors)
	}
}
