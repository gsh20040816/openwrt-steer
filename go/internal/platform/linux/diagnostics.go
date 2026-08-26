// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import (
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
	if generationID, _, _, identity, err := readCurrentIdentity(BackendOptions{RunDirectory: runDirectory}); err == nil {
		diagnostics.ActiveGeneration = generationID
		diagnostics.ActiveDigest = identity.IntentDigest
	}
	return diagnostics
}
