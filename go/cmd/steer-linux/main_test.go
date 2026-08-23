// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"testing"

	linuxplatform "github.com/gsh20040816/steer/go/internal/platform/linux"
)

func TestRunServiceDisabledConfigurationExitsCleanly(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	if _, err := (linuxplatform.JSONStore{Path: configPath}).Save(webTestIntent(), ""); err != nil {
		t.Fatal(err)
	}
	nftPath := filepath.Join(root, "nft")
	if err := os.WriteFile(nftPath, []byte("#!/bin/sh\nprintf '%s' '{\"nftables\":[]}'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := runService([]string{
		"-config", configPath,
		"-run-dir", filepath.Join(root, "run"),
		"-state-dir", filepath.Join(root, "state"),
		"-nft", nftPath,
	}); err != nil {
		t.Fatalf("disabled service entered a failure path: %v", err)
	}
}
