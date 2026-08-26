// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"github.com/gsh20040816/steer/go/internal/compiler"
	"github.com/gsh20040816/steer/go/internal/probe"
)

func ReadDiagnostics(configPath string, options BackendOptions) probe.Diagnostics {
	options = normalizeBackendOptions(options)
	diagnostics := probe.ReadDiagnostics(options.StateDirectory)
	if value, _, err := (IntentStore{Paths: Paths{ConfigPath: configPath}}).Load(); err == nil {
		diagnostics.SavedDigest = compiler.IntentDigest(value)
	} else {
		diagnostics.Warnings = append(diagnostics.Warnings, "the Saved configuration identity is unavailable")
	}
	if current, err := runtimePaths(options.RunDirectory, "").LoadCurrent(); err == nil {
		diagnostics.ActiveGeneration = current.GenerationID
		diagnostics.ActiveDigest = current.IntentDigest
	}
	return diagnostics
}

func SaveTestReport(options BackendOptions, report TestReport) error {
	options = normalizeBackendOptions(options)
	return probe.SaveReport(options.StateDirectory, report)
}
