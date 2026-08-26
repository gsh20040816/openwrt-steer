// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import (
	"path/filepath"

	"github.com/gsh20040816/steer/go/internal/compiler"
	"github.com/gsh20040816/steer/go/internal/probe"
)

func ReadDiagnostics(configPath, runDirectory, stateDirectory string) probe.Diagnostics {
	diagnostics := probe.ReadDiagnostics(stateDirectory)
	if value, _, err := (IntentStore{Path: configPath}).Load(); err == nil {
		diagnostics.SavedDigest = compiler.IntentDigest(value)
	} else {
		diagnostics.Warnings = append(diagnostics.Warnings, "the Saved configuration identity is unavailable")
	}
	if generationID, resolved, _, identity, err := readCurrentIdentity(BackendOptions{RunDirectory: runDirectory}); err == nil {
		diagnostics.ActiveGeneration = generationID
		diagnostics.ActiveDigest = identity.IntentDigest
		diagnostics.DNSCapture = probe.InspectDNSCapture(
			"dedicated_shim", generationID, filepath.Join(resolved, "sing-box.json"), filepath.Join(resolved, "firewall.nft"),
		)
	} else {
		diagnostics.DNSCapture = probe.InspectDNSCapture("dedicated_shim", "", "", "")
	}
	return diagnostics
}
