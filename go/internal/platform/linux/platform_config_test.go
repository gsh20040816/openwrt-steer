// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPlatformStoreTreatsMissingFileAsEmptySettings(t *testing.T) {
	store := PlatformStore{Path: filepath.Join(t.TempDir(), "platform.json")}
	value, revision, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if value != DefaultPlatformSettings() || revision == "" {
		t.Fatalf("missing platform settings = %#v revision %q", value, revision)
	}
	value.GeoSitePath = "/usr/share/v2ray/geosite.dat"
	newRevision, err := store.Save(value, revision)
	if err != nil || newRevision == revision {
		t.Fatalf("save platform settings: revision=%q err=%v", newRevision, err)
	}
	loaded, loadedRevision, err := store.Load()
	if err != nil || loaded != value || loadedRevision != newRevision {
		t.Fatalf("loaded platform settings = %#v revision=%q err=%v", loaded, loadedRevision, err)
	}
}

func TestPlatformStoreRejectsUnknownFieldsAndRelativePaths(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "platform.json")
	for name, content := range map[string]string{
		"unknown":  `{"schema_version":1,"provider":"example"}`,
		"relative": `{"schema_version":1,"geosite_path":"geosite.dat"}`,
		"version":  `{"schema_version":2}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := (PlatformStore{Path: path}).Load(); err == nil {
				t.Fatalf("invalid platform settings were accepted: %s", content)
			}
		})
	}
}

func TestPlatformStoreRejectsStaleRevision(t *testing.T) {
	store := PlatformStore{Path: filepath.Join(t.TempDir(), "platform.json")}
	_, revision, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	value := DefaultPlatformSettings()
	value.GeoIPPath = "/usr/share/v2ray/geoip.dat"
	if _, err := store.Save(value, revision); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(value, revision); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale platform revision error = %v", err)
	}
}
