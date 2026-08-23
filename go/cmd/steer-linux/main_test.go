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

func TestGeoCatalogLoadsConfiguredPlatformPath(t *testing.T) {
	root := t.TempDir()
	geoSitePath := filepath.Join(root, "geosite.dat")
	if err := os.WriteFile(geoSitePath, []byte("site"), 0o600); err != nil {
		t.Fatal(err)
	}
	platformPath := filepath.Join(root, "platform.json")
	settings := linuxplatform.DefaultPlatformSettings()
	settings.GeoSitePath = geoSitePath
	if _, err := (linuxplatform.PlatformStore{Path: platformPath}).Save(settings, ""); err != nil {
		t.Fatal(err)
	}
	geoViewPath := filepath.Join(root, "geoview")
	if err := os.WriteFile(geoViewPath, []byte("#!/bin/sh\nprintf 'Available codes:\\ncn\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := runGeoCatalog([]string{"-kind", "geosite", "-platform", platformPath, "-geoview", geoViewPath}); err != nil {
		t.Fatalf("geo-catalog did not use platform settings: %v", err)
	}
}
