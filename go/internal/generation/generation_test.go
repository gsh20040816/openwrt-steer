// SPDX-License-Identifier: GPL-3.0-or-later

package generation

import (
	"os"
	"path/filepath"
	"testing"

	model "github.com/gsh20040816/steer/go/internal/intent"
)

func TestCreateWritesOnlySharedFiles(t *testing.T) {
	value := model.Intent{Main: model.Main{ID: "main", SchemaVersion: model.SchemaVersion}}
	candidate, err := Create(filepath.Join(t.TempDir(), "generations"), value, map[string]any{"log": map[string]any{"level": "warn"}})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(candidate.Directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name() != "intent.json" || entries[1].Name() != "sing-box.json" {
		t.Fatalf("unexpected shared generation files: %#v", entries)
	}
	if _, err := ReadIntent(candidate.Directory); err != nil {
		t.Fatal(err)
	}
}
