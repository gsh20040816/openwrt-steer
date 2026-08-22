// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	model "github.com/gsh20040816/openwrt-steer/go/internal/intent"
)

func validIntent() model.Intent {
	return model.Intent{
		Main:        model.Main{ID: "main", SchemaVersion: model.SchemaVersion, Enabled: false, LogLevel: "warn", ProbeDirectURL: "https://direct.example/", ProbeProxyURL: "https://proxy.example/", SpeedtestProxyURL: "https://speed.example/"},
		Bootstrap:   model.Bootstrap{ID: "bootstrap", Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"},
		Routes:      []model.Route{{ID: "direct", Enabled: true, Kind: "direct"}, {ID: "block", Enabled: true, Kind: "block"}},
		DNSProfiles: []model.DNSProfile{{ID: "public", Enabled: true, Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"}},
		Rules:       []model.Rule{{ID: "default", Enabled: true, Default: true, DNSProfile: "public", Route: "direct"}},
	}
}

func TestJSONStoreRoundTripAndRevisionConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := JSONStore{Path: path}
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

func TestLinuxValidationRejectsSourceMAC(t *testing.T) {
	value := validIntent()
	value.Rules = append(value.Rules, model.Rule{ID: "mac", Enabled: true, SourceMACAddress: []string{"02:00:00:00:00:10"}, DNSProfile: "public", Route: "direct"})
	validation := Validate(value)
	if validation.OK {
		t.Fatal("source MAC rule was accepted on Linux")
	}
	found := false
	for _, issue := range validation.Errors {
		if issue.Code == "PLATFORM_UNSUPPORTED_SOURCE_MAC" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing explicit source MAC platform error: %#v", validation.Errors)
	}
}
