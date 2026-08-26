// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import "testing"

func TestReadDiagnosticsInspectsCurrentGenerationDNSCaptureArtifacts(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := loadProbeTestIntent(t)
	value.Main.Enabled = true
	writeProbeTestIntent(t, paths.ConfigPath, value)
	publishProbeTestIntent(t, paths, value)

	diagnostics := ReadDiagnostics(paths.ConfigPath, BackendOptions{
		RunDirectory: paths.Root, StateDirectory: paths.StateDirectory,
	})
	if !diagnostics.DNSCapture.Configured || diagnostics.DNSCapture.ActiveGeneration == "" ||
		diagnostics.DNSCapture.Mode != "tun_port53_hijack" {
		t.Fatalf("current macOS DNS capture artifacts were not diagnosed: %#v", diagnostics.DNSCapture)
	}
}
