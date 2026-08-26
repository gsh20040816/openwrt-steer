// SPDX-License-Identifier: GPL-3.0-or-later

package openwrt

import (
	"os"
	"path/filepath"

	"github.com/gsh20040816/steer/go/internal/compiler"
	"github.com/gsh20040816/steer/go/internal/generation"
	"github.com/gsh20040816/steer/go/internal/probe"
)

func ReadDiagnostics(configPath, runDirectory, stateDirectory string) probe.Diagnostics {
	diagnostics := probe.ReadDiagnostics(stateDirectory)
	if config, err := os.ReadFile(configPath); err == nil {
		if value, decodeErr := DecodeBytes(config); decodeErr == nil {
			diagnostics.SavedDigest = compiler.IntentDigest(value)
		} else {
			diagnostics.Warnings = append(diagnostics.Warnings, "the Saved configuration identity is unavailable")
		}
	} else {
		diagnostics.Warnings = append(diagnostics.Warnings, "the Saved configuration identity is unavailable")
	}
	if runDirectory == "" {
		runDirectory = "/run/steer"
	}
	currentPath := filepath.Join(runDirectory, "current")
	if value, err := generation.ReadIntent(currentPath); err == nil {
		diagnostics.ActiveDigest = compiler.IntentDigest(value)
		diagnostics.ActiveGeneration = currentGenerationID(currentPath)
	}
	diagnostics.DNSCapture = probe.InspectDNSCapture(
		"dedicated_shim", diagnostics.ActiveGeneration,
		filepath.Join(currentPath, "sing-box.json"), filepath.Join(currentPath, "firewall.nft"),
	)
	return diagnostics
}

func currentGenerationID(currentPath string) string {
	info, err := os.Lstat(currentPath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(currentPath)
	if err != nil {
		return ""
	}
	return filepath.Base(resolved)
}
