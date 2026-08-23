// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNewPathsRequiresAbsoluteAppGroupRoot(t *testing.T) {
	if _, err := NewPaths("relative"); err == nil {
		t.Fatal("relative App Group root was accepted")
	}
	paths, err := NewPaths(filepath.Join(t.TempDir(), "group.container"))
	if err != nil {
		t.Fatal(err)
	}
	if paths.ConfigPath != filepath.Join(paths.Root, "config", "config.json") || paths.StatusPath != filepath.Join(paths.Root, "status", "current.json") {
		t.Fatalf("unexpected App Group paths: %#v", paths)
	}
}

func TestIntentStoreUsesRevisionGuard(t *testing.T) {
	paths, err := NewPaths(filepath.Join(t.TempDir(), "group.container"))
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store := IntentStore{Paths: paths}
	value := validIntent()
	revision, err := store.Save(value, "")
	if err != nil {
		t.Fatal(err)
	}
	loaded, loadedRevision, err := store.Load()
	if err != nil || loaded.Main.ID != value.Main.ID || loadedRevision != revision {
		t.Fatalf("unexpected stored intent: %#v %q %v", loaded, loadedRevision, err)
	}
	if _, err := store.Save(value, "stale"); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale revision did not fail fast: %v", err)
	}
}

func TestStatusRoundTripRejectsUnknownFields(t *testing.T) {
	paths, err := NewPaths(filepath.Join(t.TempDir(), "group.container"))
	if err != nil {
		t.Fatal(err)
	}
	status := DefaultStatus()
	status.Healthy = true
	status.GenerationID = "generation"
	if err := paths.SaveStatus(status); err != nil {
		t.Fatal(err)
	}
	loaded, err := paths.LoadStatus()
	if err != nil || loaded.GenerationID != status.GenerationID || !loaded.Healthy {
		t.Fatalf("unexpected status: %#v %v", loaded, err)
	}
	content, err := os.ReadFile(paths.StatusPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) == 0 {
		t.Fatal("status file is empty")
	}
}

func TestPreparePublishAndLoadCurrentGeneration(t *testing.T) {
	paths, err := NewPaths(filepath.Join(t.TempDir(), "group.container"))
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := Prepare(validIntent(), paths)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"intent.json", "sing-box.json", "macos.json", "generation.json"} {
		if _, err := os.Stat(filepath.Join(prepared.Directory, name)); err != nil {
			t.Fatalf("prepared generation is missing %s: %v", name, err)
		}
	}
	if err := paths.PublishHealthy(prepared,
		ProviderHealth{Provider: "packet-tunnel", GenerationID: prepared.Metadata.GenerationID, Healthy: true},
		ProviderHealth{Provider: "dns-proxy", GenerationID: prepared.Metadata.GenerationID, Healthy: true},
	); err != nil {
		t.Fatal(err)
	}
	current, err := paths.LoadCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if current.GenerationID != prepared.Metadata.GenerationID || current.Directory != filepath.Base(prepared.Directory) {
		t.Fatalf("published generation mismatch: %#v %#v", current, prepared.Metadata)
	}
}

func TestPublishHealthyRejectsMismatchedProvider(t *testing.T) {
	paths, err := NewPaths(filepath.Join(t.TempDir(), "group.container"))
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := Prepare(validIntent(), paths)
	if err != nil {
		t.Fatal(err)
	}
	err = paths.PublishHealthy(prepared,
		ProviderHealth{Provider: "packet-tunnel", GenerationID: prepared.Metadata.GenerationID, Healthy: true},
		ProviderHealth{Provider: "dns-proxy", GenerationID: "wrong", Healthy: true},
	)
	if err == nil {
		t.Fatal("mismatched provider generation was published")
	}
}
